package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Payload validation.
//
// The download is verified by digest while it streams (see download.go), but
// that digest covers the ARCHIVE, which extract.go deletes as soon as it has
// unpacked it. Nothing downstream of that point — the extracted payload
// sitting in a temp directory, and the bundle once it has been renamed into
// place — is covered by it.
//
// That gap is load-bearing rather than theoretical. A staged .app bundle is a
// DIRECTORY, and os.Rename of a directory succeeds whether or not it still has
// anything in it, so a payload emptied between staging and the swap would
// install cleanly and report success. These checks close it at both ends:
// refuse to swap in a payload that cannot be an application, and refuse to
// throw away the rollback until the thing now on disk looks like one.

// codesignTimeout bounds the macOS signature check. It reads every sealed file
// in the bundle, so it is not instant on a large one; a bound means a wedged
// codesign degrades to "unverified" rather than hanging the swap forever.
const codesignTimeout = 90 * time.Second

// validatePayload reports whether path is something worth installing. It is
// deliberately cheap and structural: it answers "could this possibly be an
// application" rather than "is this the right application", which is the
// digest's job and already done by the time we get here.
//
// A regular file must be non-empty. A directory must contain at least one
// non-empty regular file — that alone rejects the emptied-bundle case. A
// directory that looks like a macOS .app is held to the bundle layout too.
func validatePayload(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("staged payload unreadable: %w", err)
	}

	if !info.IsDir() {
		if info.Size() == 0 {
			return fmt.Errorf("staged payload %s is empty", filepath.Base(path))
		}
		return nil
	}

	if filepath.Ext(path) == ".app" {
		if err := validateAppBundle(path); err != nil {
			return err
		}
	}

	found, err := hasContent(path)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("staged payload %s is an empty directory", filepath.Base(path))
	}
	return nil
}

// hasContent reports whether root holds at least one regular file of non-zero
// length. It stops at the first one rather than walking the whole tree, so the
// cost is a few directory reads and not a stat of every file in the bundle.
func hasContent(root string) (bool, error) {
	found := false
	err := filepath.Walk(root, func(_ string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.Mode().IsRegular() && fi.Size() > 0 {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("staged payload unreadable: %w", err)
	}
	return found, nil
}

// validateAppBundle checks the parts of a macOS .app without which the bundle
// cannot launch: an Info.plist with something in it, and a Contents/MacOS
// holding at least one non-empty executable. A zero-length main binary is
// exactly the shape a partially-deleted bundle takes, and is the failure this
// is here to catch.
//
// The layout is checked on every platform. A .app is a .app wherever it was
// unpacked, and a build that cross-stages one should not skip the check for
// want of a matching GOOS.
func validateAppBundle(bundle string) error {
	plist := filepath.Join(bundle, "Contents", "Info.plist")
	switch fi, err := os.Stat(plist); {
	case err != nil:
		return fmt.Errorf("bundle has no readable Contents/Info.plist: %w", err)
	case fi.Size() == 0:
		return fmt.Errorf("bundle Contents/Info.plist is empty")
	}

	macOS := filepath.Join(bundle, "Contents", "MacOS")
	entries, err := os.ReadDir(macOS)
	if err != nil {
		return fmt.Errorf("bundle has no readable Contents/MacOS: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			return fmt.Errorf("bundle executable unreadable: %w", err)
		}
		if fi.Mode().IsRegular() && fi.Size() > 0 && fi.Mode().Perm()&0o111 != 0 {
			return nil
		}
	}
	return fmt.Errorf("bundle Contents/MacOS holds no non-empty executable")
}

// validateInstalled runs the post-swap checks over whatever now sits at path:
// the structural check the payload already passed (cheap, and it catches a
// swap that moved the wrong thing), then the platform signature check.
//
// A signature that could not be run is logged and tolerated; one that ran and
// returned a verdict is decisive, and the caller still holds the backup.
func validateInstalled(path string, lg *helperLog) error {
	if err := validatePayload(path); err != nil {
		return err
	}
	note, err := verifySignature(path)
	if err != nil {
		return err
	}
	if note != "" {
		lg.logf("%s", note)
	}
	return nil
}

// verifySignature runs the platform's own integrity check over an installed
// payload, and is the per-file checksum the archive digest stopped providing
// the moment extract.go deleted the archive: codesign validates every sealed
// file against the hashes in the bundle's CodeDirectory, so a truncated or
// substituted binary fails it.
//
// The failure modes are kept apart deliberately. A codesign that RAN and
// rejected a signed bundle is a real verdict and returns an error. Anything
// that merely means "no opinion available" returns nil with a note for the
// log, because refusing to install on those terms would strand people whose
// situation is unusual rather than broken:
//
//   - codesign missing, or timed out;
//   - the payload is not a macOS bundle at all;
//   - the bundle carries no signature. A build made locally, or by anyone
//     without signing credentials, is unsigned by construction. Treating that
//     as corruption would mean such a build could never update itself.
func verifySignature(path string) (note string, err error) {
	if runtime.GOOS != "darwin" || filepath.Ext(path) != ".app" {
		return "", nil
	}
	tool, lookErr := exec.LookPath("codesign")
	if lookErr != nil {
		return "codesign unavailable, bundle signature not checked", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), codesignTimeout)
	defer cancel()

	out, runErr := exec.CommandContext(ctx, tool, "--verify", "--deep", "--strict", path).CombinedOutput()
	if ctx.Err() != nil {
		return "codesign timed out, bundle signature not checked", nil
	}
	if runErr == nil {
		return "", nil
	}
	if isUnsigned(out) {
		return "bundle carries no signature, integrity not checked", nil
	}
	return "", fmt.Errorf("bundle failed signature verification: %s", firstLine(out))
}

// isUnsigned distinguishes codesign's "there is nothing here to check" from a
// genuine rejection. The wording has been stable for many releases, and the
// consequence of misreading it in either direction is mild: an unsigned bundle
// wrongly called signed simply fails verification, and a corrupt bundle wrongly
// called unsigned still has to pass the structural check.
func isUnsigned(out []byte) bool {
	return strings.Contains(string(out), "not signed at all")
}

// firstLine trims codesign's output down to the line that says what is wrong,
// so a rejection reads as one line in the helper log rather than a page.
func firstLine(b []byte) string {
	s := string(b)
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	if s == "" {
		return "no output"
	}
	return s
}
