//go:build darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

const lifecycleLockName = ".agent-at-lifecycle.lock"

func darwinHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !filepath.IsAbs(home) {
		return "", fmt.Errorf("resolve HOME: %w", err)
	}
	return filepath.Clean(home), nil
}

func darwinUID() string { return strconv.Itoa(os.Geteuid()) }

func persistentIdentity() (string, string, error) {
	home, err := darwinHome()
	if err != nil {
		return "", "", err
	}
	if err := ensureDarwinOwnedDir(home); err != nil {
		return "", "", err
	}
	library := filepath.Join(home, "Library")
	if err := ensureDarwinOwnedDir(library); err != nil {
		return "", "", err
	}
	applicationSupport := filepath.Join(library, "Application Support")
	if err := ensureDarwinOwnedDir(applicationSupport); err != nil {
		return "", "", err
	}
	root := filepath.Join(applicationSupport, "agent-at", "jobs")
	for _, path := range []string{filepath.Join(applicationSupport, "agent-at"), root} {
		if err := ensureDarwinPrivateDir(path); err != nil {
			return "", "", err
		}
	}
	return root, darwinUID(), nil
}

func darwinOwned(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Geteuid()
}

func ensureDarwinOwnedDir(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err = os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !darwinOwned(info) || info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("unsafe LaunchAgent parent directory: %s", path)
	}
	return nil
}

func ensureDarwinPrivateDir(path string) error {
	if err := ensureDarwinOwnedDir(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode().Perm() != 0700 {
		return fmt.Errorf("unsafe private directory: %s", path)
	}
	return nil
}

func secureJobDir(path string) error {
	if err := os.Mkdir(path, 0700); err != nil {
		return err
	}
	if err := verifyDarwinPrivateDir(path); err != nil {
		_ = os.Remove(path)
		return err
	}
	lock := filepath.Join(path, lifecycleLockName)
	f, err := os.OpenFile(lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	if err = f.Close(); err != nil {
		_ = os.Remove(lock)
		_ = os.Remove(path)
		return err
	}
	return verifyDarwinPrivateFile(lock)
}

func verifyDarwinPrivateDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !darwinOwned(info) || info.Mode().Perm() != 0700 {
		return fmt.Errorf("unsafe job directory: %s", path)
	}
	return nil
}

func verifyDarwinPrivateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !darwinOwned(info) || info.Mode().Perm() != 0600 {
		return fmt.Errorf("unsafe private file: %s", path)
	}
	return nil
}

func lockJob(path string) (func(), error) {
	if err := verifyDarwinPrivateDir(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(path, lifecycleLockName), os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}

func verifyJobLockForExecution(path string) error {
	if err := verifyDarwinPrivateDir(path); err != nil {
		return err
	}
	return verifyDarwinPrivateFile(filepath.Join(path, lifecycleLockName))
}
