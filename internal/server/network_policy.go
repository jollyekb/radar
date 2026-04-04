package server

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/skyhook-io/radar/internal/k8s"
)

// PolicyEvaluation is the response for /api/network-policies/evaluate.
type PolicyEvaluation struct {
	SelectingPolicies []PolicyMatch `json:"selectingPolicies"`
	Verdict           string        `json:"verdict"` // "allowed", "denied", "no-policy"
}

// PolicyMatch describes a single policy's effect on a flow.
type PolicyMatch struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Kind      string `json:"kind"` // "NetworkPolicy", "CiliumNetworkPolicy", etc.
	Effect    string `json:"effect"` // "allow", "deny"
	Reason    string `json:"reason"` // human-readable explanation
}

// handleEvaluateNetworkPolicies evaluates which NetworkPolicies select the given
// destination pods and whether any allow traffic from the given source.
//
// Query params:
//
//	namespace      - destination pod namespace (required)
//	labels         - destination pod labels as key=value pairs, comma-separated (required)
//	sourceNamespace - source pod namespace (optional)
//	sourceLabels   - source pod labels as key=value pairs, comma-separated (optional)
//	port           - destination port number (optional, for port-specific matching)
//	protocol       - protocol: TCP, UDP, SCTP (optional, default TCP)
func (s *Server) handleEvaluateNetworkPolicies(w http.ResponseWriter, r *http.Request) {
	if !s.requireConnected(w) {
		return
	}

	cache := k8s.GetResourceCache()
	if cache == nil {
		s.writeError(w, http.StatusServiceUnavailable, "resource cache not ready")
		return
	}

	// Parse params
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		s.writeError(w, http.StatusBadRequest, "namespace is required")
		return
	}
	labelsParam := r.URL.Query().Get("labels")
	destLabels := parseLabelsParam(labelsParam)

	// If no labels provided but podName is, resolve labels from the pod cache
	if len(destLabels) == 0 {
		if podName := r.URL.Query().Get("podName"); podName != "" {
			destLabels = resolvePodLabels(cache, ns, podName)
		}
	}

	srcNs := r.URL.Query().Get("sourceNamespace")
	srcLabelsParam := r.URL.Query().Get("sourceLabels")
	srcLabels := parseLabelsParam(srcLabelsParam)

	// Resolve source labels from pod name if needed
	if len(srcLabels) == 0 {
		if srcPodName := r.URL.Query().Get("sourcePodName"); srcPodName != "" && srcNs != "" {
			srcLabels = resolvePodLabels(cache, srcNs, srcPodName)
		}
	}

	// Collect selecting policies
	var matches []PolicyMatch
	anyAllows := false

	// 1. Standard NetworkPolicies
	if npLister := cache.NetworkPolicies(); npLister != nil {
		nps, err := npLister.NetworkPolicies(ns).List(labels.Everything())
		if err != nil {
			log.Printf("[network-policy] Failed to list NetworkPolicies in %s: %v", ns, err)
		}
		for _, np := range nps {
			sel, err := metav1.LabelSelectorAsSelector(&np.Spec.PodSelector)
			if err != nil {
				continue
			}
			if !sel.Matches(labels.Set(destLabels)) {
				continue
			}

			effect, reason := evaluateStandardPolicy(np, srcNs, srcLabels)
			matches = append(matches, PolicyMatch{
				Name:      np.Name,
				Namespace: np.Namespace,
				Kind:      "NetworkPolicy",
				Effect:    effect,
				Reason:    reason,
			})
			if effect == "allow" {
				anyAllows = true
			}
		}
	}

	// 2. CiliumNetworkPolicies (from dynamic cache)
	if dynamicCache := k8s.GetDynamicResourceCache(); dynamicCache != nil {
		if discovery := k8s.GetResourceDiscovery(); discovery != nil {
			if cnpGVR, ok := discovery.GetGVR("CiliumNetworkPolicy"); ok {
				cnps, err := dynamicCache.List(cnpGVR, ns)
				if err == nil {
					for _, cnp := range cnps {
						selectorMap, _, _ := unstructured.NestedMap(cnp.Object, "spec", "endpointSelector", "matchLabels")
						if !matchesCRDSelector(destLabels, selectorMap) {
							continue
						}
						matches = append(matches, PolicyMatch{
							Name:      cnp.GetName(),
							Namespace: cnp.GetNamespace(),
							Kind:      "CiliumNetworkPolicy",
							Effect:    "deny",
							Reason:    "Cilium policy selects this endpoint (detailed rule evaluation not yet supported)",
						})
					}
				}
			}
		}
	}

	// Determine overall verdict
	verdict := "no-policy"
	if len(matches) > 0 {
		if anyAllows {
			verdict = "allowed"
		} else {
			verdict = "denied"
		}
	}

	s.writeJSON(w, PolicyEvaluation{
		SelectingPolicies: matches,
		Verdict:           verdict,
	})
}

// evaluateStandardPolicy checks if a NetworkPolicy's ingress rules allow
// traffic from the given source. Returns effect ("allow"/"deny") and reason.
func evaluateStandardPolicy(np *networkingv1.NetworkPolicy, srcNs string, srcLabels map[string]string) (string, string) {
	hasIngress := false
	for _, pt := range np.Spec.PolicyTypes {
		if pt == networkingv1.PolicyTypeIngress {
			hasIngress = true
			break
		}
	}
	if !hasIngress {
		return "deny", "no Ingress policy type (egress-only policy)"
	}

	if len(np.Spec.Ingress) == 0 {
		return "deny", "denies all ingress (no rules)"
	}

	for _, rule := range np.Spec.Ingress {
		if len(rule.From) == 0 {
			return "allow", "allows all sources (empty from)"
		}
		for _, peer := range rule.From {
			if matchesPeer(peer, srcNs, srcLabels, np.Namespace) {
				return "allow", formatPeerMatch(peer)
			}
		}
	}

	return "deny", fmt.Sprintf("no ingress rule matches source %s", formatLabels(srcLabels))
}

// matchesPeer checks if a source matches a NetworkPolicyPeer.
func matchesPeer(peer networkingv1.NetworkPolicyPeer, srcNs string, srcLabels map[string]string, policyNs string) bool {
	if peer.IPBlock != nil {
		return false // can't match pod-based source against CIDR
	}

	nsMatch := false
	if peer.NamespaceSelector != nil {
		nsSel, err := metav1.LabelSelectorAsSelector(peer.NamespaceSelector)
		if err != nil {
			return false
		}
		// Without namespace labels, we can check: empty selector = all namespaces,
		// same namespace always matches in practice
		if nsSel.Empty() || srcNs == policyNs {
			nsMatch = true
		}
	} else {
		// No namespaceSelector = same namespace only
		nsMatch = (srcNs == policyNs)
	}
	if !nsMatch {
		return false
	}

	if peer.PodSelector != nil {
		podSel, err := metav1.LabelSelectorAsSelector(peer.PodSelector)
		if err != nil {
			return false
		}
		return podSel.Matches(labels.Set(srcLabels))
	}

	// namespaceSelector only, no podSelector = all pods in matching namespaces
	return true
}

func matchesCRDSelector(podLabels map[string]string, selectorMap map[string]any) bool {
	if len(selectorMap) == 0 {
		return true // empty selector = all pods
	}
	for k, v := range selectorMap {
		sv, ok := v.(string)
		if !ok {
			return false
		}
		if podLabels[k] != sv {
			return false
		}
	}
	return true
}

func formatPeerMatch(peer networkingv1.NetworkPolicyPeer) string {
	parts := []string{}
	if peer.PodSelector != nil {
		parts = append(parts, "pods matching "+formatLabelSelector(peer.PodSelector))
	}
	if peer.NamespaceSelector != nil {
		sel := formatLabelSelector(peer.NamespaceSelector)
		if sel == "{}" {
			parts = append(parts, "any namespace")
		} else {
			parts = append(parts, "namespaces matching "+sel)
		}
	}
	if len(parts) == 0 {
		return "allows this source"
	}
	return "allows from " + strings.Join(parts, " in ")
}

func formatLabelSelector(sel *metav1.LabelSelector) string {
	if sel == nil {
		return "{}"
	}
	labels := sel.MatchLabels
	if len(labels) == 0 && len(sel.MatchExpressions) == 0 {
		return "{}"
	}
	parts := []string{}
	for k, v := range labels {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ", ")
}

func formatLabels(l map[string]string) string {
	if len(l) == 0 {
		return "(no labels)"
	}
	parts := []string{}
	for k, v := range l {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ", ")
}

func resolvePodLabels(cache *k8s.ResourceCache, namespace, podName string) map[string]string {
	if podLister := cache.Pods(); podLister != nil {
		pod, err := podLister.Pods(namespace).Get(podName)
		if err == nil && pod != nil {
			return pod.Labels
		}
	}
	return nil
}

func parseLabelsParam(param string) map[string]string {
	result := make(map[string]string)
	if param == "" {
		return result
	}
	for _, pair := range strings.Split(param, ",") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			result[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return result
}
