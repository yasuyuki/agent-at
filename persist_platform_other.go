//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const lifecycleLockName = ".agent-at-lifecycle.lock"

func persistentIdentity() (string, string, error) {
	return "", "", fmt.Errorf("persistent schedules are supported only on Windows")
}

// secureJobDir creates the directory and its lock entry before returning, so
// lockJob never has to create an entry in an existing job directory.
func secureJobDir(path string) error {
	if err := os.Mkdir(path, 0700); err != nil {
		return err
	}
	lockPath := filepath.Join(path, lifecycleLockName)
	f, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	return f.Close()
}

func lockJob(path string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(path, lifecycleLockName), os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
