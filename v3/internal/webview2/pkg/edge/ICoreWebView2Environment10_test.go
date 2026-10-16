//go:build windows

package edge

import (
	"testing"
	"unsafe"
)

// A method's vtable slot is fixed by WebView2.idl. A slot missing from, or
// added to, the inherited chain shifts every later method onto a different
// native function, which no compiler or type check can see.
func TestEnvironment10VtableSlots(t *testing.T) {
	var vtbl iCoreWebView2Environment10Vtbl
	slot := func(offset uintptr) uintptr { return offset / unsafe.Sizeof(ComProc(0)) }

	tests := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"CreateCoreWebView2Controller", slot(unsafe.Offsetof(vtbl.CreateCoreWebView2Controller)), 3},
		{"CreateWebResourceRequest", slot(unsafe.Offsetof(vtbl.CreateWebResourceRequest)), 8},
		{"CreateCoreWebView2CompositionController", slot(unsafe.Offsetof(vtbl.CreateCoreWebView2CompositionController)), 9},
		{"GetAutomationProviderForWindow", slot(unsafe.Offsetof(vtbl.GetAutomationProviderForWindow)), 11},
		{"CreateContextMenuItem", slot(unsafe.Offsetof(vtbl.CreateContextMenuItem)), 19},
		{"CreateCoreWebView2ControllerOptions", slot(unsafe.Offsetof(vtbl.CreateCoreWebView2ControllerOptions)), 20},
		{"CreateCoreWebView2ControllerWithOptions", slot(unsafe.Offsetof(vtbl.CreateCoreWebView2ControllerWithOptions)), 21},
		{"CreateCoreWebView2CompositionControllerWithOptions", slot(unsafe.Offsetof(vtbl.CreateCoreWebView2CompositionControllerWithOptions)), 22},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s is in slot %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

func TestControllerOptionsVtableSlots(t *testing.T) {
	var vtbl iCoreWebView2ControllerOptionsVtbl
	slot := unsafe.Offsetof(vtbl.PutIsInPrivateModeEnabled) / unsafe.Sizeof(ComProc(0))
	if slot != 6 {
		t.Errorf("PutIsInPrivateModeEnabled is in slot %d, want 6", slot)
	}
}
