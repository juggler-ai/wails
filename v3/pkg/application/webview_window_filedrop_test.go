package application

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/events"
)

func newFileDropTestWindow(enabled bool, delivered *[][]string) *WebviewWindow {
	return &WebviewWindow{
		options: WebviewWindowOptions{EnableFileDrop: enabled},
		eventListeners: map[uint][]*WindowEventListener{
			uint(events.Common.WindowFilesDropped): {{
				callback: func(event *WindowEvent) {
					*delivered = append(*delivered, event.Context().DroppedFiles())
				},
			}},
		},
	}
}

func TestFilesDroppedNotDeliveredWhileFileDropDisabled(t *testing.T) {
	var delivered [][]string
	w := newFileDropTestWindow(false, &delivered)

	w.handleDragAndDropMessage([]string{"/tmp/secret.txt"}, nil)

	if len(delivered) != 0 {
		t.Fatalf("WindowFilesDropped fired with file drop disabled: %v", delivered)
	}
}

func TestFilesDroppedFollowsSetEnableFileDrop(t *testing.T) {
	var delivered [][]string
	w := newFileDropTestWindow(true, &delivered)

	w.handleDragAndDropMessage([]string{"/tmp/a.txt"}, nil)
	if len(delivered) != 1 {
		t.Fatalf("expected one delivery while enabled, got %d", len(delivered))
	}

	// A drop that was already in flight when the application turned file
	// drop off must not reach its listeners.
	w.SetEnableFileDrop(false)
	if w.IsFileDropEnabled() {
		t.Fatal("IsFileDropEnabled reported true after SetEnableFileDrop(false)")
	}
	w.handleDragAndDropMessage([]string{"/tmp/b.txt"}, nil)
	if len(delivered) != 1 {
		t.Fatalf("drop delivered after SetEnableFileDrop(false): %v", delivered)
	}

	w.SetEnableFileDrop(true)
	if !w.IsFileDropEnabled() {
		t.Fatal("IsFileDropEnabled reported false after SetEnableFileDrop(true)")
	}
	w.handleDragAndDropMessage([]string{"/tmp/c.txt"}, nil)
	if len(delivered) != 2 || delivered[1][0] != "/tmp/c.txt" {
		t.Fatalf("expected delivery after re-enabling, got %v", delivered)
	}
}
