//go:build windows

package edge

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The vtable carries every slot ICoreWebView2Environment10 inherits, in
// WebView2.idl declaration order, so that its own three methods land at
// slots 20-22.
type iCoreWebView2Environment10Vtbl struct {
	iCoreWebView2Environment3Vtbl
	GetAutomationProviderForWindow                     ComProc // Environment4
	AddBrowserProcessExited                            ComProc // Environment5
	RemoveBrowserProcessExited                         ComProc
	CreatePrintSettings                                ComProc // Environment6
	GetUserDataFolder                                  ComProc // Environment7
	AddProcessInfosChanged                             ComProc // Environment8
	RemoveProcessInfosChanged                          ComProc
	GetProcessInfos                                    ComProc
	CreateContextMenuItem                              ComProc // Environment9
	CreateCoreWebView2ControllerOptions                ComProc // Environment10
	CreateCoreWebView2ControllerWithOptions            ComProc
	CreateCoreWebView2CompositionControllerWithOptions ComProc
}

type ICoreWebView2Environment10 struct {
	vtbl *iCoreWebView2Environment10Vtbl
}

func (e *ICoreWebView2Environment10) Release() uintptr {
	ret, _, _ := e.vtbl.Release.Call(uintptr(unsafe.Pointer(e)))

	return ret
}

// GetICoreWebView2Environment10 returns nil on a runtime that predates the
// interface. The caller must Release a non-nil result.
func (e *ICoreWebView2Environment) GetICoreWebView2Environment10() *ICoreWebView2Environment10 {
	var result *ICoreWebView2Environment10

	iidICoreWebView2Environment10 := NewGUID("{ee0eb9df-6f12-46ce-b53f-3f47b9c928e0}")
	_, _, _ = e.vtbl.QueryInterface.Call(
		uintptr(unsafe.Pointer(e)),
		uintptr(unsafe.Pointer(iidICoreWebView2Environment10)),
		uintptr(unsafe.Pointer(&result)))

	return result
}

// CreateCoreWebView2ControllerOptions returns options the caller must Release.
func (e *ICoreWebView2Environment10) CreateCoreWebView2ControllerOptions() (*ICoreWebView2ControllerOptions, error) {
	var options *ICoreWebView2ControllerOptions
	hr, _, _ := e.vtbl.CreateCoreWebView2ControllerOptions.Call(
		uintptr(unsafe.Pointer(e)),
		uintptr(unsafe.Pointer(&options)),
	)
	if windows.Handle(hr) != windows.S_OK {
		return nil, syscall.Errno(hr)
	}

	return options, nil
}

func (e *ICoreWebView2Environment10) CreateCoreWebView2ControllerWithOptions(parentWindow uintptr, options *ICoreWebView2ControllerOptions, handler *iCoreWebView2CreateCoreWebView2ControllerCompletedHandler) error {
	hr, _, _ := e.vtbl.CreateCoreWebView2ControllerWithOptions.Call(
		uintptr(unsafe.Pointer(e)),
		parentWindow,
		uintptr(unsafe.Pointer(options)),
		uintptr(unsafe.Pointer(handler)),
	)
	if windows.Handle(hr) != windows.S_OK {
		return syscall.Errno(hr)
	}

	return nil
}

func (e *ICoreWebView2Environment10) CreateCoreWebView2CompositionControllerWithOptions(parentWindow uintptr, options *ICoreWebView2ControllerOptions, handler *iCoreWebView2CreateCoreWebView2CompositionControllerCompletedHandler) error {
	hr, _, _ := e.vtbl.CreateCoreWebView2CompositionControllerWithOptions.Call(
		uintptr(unsafe.Pointer(e)),
		parentWindow,
		uintptr(unsafe.Pointer(options)),
		uintptr(unsafe.Pointer(handler)),
	)
	if windows.Handle(hr) != windows.S_OK {
		return syscall.Errno(hr)
	}

	return nil
}

type iCoreWebView2ControllerOptionsVtbl struct {
	_IUnknownVtbl
	GetProfileName            ComProc
	PutProfileName            ComProc
	GetIsInPrivateModeEnabled ComProc
	PutIsInPrivateModeEnabled ComProc
}

type ICoreWebView2ControllerOptions struct {
	vtbl *iCoreWebView2ControllerOptionsVtbl
}

func (o *ICoreWebView2ControllerOptions) Release() uintptr {
	ret, _, _ := o.vtbl.Release.Call(uintptr(unsafe.Pointer(o)))

	return ret
}

func (o *ICoreWebView2ControllerOptions) PutIsInPrivateModeEnabled(enabled bool) error {
	// BOOL is a 4-byte value parameter.
	var value uintptr
	if enabled {
		value = 1
	}
	hr, _, _ := o.vtbl.PutIsInPrivateModeEnabled.Call(
		uintptr(unsafe.Pointer(o)),
		value,
	)
	if windows.Handle(hr) != windows.S_OK {
		return syscall.Errno(hr)
	}

	return nil
}
