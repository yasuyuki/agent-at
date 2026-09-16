//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

// lockJobForExecution waits for registration or removal to finish.  A timer
// can fire immediately after enable; treating that ordinary race as a failed
// run would lose the one scheduled execution.
func lockJobForExecution(path string) (func(), error) {
	if err := verifyJobLockForExecution(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(path, lifecycleLockName), os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
