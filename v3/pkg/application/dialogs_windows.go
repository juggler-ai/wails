//go:build windows && !server

package application

import (
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/internal/go-common-file-dialog/cfd"
	"github.com/wailsapp/wails/v3/pkg/w32"
	"golang.org/x/sys/windows"
)

func (m *windowsApp) showAboutDialog(title string, message string, _ []byte) {
	about := newDialogImpl(&MessageDialog{
		MessageDialogOptions: MessageDialogOptions{
			DialogType: InfoDialogType,
			Title:      title,
			Message:    message,
		},
	})
	about.UseAppIcon = true
	about.show()
}

type windowsDialog struct {
	dialog *MessageDialog

	//dialogImpl unsafe.Pointer
	UseAppIcon bool
}

func (m *windowsDialog) show() {
	// Prefer the modern, themed TaskDialog. It falls back to the legacy
	// MessageBox on any failure (e.g. comctl32 v6 unavailable), so the classic
	// path below always remains as a safety net.
	if m.showModern() {
		return
	}
	m.showLegacy()
}

// showModern renders the dialog with the themed comctl32 v6 TaskDialog. It
// reports whether it handled the dialog; false means the caller should fall
// back to showLegacy. Unlike MessageBox, TaskDialog honours the dialog's own
// custom button captions, so a Question dialog's buttons match the labels the
// caller registered OnClick handlers against.
func (m *windowsDialog) showModern() bool {
	var parent uintptr
	if m.dialog.window != nil {
		if nativeWindow := m.dialog.window.NativeWindow(); nativeWindow != nil {
			parent = uintptr(nativeWindow)
		}
	}

	// The app icon lives at resource ID 3 (the same one MessageBoxWithIcon
	// uses). Dev binaries have no embedded resource, so this may be 0 — in which
	// case buildTaskDialogConfig falls back to a standard icon.
	var appIcon uintptr
	if m.UseAppIcon || m.dialog.Icon != nil {
		appIcon = uintptr(w32.LoadIconWithResourceID(w32.GetApplicationHandle(), 3))
	}

	cfg, byID := buildTaskDialogConfig(m.dialog.MessageDialogOptions, m.UseAppIcon, appIcon)
	cfg.Parent = w32.HWND(parent)
	cfg.Instance = w32.GetApplicationHandle()
	if parent != 0 {
		cfg.Flags |= w32.TDF_POSITION_RELATIVE_TO_WINDOW
	}

	clicked, err := w32.TaskDialogIndirect(&cfg)
	if err != nil {
		return false
	}

	// A custom button: fire its handler directly.
	if button, ok := byID[clicked]; ok {
		if button.Callback != nil {
			button.Callback()
		}
		return true
	}
	// Esc or the title-bar close box reports IDCANCEL: honour the button the
	// caller marked as the cancel action, if any.
	if clicked == w32.IDCANCEL {
		for _, button := range m.dialog.Buttons {
			if button.IsCancel {
				if button.Callback != nil {
					button.Callback()
				}
				break
			}
		}
		return true
	}
	// A common button (OK/Yes/No/Cancel): match a registered button by its
	// conventional label, mirroring the legacy MessageBox mapping.
	if label := commonButtonLabel(clicked); label != "" {
		for _, button := range m.dialog.Buttons {
			if button.Label == label {
				if button.Callback != nil {
					button.Callback()
				}
				break
			}
		}
	}
	return true
}

// buildTaskDialogConfig translates a MessageDialog into a w32.TaskDialogConfig
// and returns a map from each assigned custom-button ID back to the originating
// Button, so the caller can invoke the correct OnClick handler. It performs no
// Win32 calls, so it is unit-testable without a display.
func buildTaskDialogConfig(opts MessageDialogOptions, useAppIcon bool, appIcon uintptr) (w32.TaskDialogConfig, map[int32]*Button) {
	cfg := w32.TaskDialogConfig{
		WindowTitle:     opts.Title,
		MainInstruction: opts.Title, // shown as the bold heading
		Content:         opts.Message,
		Flags:           w32.TDF_ALLOW_DIALOG_CANCELLATION,
	}

	switch {
	case useAppIcon || opts.Icon != nil:
		if appIcon != 0 {
			cfg.MainIcon = appIcon
			cfg.Flags |= w32.TDF_USE_HICON_MAIN
		} else {
			cfg.MainIcon = w32.TD_INFORMATION_ICON
		}
	default:
		switch opts.DialogType {
		case InfoDialogType:
			cfg.MainIcon = w32.TD_INFORMATION_ICON
		case WarningDialogType:
			cfg.MainIcon = w32.TD_WARNING_ICON
		case ErrorDialogType:
			cfg.MainIcon = w32.TD_ERROR_ICON
		case QuestionDialogType:
			// Modern task dialogs have no standard question icon; leave it blank.
		}
	}

	byID := map[int32]*Button{}
	var custom []*Button
	for _, button := range opts.Buttons {
		if strings.TrimSpace(button.Label) != "" {
			custom = append(custom, button)
		}
	}
	if len(custom) > 0 {
		id := int32(w32.TaskDialogFirstButtonID)
		for _, button := range custom {
			cfg.Buttons = append(cfg.Buttons, w32.TaskDialogButton{ID: id, Text: button.Label})
			byID[id] = button
			if button.IsDefault {
				cfg.DefaultButton = id
			}
			id++
		}
	} else {
		switch opts.DialogType {
		case QuestionDialogType:
			cfg.CommonButtons = w32.TDCBF_YES_BUTTON | w32.TDCBF_NO_BUTTON
		default:
			cfg.CommonButtons = w32.TDCBF_OK_BUTTON
		}
	}
	return cfg, byID
}

// commonButtonLabel maps a standard button result to the label string the
// legacy path uses, so a caller that registered OnClick against "Yes"/"No"/etc.
// still receives its callback when a common button is clicked.
func commonButtonLabel(id int32) string {
	switch id {
	case w32.IDOK:
		return "Ok"
	case w32.IDCANCEL:
		return "Cancel"
	case w32.IDYES:
		return "Yes"
	case w32.IDNO:
		return "No"
	}
	return ""
}

func (m *windowsDialog) showLegacy() {

	title := w32.MustStringToUTF16Ptr(m.dialog.Title)
	message := w32.MustStringToUTF16Ptr(m.dialog.Message)
	flags := calculateMessageDialogFlags(m.dialog.MessageDialogOptions)
	var button int32
	var err error

	var parentWindow uintptr
	if m.dialog.window != nil {
		nativeWindow := m.dialog.window.NativeWindow()
		if nativeWindow != nil {
			parentWindow = uintptr(nativeWindow)
		}
	}

	if m.UseAppIcon || m.dialog.Icon != nil {
		// Use the application's embedded icon resource (ID 3). MessageBoxIndirect
		// cannot render arbitrary icon bytes, so a custom Icon also maps to the
		// app icon — the closest supported behaviour (full custom-icon support
		// needs a TaskDialog implementation).
		//
		button, err = w32.MessageBoxWithIcon(parentWindow, message, title, 3, messageDialogUserIconFlags(flags))
		if err != nil {
			// Dev binaries (`go run`, `wails3 dev`) have no embedded icon
			// resource, which makes MessageBoxIndirect fail outright — the
			// dialog never appeared and the app aborted via fatal error
			// (#4233). Fall back to a standard dialog instead.
			button, err = windows.MessageBox(windows.HWND(parentWindow), message, title, flags|windows.MB_SYSTEMMODAL)
		}
		if err != nil {
			globalApplication.handleFatalError(err)
		}
	} else {
		button, err = windows.MessageBox(windows.HWND(parentWindow), message, title, flags|windows.MB_SYSTEMMODAL)
		if err != nil {
			globalApplication.handleFatalError(err)
		}
	}
	// This maps MessageBox return values to strings
	responses := []string{"", "Ok", "Cancel", "Abort", "Retry", "Ignore", "Yes", "No", "", "", "Try Again", "Continue"}
	result := "Error"
	if int(button) < len(responses) {
		result = responses[button]
	}
	// Check if there's a callback for the button pressed
	for _, buttonInDialog := range m.dialog.Buttons {
		if buttonInDialog.Label == result {
			if buttonInDialog.Callback != nil {
				buttonInDialog.Callback()
			}
		}
	}
}

func newDialogImpl(d *MessageDialog) *windowsDialog {
	return &windowsDialog{
		dialog: d,
	}
}

type windowOpenFileDialog struct {
	dialog *OpenFileDialogStruct
}

func newOpenFileDialogImpl(d *OpenFileDialogStruct) *windowOpenFileDialog {
	return &windowOpenFileDialog{
		dialog: d,
	}
}

func getDefaultFolder(folder string) (string, error) {
	if folder == "" {
		return "", nil
	}
	return filepath.Abs(folder)
}

func (m *windowOpenFileDialog) show() (chan string, error) {

	defaultFolder, err := getDefaultFolder(m.dialog.directory)
	if err != nil {
		return nil, err
	}

	config := cfd.DialogConfig{
		Title:       m.dialog.title,
		Role:        "PickFolder",
		FileFilters: convertFilters(m.dialog.filters),
		Folder:      defaultFolder,
	}

	var result []string
	if m.dialog.allowsMultipleSelection && !m.dialog.canChooseDirectories {
		temp, err := showCfdDialog(
			func() (cfd.Dialog, error) {
				return cfd.NewOpenMultipleFilesDialog(config)
			}, true, m.dialog.window)
		if err != nil {
			return nil, err
		}
		result = temp.([]string)
	} else {
		if m.dialog.canChooseDirectories {
			temp, err := showCfdDialog(
				func() (cfd.Dialog, error) {
					return cfd.NewSelectFolderDialog(config)
				}, false, m.dialog.window)
			if err != nil {
				return nil, err
			}
			result = []string{temp.(string)}
		} else {
			temp, err := showCfdDialog(
				func() (cfd.Dialog, error) {
					return cfd.NewOpenFileDialog(config)
				}, false, m.dialog.window)
			if err != nil {
				return nil, err
			}
			result = []string{temp.(string)}
		}
	}

	files := make(chan string)
	go func() {
		defer handlePanic()
		for _, file := range result {
			files <- file
		}
		close(files)
	}()
	return files, nil
}

type windowSaveFileDialog struct {
	dialog *SaveFileDialogStruct
}

func newSaveFileDialogImpl(d *SaveFileDialogStruct) *windowSaveFileDialog {
	return &windowSaveFileDialog{
		dialog: d,
	}
}

func (m *windowSaveFileDialog) show() (chan string, error) {
	files := make(chan string)
	defaultFolder, err := getDefaultFolder(m.dialog.directory)
	if err != nil {
		close(files)
		return files, err
	}

	config := cfd.DialogConfig{
		Title:       m.dialog.title,
		Role:        "SaveFile",
		FileFilters: convertFilters(m.dialog.filters),
		FileName:    m.dialog.filename,
		Folder:      defaultFolder,
	}

	// Original PR for v2 by @almas1992: https://github.com/wailsapp/wails/pull/3205
	if len(m.dialog.filters) > 0 {
		config.DefaultExtension = strings.TrimPrefix(strings.Split(m.dialog.filters[0].Pattern, ";")[0], "*")
	}

	result, err := showCfdDialog(
		func() (cfd.Dialog, error) {
			return cfd.NewSaveFileDialog(config)
		}, false, m.dialog.window)
	if err != nil {
		close(files)
		return files, err
	}
	go func() {
		defer handlePanic()
		f, ok := result.(string)
		if ok {
			files <- f
		}
		close(files)
	}()
	return files, err
}

// messageDialogUserIconFlags converts standard message dialog flags for use
// with MB_USERICON. The user icon replaces the standard one, so the MB_ICON*
// bits are stripped — but the button configuration is preserved: forcing
// MB_OK here (the old behaviour) silently destroyed Yes/No buttons on
// question dialogs shown with an icon (#4233).
func messageDialogUserIconFlags(flags uint32) uint32 {
	const mbIconMask = w32.MB_ICONHAND | w32.MB_ICONQUESTION | w32.MB_ICONASTERISK | w32.MB_USERICON
	return (flags &^ uint32(mbIconMask)) | windows.MB_USERICON | windows.MB_SYSTEMMODAL
}

func calculateMessageDialogFlags(options MessageDialogOptions) uint32 {
	var flags uint32

	switch options.DialogType {
	case InfoDialogType:
		flags = windows.MB_OK | windows.MB_ICONINFORMATION
	case ErrorDialogType:
		flags = windows.MB_ICONERROR | windows.MB_OK
	case QuestionDialogType:
		flags = windows.MB_YESNO
		for _, button := range options.Buttons {
			if strings.TrimSpace(strings.ToLower(button.Label)) == "no" && button.IsDefault {
				flags |= windows.MB_DEFBUTTON2
			}
		}
	case WarningDialogType:
		flags = windows.MB_OK | windows.MB_ICONWARNING
	}

	return flags
}

func convertFilters(filters []FileFilter) []cfd.FileFilter {
	var result []cfd.FileFilter
	for _, filter := range filters {
		result = append(result, cfd.FileFilter(filter))
	}
	return result
}

func showCfdDialog(newDlg func() (cfd.Dialog, error), isMultiSelect bool, parentWindow Window) (any, error) {
	dlg, err := newDlg()
	if err != nil {
		return nil, err
	}

	// Set parent window if provided
	if parentWindow != nil {
		nativeWindow := parentWindow.NativeWindow()
		if nativeWindow != nil {
			dlg.SetParentWindowHandle(uintptr(nativeWindow))
		}
	}

	defer func() {
		err := dlg.Release()
		if err != nil {
			globalApplication.error("unable to release dialog: %w", err)
		}
	}()

	if multi, _ := dlg.(cfd.OpenMultipleFilesDialog); multi != nil && isMultiSelect {
		paths, err := multi.ShowAndGetResults()
		if err != nil {
			return nil, err
		}

		for i, path := range paths {
			paths[i] = filepath.Clean(path)
		}
		return paths, nil
	}

	path, err := dlg.ShowAndGetResult()
	if err != nil {
		return nil, err
	}
	return filepath.Clean(path), nil
}
