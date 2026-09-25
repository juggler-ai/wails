package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A staged .app bundle emptied while the helper waits for the parent to exit
// must not be installed. os.Rename succeeds on a directory whether or not it
// still holds anything, so without a check the helper renames the husk over a
// working application, reports success, and deletes the only copy of what it
// replaced. That is how a working install becomes a zero-byte one.
//
// helper_recovery_test.go covers newPath being REMOVED, which makes the rename
// fail and rolls back on its own. Emptying is the case a bundle actually hits,
// because the thing deleting it walks into the directory rather than unlinking
// it whole.
func TestRunHelperSwap_StagedBundleEmptiedDuringWait(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "App.app")
	newPath := filepath.Join(dir, "staging", "App.app")
	makeAppBundle(t, target, "old-bin")
	makeAppBundle(t, newPath, "new-bin")

	// Stand in for a concurrent discardStaging walking the staging directory
	// while the helper is blocked waiting for the parent.
	wait := func(int, time.Duration) error {
		return os.RemoveAll(filepath.Join(newPath, "Contents"))
	}

	launched := 0
	l := &funcLauncher{fn: func(path string) error {
		launched++
		if path != target {
			t.Errorf("relaunched %q, want the untouched original %q", path, target)
		}
		return nil
	}}

	code := runHelperSwap(target, newPath, 1234, filepath.Join(dir, "log"), wait, l)
	if code != 18 {
		t.Fatalf("code = %d, want 18 (payload not installable)", code)
	}
	if got := readFile(t, filepath.Join(target, "Contents", "MacOS", "exe")); string(got) != "old-bin" {
		t.Errorf("original application was damaged: %q", got)
	}
	if launched != 1 {
		t.Errorf("launches = %d, want the original relaunched exactly once", launched)
	}
}

// The same corruption arriving after the backup has been taken must be caught
// before the backup is discarded, and must leave the old application running.
func TestRunHelperSwap_InstalledBundleInvalid_RestoresBackup(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "App.app")
	newPath := filepath.Join(dir, "App.app.new")
	makeAppBundle(t, target, "old-bin")

	// A payload that passes the generic non-empty check but cannot be a working
	// bundle once it lands at a .app path: no Info.plist, no executable bit.
	writeFile(t, filepath.Join(newPath, "Contents", "MacOS", "exe"), []byte(""))
	writeFile(t, filepath.Join(newPath, "Contents", "junk"), []byte("x"))

	l := &funcLauncher{fn: func(string) error { return nil }}
	code := runHelperSwap(target, newPath, 0, filepath.Join(dir, "log"), instantWaiter, l)
	// 19 specifically. The payload passes the pre-swap check — it is a
	// directory holding a non-empty file, and its own path is not a .app — so
	// only once it has landed at a .app path do the missing Info.plist and the
	// zero-length executable make it a bundle that cannot launch. Any other
	// code here means the rejection came from somewhere other than the
	// post-swap check this test exists to cover.
	if code != 19 {
		t.Fatalf("code = %d, want 19 (rejected after the swap, backup restored)", code)
	}
	if got := readFile(t, filepath.Join(target, "Contents", "MacOS", "exe")); string(got) != "old-bin" {
		t.Errorf("original application not recovered: %q", got)
	}
}

func TestValidatePayload(t *testing.T) {
	dir := t.TempDir()

	t.Run("empty file rejected", func(t *testing.T) {
		p := filepath.Join(dir, "empty.bin")
		writeFile(t, p, nil)
		if err := validatePayload(p); err == nil {
			t.Fatal("an empty binary must not be installable")
		}
	})

	t.Run("non-empty file accepted", func(t *testing.T) {
		p := filepath.Join(dir, "ok.bin")
		writeFile(t, p, []byte("ELF"))
		if err := validatePayload(p); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})

	t.Run("empty directory rejected", func(t *testing.T) {
		p := filepath.Join(dir, "hollow")
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := validatePayload(p); err == nil {
			t.Fatal("an empty directory must not be installable")
		}
	})

	t.Run("directory of empty files rejected", func(t *testing.T) {
		p := filepath.Join(dir, "husk")
		writeFile(t, filepath.Join(p, "a"), nil)
		writeFile(t, filepath.Join(p, "b"), nil)
		if err := validatePayload(p); err == nil {
			t.Fatal("a directory holding nothing but empty files must not be installable")
		}
	})

	t.Run("bundle with zero-length binary rejected", func(t *testing.T) {
		p := filepath.Join(dir, "Zero.app")
		makeAppBundle(t, p, "real")
		exe := filepath.Join(p, "Contents", "MacOS", "exe")
		if err := os.Truncate(exe, 0); err != nil {
			t.Fatal(err)
		}
		err := validatePayload(p)
		if err == nil {
			t.Fatal("a bundle whose only executable is zero-length must not be installable")
		}
		if !strings.Contains(err.Error(), "MacOS") {
			t.Errorf("error should name the missing executable, got: %v", err)
		}
	})

	t.Run("bundle without Info.plist rejected", func(t *testing.T) {
		p := filepath.Join(dir, "NoPlist.app")
		makeAppBundle(t, p, "real")
		if err := os.Remove(filepath.Join(p, "Contents", "Info.plist")); err != nil {
			t.Fatal(err)
		}
		if err := validatePayload(p); err == nil {
			t.Fatal("a bundle without an Info.plist must not be installable")
		}
	})

	t.Run("well-formed bundle accepted", func(t *testing.T) {
		p := filepath.Join(dir, "Good.app")
		makeAppBundle(t, p, "real")
		if err := validatePayload(p); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})
}

// An unsigned bundle is unverifiable, not corrupt: a build made without signing
// credentials must still be able to update itself.
func TestVerifySignature_UnsignedBundleIsNotAFailure(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Unsigned.app")
	makeAppBundle(t, p, "real")

	note, err := verifySignature(p)
	if err != nil {
		t.Fatalf("an unsigned bundle must not be treated as corrupt: %v", err)
	}
	t.Logf("note: %q", note)
}

func TestIsUnsigned(t *testing.T) {
	if !isUnsigned([]byte("/x/Foo.app: code object is not signed at all")) {
		t.Error("codesign's unsigned verdict must be recognised")
	}
	if isUnsigned([]byte("/x/Foo.app: a sealed resource is missing or invalid")) {
		t.Error("a genuine rejection must not be read as unsigned")
	}
}
