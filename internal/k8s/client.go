package k8s

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"

	"github.com/skyhook-io/radar/internal/errorlog"
)

var (
	k8sClient          *kubernetes.Clientset
	k8sConfig          *rest.Config
	discoveryClient    *discovery.DiscoveryClient
	dynamicClient      dynamic.Interface
	initOnce           sync.Once
	initErr            error
	kubeconfigPath     string
	kubeconfigPaths    []string // Multiple kubeconfig paths when using --kubeconfig-dir or KUBECONFIG env
	kubeconfigMode     string   // One of: "in-cluster", "single", "multi-env", "multi-dir"
	mergedContextCount int      // Number of contexts exposed after client-go merged all kubeconfig files
	contextName        string
	clusterName        string
	contextNamespace   string // Default namespace from kubeconfig context
	fallbackNamespace  string // Explicit namespace from --namespace flag
	contextUsesExec    bool   // True when the current context uses an exec credential plugin
	// execPluginCommands is the set of unique exec-auth plugin command basenames
	// referenced by any context in the merged kubeconfig. Populated from
	// rawConfig.AuthInfos at load time and refreshed on SwitchContext. Stored
	// as basenames only so diagnostics never leak full binary paths. Used by
	// GetKubeconfigSummary() to produce present/missing lists against the
	// current process PATH.
	execPluginCommands []string
	// enrichedKubeconfigFromShell is set by the desktop app's enrichEnv() when
	// it successfully captured KUBECONFIG from the user's login shell. Surfaced
	// in diagnostics so we can tell whether the GUI app's env was enriched or
	// whether we fell back to whatever the parent process handed us. All access
	// goes through clientMu like the rest of the globals in this file —
	// callers use SetEnrichedKubeconfigFromShell to write.
	enrichedKubeconfigFromShell bool
	// clientMu protects access to client variables during context switches.
	// Readers use RLock, context switch uses Lock.
	clientMu sync.RWMutex
)

// SetEnrichedKubeconfigFromShell records that the desktop app's enrichEnv()
// successfully captured KUBECONFIG from the user's login shell. Used only for
// diagnostic reporting — does not affect K8s client behavior. Takes clientMu
// like every other write to the package-level state.
func SetEnrichedKubeconfigFromShell(v bool) {
	clientMu.Lock()
	defer clientMu.Unlock()
	enrichedKubeconfigFromShell = v
}

// InitOptions configures the K8s client initialization
type InitOptions struct {
	KubeconfigPath string
	KubeconfigDirs []string // Directories containing kubeconfig files
}

// Initialize initializes the K8s client with the given options
func Initialize(opts InitOptions) error {
	initOnce.Do(func() {
		initErr = doInit(opts)
	})
	return initErr
}

// MustInitialize is like Initialize but panics on error
func MustInitialize(opts InitOptions) {
	if err := Initialize(opts); err != nil {
		panic(fmt.Sprintf("failed to initialize k8s client: %v", err))
	}
}

func doInit(opts InitOptions) error {
	var config *rest.Config
	var err error

	// Configuration precedence (matches kubectl behavior):
	//   1. --kubeconfig flag (opts.KubeconfigPath)
	//   2. KUBECONFIG environment variable
	//   3. --kubeconfig-dir flag (opts.KubeconfigDirs)
	//   4. In-cluster config (when KUBERNETES_SERVICE_HOST is set)
	//   5. Default ~/.kube/config
	//
	// We only try in-cluster config if no explicit kubeconfig is specified.
	// This handles the case where KUBERNETES_SERVICE_HOST is set (e.g., inside
	// a pod) but the user wants to connect to a different cluster via kubeconfig.
	// See: https://github.com/kubernetes/kubernetes/issues/43662
	if opts.KubeconfigPath == "" && os.Getenv("KUBECONFIG") == "" && len(opts.KubeconfigDirs) == 0 {
		config, err = rest.InClusterConfig()
		if err == nil {
			contextName = "in-cluster"
			clusterName = "in-cluster"
			kubeconfigMode = "in-cluster"
		}
	}

	if config == nil {
		// Use kubeconfig (for local development / CLI usage)
		var loadingRules *clientcmd.ClientConfigLoadingRules

		if len(opts.KubeconfigDirs) > 0 {
			// Multi-kubeconfig mode: discover and merge configs from directories
			configs, err := discoverKubeconfigs(opts.KubeconfigDirs)
			if err != nil {
				return fmt.Errorf("failed to discover kubeconfigs: %w", err)
			}
			if len(configs) == 0 {
				return fmt.Errorf("no valid kubeconfig files found in directories: %v", opts.KubeconfigDirs)
			}
			log.Printf("Discovered %d kubeconfig files from %d directories", len(configs), len(opts.KubeconfigDirs))
			kubeconfigPaths = configs
			kubeconfigMode = "multi-dir"
			loadingRules = &clientcmd.ClientConfigLoadingRules{Precedence: configs}
		} else {
			// Single kubeconfig mode (existing behavior)
			kubeconfig := opts.KubeconfigPath
			if kubeconfig == "" {
				kubeconfig = os.Getenv("KUBECONFIG")
			}
			if kubeconfig == "" {
				if home := homedir.HomeDir(); home != "" {
					kubeconfig = filepath.Join(home, ".kube", "config")
				}
			}

			// KUBECONFIG can contain multiple paths separated by the OS path
			// list separator (colon on Unix, semicolon on Windows).
			// Split and use Precedence when there are multiple entries.
			if paths := filepath.SplitList(kubeconfig); len(paths) > 1 {
				kubeconfigPaths = paths
				kubeconfigMode = "multi-env"
				loadingRules = &clientcmd.ClientConfigLoadingRules{Precedence: paths}
				log.Printf("KUBECONFIG contains %d paths, using merged mode", len(paths))
			} else {
				kubeconfigPath = kubeconfig
				kubeconfigMode = "single"
				loadingRules = &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
			}
		}

		configOverrides := &clientcmd.ConfigOverrides{}
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

		// Get raw config to extract context/cluster names. If this fails
		// we still let ClientConfig() run below — it's likely to fail too,
		// but we record the two failures separately so the snapshot shows
		// the first error without bookkeeping getting silently skipped.
		// Emitting at "error" severity (not "warning") because a RawConfig
		// failure zeroes out every downstream diagnostic field this PR
		// exists to surface — the entry must not be easy to overlook.
		rawConfig, rawErr := kubeConfig.RawConfig()
		if rawErr != nil {
			log.Printf("Kubeconfig metadata load failed (mode=%s): %v", kubeconfigMode, rawErr)
			errorlog.Record("k8s-init", "error",
				"RawConfig() failed; context metadata and diagnostic counts unavailable: %v", rawErr)
		} else {
			contextName = rawConfig.CurrentContext
			mergedContextCount = len(rawConfig.Contexts)
			cmds, emptyAIs := collectExecPluginCommands(&rawConfig)
			execPluginCommands = cmds
			if len(emptyAIs) > 0 {
				// Aggregate into a single errorlog entry — a pathological
				// kubeconfig with hundreds of broken AuthInfos would otherwise
				// flood the 200-entry ring buffer and evict other diagnostics.
				recordEmptyCommandWarning("k8s-init", emptyAIs)
			}
			fileCount := len(kubeconfigPaths)
			if fileCount == 0 && kubeconfigPath != "" {
				fileCount = 1
			}
			// Log the merged context count so multi-file collisions are visible.
			// client-go silently drops later duplicates when merging by name, so
			// this count is the "ground truth" of what the user can actually pick
			// from the dropdown — not the sum of per-file contexts.
			log.Printf("Kubeconfig loaded: mode=%s, files=%d, contexts=%d, exec-plugins=%d",
				kubeconfigMode, fileCount, mergedContextCount, len(execPluginCommands))
			if ctx, ok := rawConfig.Contexts[contextName]; ok {
				clusterName = ctx.Cluster
				contextNamespace = ctx.Namespace
				if ai, ok := rawConfig.AuthInfos[ctx.AuthInfo]; ok && ai.Exec != nil {
					contextUsesExec = true
				}
			}
		}

		config, err = kubeConfig.ClientConfig()
		if err != nil {
			// Record to errorlog so the failure lands in the diagnostics
			// snapshot's recentErrors. Include only the file count and mode —
			// never the kubeconfig paths — so the snapshot stays shareable.
			errorlog.Record("k8s-init", "error",
				"failed to build kubeconfig client config (mode=%s, files=%d): %v",
				kubeconfigMode, len(kubeconfigPaths), err)
			if len(kubeconfigPaths) > 0 {
				return fmt.Errorf("failed to build kubeconfig from %d files: %w", len(kubeconfigPaths), err)
			}
			return fmt.Errorf("failed to build kubeconfig from %s: %w", kubeconfigPath, err)
		}
	}

	// Increase QPS/Burst to speed up CRD discovery and reduce throttling
	// Default client-go is 5 QPS / 10 Burst, kubectl uses 50/100
	// This is safe for a read-only visibility tool
	config.QPS = 50
	config.Burst = 100

	k8sConfig = config

	k8sClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create k8s clientset: %w", err)
	}

	// Create discovery client for API resource discovery
	discoveryClient, err = discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}

	// Create dynamic client for CRD access
	dynamicClient, err = dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return nil
}

// discoverKubeconfigs scans directories for valid kubeconfig files
func discoverKubeconfigs(dirs []string) ([]string, error) {
	var configs []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			log.Printf("Warning: cannot read kubeconfig directory %s: %v", dir, err)
			// Surface scan failures in the diagnostics snapshot so "my
			// dropdown is empty" reports can tell permission/missing-dir
			// apart from "dir was there but held no valid configs".
			// Strip full paths from the error text via *os.PathError so
			// the snapshot stays shareable — just Op + underlying cause.
			errorlog.Record("k8s-init", "warning",
				"kubeconfig dir %q scan failed: %s",
				filepath.Base(dir), scrubPathError(err))
			continue // Skip inaccessible dirs
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			// Skip hidden files and common non-config files
			name := entry.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			path := filepath.Join(dir, name)
			if isValidKubeconfig(path) {
				configs = append(configs, path)
				log.Printf("Found kubeconfig: %s", path)
			} else {
				log.Printf("Skipping invalid kubeconfig: %s", path)
				// Per-file parse/validation failures are invisible from
				// the merged-config counts alone — a broken file lowers
				// fileCount without explaining why. Record the basename
				// (never the full path) so the triager knows which file
				// to ask the user about.
				errorlog.Record("k8s-init", "warning",
					"skipping invalid kubeconfig file %q during directory scan",
					filepath.Base(path))
			}
		}
	}
	return configs, nil
}

// scrubPathError returns the underlying error cause (e.g. "permission denied",
// "no such file or directory") without the filesystem path that produced it,
// so errorlog entries derived from os.ReadDir / os.Open can safely ship in a
// bug report. Errors that aren't an `*os.PathError` (or whose inner Err is
// nil) are *not* passed through via err.Error() — their text may still
// contain the originating path — so they collapse to a conservative
// "unscrubbable" placeholder. The helper's entire point is the privacy
// contract; a future caller adding a non-PathError must not silently leak.
func scrubPathError(err error) string {
	if err == nil {
		return ""
	}
	var pErr *os.PathError
	if errors.As(err, &pErr) && pErr.Err != nil {
		return pErr.Op + ": " + pErr.Err.Error()
	}
	return "(unscrubbable error — omitted to avoid leaking paths)"
}

// isValidKubeconfig checks if a file is a valid kubeconfig
func isValidKubeconfig(path string) bool {
	// Try to load the file as a kubeconfig
	config, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return false
	}
	// A valid kubeconfig should have at least one context or cluster
	return len(config.Contexts) > 0 || len(config.Clusters) > 0
}

// GetClient returns the K8s clientset
func GetClient() *kubernetes.Clientset {
	clientMu.RLock()
	defer clientMu.RUnlock()
	return k8sClient
}

// GetConfig returns the K8s rest config
func GetConfig() *rest.Config {
	clientMu.RLock()
	defer clientMu.RUnlock()
	return k8sConfig
}

// GetDiscoveryClient returns the K8s discovery client for API resource discovery
func GetDiscoveryClient() *discovery.DiscoveryClient {
	clientMu.RLock()
	defer clientMu.RUnlock()
	return discoveryClient
}

// GetDynamicClient returns the K8s dynamic client for CRD access
func GetDynamicClient() dynamic.Interface {
	clientMu.RLock()
	defer clientMu.RUnlock()
	return dynamicClient
}

// GetKubeconfigPath returns the path to the kubeconfig file used
func GetKubeconfigPath() string {
	clientMu.RLock()
	defer clientMu.RUnlock()
	return kubeconfigPath
}

// KubeconfigSummary is a non-sensitive snapshot of kubeconfig loading state,
// suitable for inclusion in diagnostic output. It never includes the resolved
// paths themselves, only counts, mode flags, and exec plugin basenames.
type KubeconfigSummary struct {
	Mode                   string   // "in-cluster", "single", "multi-env", "multi-dir", or "" if not initialized
	FileCount              int      // Number of kubeconfig files loaded (0 for in-cluster)
	ContextCount           int      // Number of contexts exposed after client-go merged all files
	EnrichedFromShell      bool     // Desktop app captured KUBECONFIG from login shell
	CurrentContextUsesExec bool     // Current context's AuthInfo uses an exec credential plugin
	ExecPluginsPresent     []string // Unique exec plugin command basenames (any context) resolvable on $PATH
	ExecPluginsMissing     []string // Unique exec plugin command basenames (any context) NOT resolvable on $PATH
}

// GetKubeconfigSummary returns the current kubeconfig loading state for
// diagnostics. All values are safe to include in a bug report.
//
// ExecPluginsPresent/Missing are computed lazily against the *current*
// process PATH at snapshot time (not init time) so a user who installs
// `gke-gcloud-auth-plugin` (or similar) *after* launching Radar sees the
// plugin move from "missing" to "present" in their next snapshot without
// restarting — and a user whose PATH is smaller in a long-running session
// still gets accurate data.
func GetKubeconfigSummary() KubeconfigSummary {
	clientMu.RLock()
	mode := kubeconfigMode
	fileCount := len(kubeconfigPaths)
	if fileCount == 0 && kubeconfigPath != "" {
		fileCount = 1
	}
	contextCount := mergedContextCount
	enriched := enrichedKubeconfigFromShell
	currentExec := contextUsesExec
	cmds := append([]string(nil), execPluginCommands...)
	clientMu.RUnlock()

	// LookPath outside the lock — it can stat the filesystem and we don't
	// want to hold clientMu across I/O.
	var present, missing []string
	for _, cmd := range cmds {
		if _, err := exec.LookPath(cmd); err == nil {
			present = append(present, cmd)
		} else {
			missing = append(missing, cmd)
		}
	}

	return KubeconfigSummary{
		Mode:                   mode,
		FileCount:              fileCount,
		ContextCount:           contextCount,
		EnrichedFromShell:      enriched,
		CurrentContextUsesExec: currentExec,
		ExecPluginsPresent:     present,
		ExecPluginsMissing:     missing,
	}
}

// collectExecPluginCommands walks every context in raw and returns:
//
//   - cmds: the unique, sorted basenames of any exec plugin command
//     referenced by a context's AuthInfo. Basenames only — never full
//     paths — so the result is safe to surface in diagnostics.
//   - emptyCommandAuthInfos: the unique, sorted names of AuthInfos that
//     reference an exec block with an empty Command. This is a user
//     misconfiguration that will fail at auth time — the caller should
//     record each one via errorlog so it shows up in a bug report.
//
// Orphan AuthInfos (not referenced by any context) are intentionally
// skipped: they can't cause a context switch to fail, so there's no
// signal in them.
//
// The function is pure on its *clientcmdapi.Config argument and touches
// no shared state, so it is safe to call without any lock held. Callers
// are responsible for assigning the returned cmds slice to the package
// global `execPluginCommands` under clientMu.Lock.
func collectExecPluginCommands(raw *clientcmdapi.Config) (cmds []string, emptyCommandAuthInfos []string) {
	if raw == nil {
		return nil, nil
	}
	seenCmds := make(map[string]struct{})
	seenEmpty := make(map[string]struct{})
	for _, ctx := range raw.Contexts {
		if ctx == nil {
			continue
		}
		ai, ok := raw.AuthInfos[ctx.AuthInfo]
		if !ok || ai == nil || ai.Exec == nil {
			continue
		}
		if ai.Exec.Command == "" {
			// Malformed exec block — surface via the second return
			// so the caller can record a warning. Dedupe by AuthInfo
			// name since the same AuthInfo may be referenced by
			// multiple contexts.
			if _, dup := seenEmpty[ctx.AuthInfo]; !dup {
				seenEmpty[ctx.AuthInfo] = struct{}{}
				emptyCommandAuthInfos = append(emptyCommandAuthInfos, ctx.AuthInfo)
			}
			continue
		}
		base := filepath.Base(ai.Exec.Command)
		if _, dup := seenCmds[base]; dup {
			continue
		}
		seenCmds[base] = struct{}{}
		cmds = append(cmds, base)
	}
	sort.Strings(cmds)
	sort.Strings(emptyCommandAuthInfos)
	return cmds, emptyCommandAuthInfos
}

// recordEmptyCommandWarning records a single aggregated errorlog entry for a
// batch of AuthInfos that reference exec plugins with an empty Command. A
// single errorlog call (rather than one-per-name) is deliberate — a
// pathological or corrupted kubeconfig with hundreds of broken AuthInfos
// would otherwise flood the 200-entry ring buffer and evict unrelated
// diagnostics. Listing is capped at the first maxListed names so the
// message text itself stays bounded; the count is always accurate.
func recordEmptyCommandWarning(source string, authInfos []string) {
	if len(authInfos) == 0 {
		return
	}
	const maxListed = 10
	listed := authInfos
	truncated := false
	if len(listed) > maxListed {
		listed = listed[:maxListed]
		truncated = true
	}
	suffix := ""
	if truncated {
		suffix = fmt.Sprintf(" (+%d more)", len(authInfos)-maxListed)
	}
	errorlog.Record(source, "warning",
		"%d AuthInfo(s) reference exec plugins with empty command — context switches to these identities will fail at auth time: %v%s",
		len(authInfos), listed, suffix)
}

// WriteKubeconfigForCurrentContext creates a temporary kubeconfig file with
// current-context set to Radar's active context. The caller must remove the
// file when done. Returns the temp file path.
func WriteKubeconfigForCurrentContext() (string, error) {
	clientMu.RLock()
	ctx := contextName
	paths := kubeconfigPaths
	singlePath := kubeconfigPath
	clientMu.RUnlock()

	var loadingRules *clientcmd.ClientConfigLoadingRules
	if len(paths) > 0 {
		loadingRules = &clientcmd.ClientConfigLoadingRules{Precedence: paths}
	} else {
		if singlePath == "" {
			return "", fmt.Errorf("kubeconfig path not set")
		}
		loadingRules = &clientcmd.ClientConfigLoadingRules{ExplicitPath: singlePath}
	}

	rawConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{},
	).RawConfig()
	if err != nil {
		return "", fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	if ctx != "" {
		rawConfig.CurrentContext = ctx
	}

	tmpFile, err := os.CreateTemp("", "radar-kubeconfig-*.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to create temp kubeconfig: %w", err)
	}
	tmpFile.Close()

	if err := clientcmd.WriteToFile(rawConfig, tmpFile.Name()); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write temp kubeconfig: %w", err)
	}

	return tmpFile.Name(), nil
}

// GetContextName returns the current kubeconfig context name
func GetContextName() string {
	clientMu.RLock()
	defer clientMu.RUnlock()
	return contextName
}

// GetClusterName returns the current cluster name from kubeconfig
func GetClusterName() string {
	clientMu.RLock()
	defer clientMu.RUnlock()
	return clusterName
}

// GetContextNamespace returns the default namespace from the kubeconfig context
func GetContextNamespace() string {
	clientMu.RLock()
	defer clientMu.RUnlock()
	return contextNamespace
}

// UsesExecAuth returns true if the current context uses an exec credential plugin.
// This covers any plugin configured in kubeconfig AuthInfo.Exec (e.g., GKE, EKS,
// AKS, OIDC/Dex/Keycloak, Teleport). These plugins can hang when credentials
// expire, causing generic timeouts instead of auth errors.
func UsesExecAuth() bool {
	clientMu.RLock()
	defer clientMu.RUnlock()
	return contextUsesExec
}

// SetFallbackNamespace sets an explicit namespace to use as RBAC fallback
// (typically from the --namespace CLI flag). Used when the kubeconfig context
// doesn't specify a namespace but the user wants namespace-scoped access.
func SetFallbackNamespace(ns string) {
	clientMu.Lock()
	defer clientMu.Unlock()
	fallbackNamespace = ns
}

// GetEffectiveNamespace returns the namespace to use for RBAC fallback checks.
// Prefers the kubeconfig context namespace, falls back to the explicit --namespace flag.
func GetEffectiveNamespace() string {
	clientMu.RLock()
	defer clientMu.RUnlock()
	if contextNamespace != "" {
		return contextNamespace
	}
	return fallbackNamespace
}

// ForceInCluster overrides in-cluster detection for testing
var ForceInCluster bool

// IsInCluster returns true if running inside a Kubernetes cluster
func IsInCluster() bool {
	return ForceInCluster || (kubeconfigPath == "" && len(kubeconfigPaths) == 0)
}

// ContextInfo represents information about a kubeconfig context
type ContextInfo struct {
	Name      string `json:"name"`
	Cluster   string `json:"cluster"`
	User      string `json:"user"`
	Namespace string `json:"namespace"`
	IsCurrent bool   `json:"isCurrent"`
}

// GetAvailableContexts returns all available contexts from the kubeconfig
func GetAvailableContexts() ([]ContextInfo, error) {
	if IsInCluster() {
		// In-cluster mode - only one "context" available
		return []ContextInfo{
			{
				Name:      "in-cluster",
				Cluster:   "in-cluster",
				User:      "service-account",
				Namespace: "",
				IsCurrent: true,
			},
		}, nil
	}

	var loadingRules *clientcmd.ClientConfigLoadingRules
	if len(kubeconfigPaths) > 0 {
		// Multi-kubeconfig mode
		loadingRules = &clientcmd.ClientConfigLoadingRules{Precedence: kubeconfigPaths}
	} else {
		// Single kubeconfig mode
		kubeconfig := kubeconfigPath
		if kubeconfig == "" {
			return nil, fmt.Errorf("kubeconfig path not set")
		}
		loadingRules = &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	rawConfig, err := kubeConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Use Explorer's in-memory contextName to determine current context
	// This allows Explorer to switch contexts without modifying the kubeconfig file
	currentCtx := contextName
	if currentCtx == "" {
		// Fall back to kubeconfig's current-context if we haven't switched yet
		currentCtx = rawConfig.CurrentContext
	}

	contexts := make([]ContextInfo, 0, len(rawConfig.Contexts))
	for name, ctx := range rawConfig.Contexts {
		contexts = append(contexts, ContextInfo{
			Name:      name,
			Cluster:   ctx.Cluster,
			User:      ctx.AuthInfo,
			Namespace: ctx.Namespace,
			IsCurrent: name == currentCtx,
		})
	}

	return contexts, nil
}

// SwitchContext switches the K8s client to use a different context
// This reinitializes all clients (k8sClient, discoveryClient, dynamicClient)
func SwitchContext(name string) error {
	if IsInCluster() {
		return fmt.Errorf("cannot switch context when running in-cluster")
	}

	var loadingRules *clientcmd.ClientConfigLoadingRules
	if len(kubeconfigPaths) > 0 {
		// Multi-kubeconfig mode
		loadingRules = &clientcmd.ClientConfigLoadingRules{Precedence: kubeconfigPaths}
	} else {
		// Single kubeconfig mode
		kubeconfig := kubeconfigPath
		if kubeconfig == "" {
			return fmt.Errorf("kubeconfig path not set")
		}
		loadingRules = &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	}

	// Build config with the new context
	configOverrides := &clientcmd.ConfigOverrides{CurrentContext: name}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	// Verify the context exists
	rawConfig, err := kubeConfig.RawConfig()
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	ctx, ok := rawConfig.Contexts[name]
	if !ok {
		return fmt.Errorf("context %q not found in kubeconfig", name)
	}

	// Build the REST config for the new context
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to build config for context %q: %w", name, err)
	}

	// Apply the same QPS/Burst settings as initial client creation.
	// Without this, new clients use the default 5 QPS / 10 Burst, causing
	// severe client-side throttling during CRD discovery after context switch.
	config.QPS = 50
	config.Burst = 100

	// Create new clients
	newK8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create k8s client for context %q: %w", name, err)
	}

	newDiscoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create discovery client for context %q: %w", name, err)
	}

	newDynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client for context %q: %w", name, err)
	}

	// Update global variables atomically
	usesExec := false
	if ai, ok := rawConfig.AuthInfos[ctx.AuthInfo]; ok && ai.Exec != nil {
		usesExec = true
	}
	// Re-collect exec plugin commands — SwitchContext loads a fresh RawConfig
	// so the user may have added/removed kubeconfig files since init.
	execCmds, emptyAIs := collectExecPluginCommands(&rawConfig)
	if len(emptyAIs) > 0 {
		recordEmptyCommandWarning("context-switch", emptyAIs)
	}

	clientMu.Lock()
	k8sConfig = config
	k8sClient = newK8sClient
	discoveryClient = newDiscoveryClient
	dynamicClient = newDynamicClient
	contextName = name
	clusterName = ctx.Cluster
	contextNamespace = ctx.Namespace
	contextUsesExec = usesExec
	mergedContextCount = len(rawConfig.Contexts)
	execPluginCommands = execCmds
	clientMu.Unlock()

	return nil
}

// capiKubeconfigs tracks temp kubeconfig files by context name to avoid accumulation.
var capiKubeconfigs = make(map[string]string) // contextName -> tmpPath

// MergeAndSwitchContext writes the provided kubeconfig data to a temporary file
// and adds it to Radar's kubeconfig search path so that the context becomes
// available for switching. Returns the temp file path.
func MergeAndSwitchContext(kubeconfigData []byte, contextName string) (string, error) {
	// Parse the incoming kubeconfig
	newConfig, err := clientcmd.Load(kubeconfigData)
	if err != nil {
		return "", fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	// Verify the context exists
	if _, ok := newConfig.Contexts[contextName]; !ok {
		return "", fmt.Errorf("context %q not found in provided kubeconfig", contextName)
	}

	// Reuse existing temp file for same context (avoids accumulation)
	clientMu.Lock()
	existingPath := capiKubeconfigs[contextName]
	clientMu.Unlock()

	if existingPath != "" {
		// Overwrite existing file with fresh kubeconfig data
		if err := clientcmd.WriteToFile(*newConfig, existingPath); err == nil {
			log.Printf("[capi] Updated existing kubeconfig for context %s: %s", contextName, existingPath)
			return existingPath, nil
		}
		// If overwrite fails, fall through to create a new file
	}

	// Write to a new temp file
	tmpFile, err := os.CreateTemp("", "radar-capi-kubeconfig-*.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to create temp kubeconfig: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	if err := clientcmd.WriteToFile(*newConfig, tmpPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	// Add the temp file to Radar's kubeconfig search path.
	// In single-kubeconfig mode, kubeconfigPaths is empty — seed it with the
	// original path so SwitchContext still sees the user's other contexts.
	clientMu.Lock()
	if len(kubeconfigPaths) == 0 && kubeconfigPath != "" {
		kubeconfigPaths = []string{kubeconfigPath}
	}
	// Remove stale path for this context if overwrite failed above
	if existingPath != "" {
		updated := kubeconfigPaths[:0]
		for _, p := range kubeconfigPaths {
			if p != existingPath {
				updated = append(updated, p)
			}
		}
		kubeconfigPaths = updated
	}
	kubeconfigPaths = append(kubeconfigPaths, tmpPath)
	capiKubeconfigs[contextName] = tmpPath
	clientMu.Unlock()

	log.Printf("[capi] Added workload cluster kubeconfig: %s (context: %s)", tmpPath, contextName)
	return tmpPath, nil
}
