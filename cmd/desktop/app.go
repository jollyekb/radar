package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/skyhook-io/radar/internal/app"
	"github.com/skyhook-io/radar/internal/k8s"
	"github.com/skyhook-io/radar/internal/server"
	"github.com/skyhook-io/radar/internal/timeline"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// DesktopApp manages the desktop application lifecycle.
type DesktopApp struct {
	ctx              context.Context
	srv              *server.Server
	timelineStoreCfg timeline.StoreConfig
}

func NewDesktopApp(srv *server.Server, timelineStoreCfg timeline.StoreConfig) *DesktopApp {
	return &DesktopApp{
		srv:              srv,
		timelineStoreCfg: timelineStoreCfg,
	}
}

// startup is called when the Wails app starts.
func (a *DesktopApp) startup(ctx context.Context) {
	a.ctx = ctx
	startNativeMouseMonitor(ctx)
	a.srv.SetSaveFileFunc(a.saveFile)
}

// saveFile writes a file to the user's Downloads folder.
// We write directly to ~/Downloads instead of showing a native save dialog
// because Wails' SaveFileDialog is immediately dismissed by the webview on macOS.
func (a *DesktopApp) saveFile(defaultFilename string, data []byte) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("cannot access Downloads folder: %w", err)
	}

	// Collision handling: file.txt → file (1).txt → file (2).txt
	base := defaultFilename
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	path := filepath.Join(dir, base)
	for i := 1; i <= 1000; i++ {
		_, statErr := os.Stat(path)
		if os.IsNotExist(statErr) {
			break
		}
		if statErr != nil {
			return "", fmt.Errorf("cannot check file %q: %w", path, statErr)
		}
		path = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", name, i, ext))
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", err
	}
	return path, nil
}

// domReady is called when the webview DOM is ready.
func (a *DesktopApp) domReady(ctx context.Context) {
	// Update window title with cluster context
	contextName := k8s.GetContextName()
	if contextName != "" {
		wailsRuntime.WindowSetTitle(ctx, "Radar — "+contextName)
	}
}

// beforeClose is called before the window closes. Return true to prevent closing.
func (a *DesktopApp) beforeClose(ctx context.Context) bool {
	return false // allow close
}

// shutdown is called when the application is shutting down.
func (a *DesktopApp) shutdown(ctx context.Context) {
	stopNativeMouseMonitor()
	log.Println("Desktop app shutting down...")
	app.Shutdown(a.srv)
}
