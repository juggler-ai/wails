//go:build darwin && !ios && !server

package application

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// buildEscapeFullscreenProbe compiles testdata/escape_fullscreen/main.m, which
// drives windowDropsEscapeCommand from webview_window_darwin.h — the decision
// WebviewWindow and WebviewPanel make for DisableEscapeExitsFullscreen.
func buildEscapeFullscreenProbe(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("xcrun"); err != nil {
		t.Skip("requires Xcode command line tools")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "escape-fullscreen")
	macNativeCommand(t, "xcrun", "clang", "-Werror", "-I", root,
		"testdata/escape_fullscreen/main.m", "-framework", "Cocoa", "-framework", "WebKit", "-o", executable)
	return executable
}

func TestEscapeFullscreenCommandFilter(t *testing.T) {
	macNativeCommand(t, buildEscapeFullscreenProbe(t), "--filter")
}

// TestEscapeKeepsFullscreen presses Escape in a focused <textarea> of a real
// fullscreen window, where WebKit's command for the key reaches the window as
// doCommandBySelector:cancelOperation: rather than as cancelOperation:. It
// needs a logged-in display session and takes over the screen while it runs,
// so it is opt-in.
func TestEscapeKeepsFullscreen(t *testing.T) {
	if os.Getenv("WAILS_TEST_FULLSCREEN") == "" {
		t.Skip("takes the screen into fullscreen; set WAILS_TEST_FULLSCREEN=1 to run")
	}
	executable := buildEscapeFullscreenProbe(t)
	t.Run("option on stays fullscreen", func(t *testing.T) {
		macNativeCommand(t, executable, "--fullscreen")
	})
	t.Run("option off leaves fullscreen", func(t *testing.T) {
		macNativeCommand(t, executable, "--fullscreen", "--allow")
	})
}
