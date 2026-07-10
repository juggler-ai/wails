//go:build windows

package w32

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"unsafe"
)

// procTaskDialogIndirect is the modern (comctl32 v6, Vista+) themed dialog
// primitive. It is only present when the process runs with the
// Microsoft.Windows.Common-Controls v6 side-by-side assembly activated (the
// app manifest). When it is absent, Find() fails and callers fall back to the
// legacy MessageBox path — so this is always safe to attempt.
var procTaskDialogIndirect = modcomctl32.NewProc("TaskDialogIndirect")

// TaskDialog common-button flags (TDCBF_*). Used when a dialog has no custom
// buttons and should show the standard set for its type.
const (
	TDCBF_OK_BUTTON     = 0x0001
	TDCBF_YES_BUTTON    = 0x0002
	TDCBF_NO_BUTTON     = 0x0004
	TDCBF_CANCEL_BUTTON = 0x0008
	TDCBF_RETRY_BUTTON  = 0x0010
	TDCBF_CLOSE_BUTTON  = 0x0020
)

// TaskDialog behaviour flags (TDF_*). Only the ones we use are defined.
const (
	TDF_USE_HICON_MAIN              = 0x0002
	TDF_ALLOW_DIALOG_CANCELLATION   = 0x0008
	TDF_POSITION_RELATIVE_TO_WINDOW = 0x1000
)

// Standard TaskDialog icons, expressed as the MAKEINTRESOURCE values the API
// expects in the pszMainIcon field: MAKEINTRESOURCE(-1..-4) == 0xFFFF..0xFFFC.
const (
	TD_WARNING_ICON     = uintptr(0xFFFF) // MAKEINTRESOURCE(-1)
	TD_ERROR_ICON       = uintptr(0xFFFE) // MAKEINTRESOURCE(-2)
	TD_INFORMATION_ICON = uintptr(0xFFFD) // MAKEINTRESOURCE(-3)
	TD_SHIELD_ICON      = uintptr(0xFFFC) // MAKEINTRESOURCE(-4)
)

// TaskDialogFirstButtonID is the first ID assigned to a custom button. It sits
// above every standard ID* constant (IDOK..IDCONTINUE) so a custom-button
// result can never be confused with a common-button or cancel (X/Esc) result.
const TaskDialogFirstButtonID = 100

// TaskDialogButton is a single custom button: a caption and the ID reported
// back when it is clicked.
type TaskDialogButton struct {
	ID   int32
	Text string
}

// TaskDialogConfig is a high-level, Go-native description of a task dialog. The
// packed Win32 TASKDIALOGCONFIG layout is an implementation detail handled
// entirely inside TaskDialogIndirect.
type TaskDialogConfig struct {
	Parent          HWND
	Instance        HINSTANCE
	Flags           uint32
	CommonButtons   uint32
	WindowTitle     string
	MainIcon        uintptr // a TD_*_ICON value, or an HICON when TDF_USE_HICON_MAIN is set
	MainInstruction string
	Content         string
	Buttons         []TaskDialogButton
	DefaultButton   int32
}

// TaskDialogIndirect shows a themed task dialog and returns the ID of the
// clicked button (a custom ID, a standard ID* for a common button, or IDCANCEL
// when dismissed via Esc/the title-bar close box). It returns an error — rather
// than panicking or aborting — when the API is unavailable or the call fails,
// so callers can fall back to a legacy MessageBox.
//
// TASKDIALOGCONFIG and TASKDIALOG_BUTTON are declared #pragma pack(1) in
// commctrl.h, so they contain no inter-field padding. A natural Go struct would
// insert alignment padding before every pointer on 64-bit and corrupt the
// layout, so both are serialized here by hand into byte buffers with fields
// written at their exact packed offsets. Pointer-sized fields are written at
// the native width (4 or 8 bytes), which keeps the layout correct on 386 as
// well as amd64/arm64.
func TaskDialogIndirect(cfg *TaskDialogConfig) (int32, error) {
	if err := procTaskDialogIndirect.Find(); err != nil {
		return 0, err
	}

	ptrSize := int(unsafe.Sizeof(uintptr(0)))

	// keep holds every heap allocation whose address we bury inside a byte
	// buffer. The GC does not scan []byte for pointers, so without this the
	// UTF-16 strings and the button buffer could be collected mid-call.
	keep := make([]any, 0, len(cfg.Buttons)+4)

	utf16 := func(s string) uintptr {
		if s == "" {
			return 0
		}
		p := MustStringToUTF16Ptr(s)
		keep = append(keep, p)
		return uintptr(unsafe.Pointer(p))
	}

	// Serialize the custom-button array (packed: int32 ID + native pointer).
	var buttonsPtr uintptr
	if len(cfg.Buttons) > 0 {
		btnBuf := make([]byte, 0, len(cfg.Buttons)*(4+ptrSize))
		for _, b := range cfg.Buttons {
			var idb [4]byte
			binary.LittleEndian.PutUint32(idb[:], uint32(b.ID))
			btnBuf = append(btnBuf, idb[:]...)
			btnBuf = appendPtr(btnBuf, utf16(b.Text), ptrSize)
		}
		keep = append(keep, btnBuf)
		buttonsPtr = uintptr(unsafe.Pointer(&btnBuf[0]))
	}

	// Serialize TASKDIALOGCONFIG field-by-field in declaration order.
	var buf []byte
	putU32 := func(v uint32) {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], v)
		buf = append(buf, b[:]...)
	}
	putPtr := func(v uintptr) { buf = appendPtr(buf, v, ptrSize) }

	putU32(0) // cbSize — patched below once the full size is known
	putPtr(uintptr(cfg.Parent))
	putPtr(uintptr(cfg.Instance))
	putU32(cfg.Flags)
	putU32(cfg.CommonButtons)
	putPtr(utf16(cfg.WindowTitle))
	putPtr(cfg.MainIcon)
	putPtr(utf16(cfg.MainInstruction))
	putPtr(utf16(cfg.Content))
	putU32(uint32(len(cfg.Buttons)))
	putPtr(buttonsPtr)
	putU32(uint32(cfg.DefaultButton))
	putU32(0) // cRadioButtons
	putPtr(0) // pRadioButtons
	putU32(0) // nDefaultRadioButton
	putPtr(0) // pszVerificationText
	putPtr(0) // pszExpandedInformation
	putPtr(0) // pszExpandedControlText
	putPtr(0) // pszCollapsedControlText
	putPtr(0) // hFooterIcon
	putPtr(0) // pszFooter
	putPtr(0) // pfCallback
	putPtr(0) // lpCallbackData (LONG_PTR, pointer-sized)
	putU32(0) // cxWidth

	binary.LittleEndian.PutUint32(buf[0:4], uint32(len(buf))) // cbSize

	var clicked int32
	hr, _, _ := procTaskDialogIndirect.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&clicked)),
		0, // pnRadioButton
		0, // pfVerificationFlagChecked
	)
	runtime.KeepAlive(keep)
	runtime.KeepAlive(buf)

	if HRESULT(hr) != 0 { // S_OK == 0
		return 0, fmt.Errorf("TaskDialogIndirect failed: hr=0x%08x", uint32(hr))
	}
	return clicked, nil
}

// appendPtr appends v to buf as a little-endian, native-pointer-width value.
func appendPtr(buf []byte, v uintptr, ptrSize int) []byte {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(v))
	return append(buf, b[:ptrSize]...)
}
