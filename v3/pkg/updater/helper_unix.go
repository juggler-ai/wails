//go:build !windows

package updater

import (
	"os"
	"syscall"
)

// platformIsAlive reports whether pid names a running process. Sending the
// no-op signal 0 to a pid either succeeds (process is running and we have
// permission) or fails (process is gone / no permission).
func platformIsAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// replaceTarget removes the existing file or directory at target and renames
// newPath into its place. On Unix this is straightforward: open file handles
// remain valid against the unlinked inode, so we can delete a running binary
// and immediately put a new one at its path. macOS .app bundles are
// directories, hence RemoveAll rather than Remove.
//
// The payload is re-checked immediately before the target is destroyed. This
// duplicates the caller's check on purpose: the removal below is irreversible
// without the backup, and os.Rename cannot tell a populated directory from a
// hollow one, so this is the last point at which refusing costs nothing.
func replaceTarget(target, newPath string) error {
	if err := validatePayload(newPath); err != nil {
		return err
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.Rename(newPath, target)
}
