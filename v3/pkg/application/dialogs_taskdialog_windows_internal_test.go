//go:build windows

package application

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/w32"
)

// Custom buttons must get sequential IDs from TaskDialogFirstButtonID, map back
// to their originating Button, and carry over the default/cancel behaviour the
// caller requested. This is the case the legacy MessageBox path cannot render
// faithfully, since it can only show the fixed Yes/No/OK/Cancel captions.
func TestBuildTaskDialogConfigCustomButtons(t *testing.T) {
	confirm := &Button{Label: "Discard"}
	cancel := &Button{Label: "Cancel", IsDefault: true, IsCancel: true}
	opts := MessageDialogOptions{
		DialogType: QuestionDialogType,
		Title:      "Discard changes?",
		Message:    "Your unsaved edits will be lost.",
		Buttons:    []*Button{confirm, cancel},
	}

	cfg, byID := buildTaskDialogConfig(opts, false, 0)

	if cfg.WindowTitle != "Discard changes?" || cfg.MainInstruction != "Discard changes?" {
		t.Errorf("title not mapped: window=%q instruction=%q", cfg.WindowTitle, cfg.MainInstruction)
	}
	if cfg.Content != "Your unsaved edits will be lost." {
		t.Errorf("message not mapped to content: %q", cfg.Content)
	}
	if cfg.Flags&w32.TDF_ALLOW_DIALOG_CANCELLATION == 0 {
		t.Errorf("dialog cancellation (Esc/X) not enabled: flags=%#x", cfg.Flags)
	}
	if cfg.CommonButtons != 0 {
		t.Errorf("custom buttons present but common buttons set: %#x", cfg.CommonButtons)
	}
	if len(cfg.Buttons) != 2 {
		t.Fatalf("want 2 custom buttons, got %d", len(cfg.Buttons))
	}
	if cfg.Buttons[0].ID != w32.TaskDialogFirstButtonID || cfg.Buttons[1].ID != w32.TaskDialogFirstButtonID+1 {
		t.Errorf("button IDs not sequential from %d: %d, %d", w32.TaskDialogFirstButtonID, cfg.Buttons[0].ID, cfg.Buttons[1].ID)
	}
	if byID[cfg.Buttons[0].ID] != confirm || byID[cfg.Buttons[1].ID] != cancel {
		t.Errorf("byID does not map back to the originating buttons")
	}
	if cfg.DefaultButton != cfg.Buttons[1].ID {
		t.Errorf("default button = %d, want %d (the IsDefault button)", cfg.DefaultButton, cfg.Buttons[1].ID)
	}
}

// With no custom buttons, each dialog type falls back to its standard common
// buttons and icon.
func TestBuildTaskDialogConfigStandardTypes(t *testing.T) {
	cases := []struct {
		typ     DialogType
		buttons uint32
		icon    uintptr
	}{
		{InfoDialogType, w32.TDCBF_OK_BUTTON, w32.TD_INFORMATION_ICON},
		{WarningDialogType, w32.TDCBF_OK_BUTTON, w32.TD_WARNING_ICON},
		{ErrorDialogType, w32.TDCBF_OK_BUTTON, w32.TD_ERROR_ICON},
		{QuestionDialogType, w32.TDCBF_YES_BUTTON | w32.TDCBF_NO_BUTTON, 0},
	}
	for _, c := range cases {
		cfg, byID := buildTaskDialogConfig(MessageDialogOptions{DialogType: c.typ}, false, 0)
		if len(byID) != 0 || len(cfg.Buttons) != 0 {
			t.Errorf("type %d: unexpected custom buttons", c.typ)
		}
		if cfg.CommonButtons != c.buttons {
			t.Errorf("type %d: common buttons = %#x, want %#x", c.typ, cfg.CommonButtons, c.buttons)
		}
		if cfg.MainIcon != c.icon {
			t.Errorf("type %d: icon = %#x, want %#x", c.typ, cfg.MainIcon, c.icon)
		}
	}
}

// A caller-supplied app icon is used as an HICON (with TDF_USE_HICON_MAIN); when
// the icon fails to load (0, e.g. a dev binary) it degrades to a standard icon.
func TestBuildTaskDialogConfigAppIcon(t *testing.T) {
	opts := MessageDialogOptions{DialogType: InfoDialogType, Title: "About"}

	cfg, _ := buildTaskDialogConfig(opts, true, 0x1234)
	if cfg.MainIcon != 0x1234 || cfg.Flags&w32.TDF_USE_HICON_MAIN == 0 {
		t.Errorf("app icon not applied as HICON: icon=%#x flags=%#x", cfg.MainIcon, cfg.Flags)
	}

	cfg, _ = buildTaskDialogConfig(opts, true, 0)
	if cfg.MainIcon != w32.TD_INFORMATION_ICON || cfg.Flags&w32.TDF_USE_HICON_MAIN != 0 {
		t.Errorf("missing app icon did not fall back to a standard icon: icon=%#x flags=%#x", cfg.MainIcon, cfg.Flags)
	}
}

func TestCommonButtonLabel(t *testing.T) {
	cases := map[int32]string{
		w32.IDOK:     "Ok",
		w32.IDCANCEL: "Cancel",
		w32.IDYES:    "Yes",
		w32.IDNO:     "No",
		999:          "",
	}
	for id, want := range cases {
		if got := commonButtonLabel(id); got != want {
			t.Errorf("commonButtonLabel(%d) = %q, want %q", id, got, want)
		}
	}
}
