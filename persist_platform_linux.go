//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

const lifecycleLockName = ".agent-at-lifecycle.lock"

func linuxHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve HOME: %w", err)
	}
	if home == "" || !filepath.IsAbs(home) {
		return "", fmt.Errorf("resolve HOME: not an absolute path")
	}
	return filepath.Clean(home), nil
}

func linuxUID() string { return strconv.Itoa(os.Geteuid()) }

func persistentIdentity() (string, string, error) {
	home, err := linuxHome()
	if err != nil {
		return "", "", err
	}
	root := filepath.Join(home, ".local", "state", "agent-at", "jobs")
	for _, path := range []string{filepath.Join(home, ".local"), filepath.Join(home, ".local", "state")} {
		if err := ensureLinuxAncestorDir(path); err != nil {
			return "", "", err
		}
	}
	for _, path := range []string{filepath.Join(home, ".local", "state", "agent-at"), root} {
		if err := ensureLinuxPrivateDir(path); err != nil {
			return "", "", err
		}
	}
	return root, linuxUID(), nil
}

func linuxOwned(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Geteuid()
}

func ensureLinuxPrivateDir(path string) error {
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
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !linuxOwned(info) || info.Mode().Perm() != 0700 {
		return fmt.Errorf("unsafe private directory: %s", path)
	}
	return nil
}

// Existing ~/.local is often 0755.  It need not be secret, but it must be
// owned by this UID, non-symlinked, and not writable by another user before a
// 0700 descendant below it becomes a safe private store.
func ensureLinuxAncestorDir(path string) error {
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
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !linuxOwned(info) || info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("unsafe private-store ancestor: %s", path)
	}
	return nil
}

func ensureLinuxOwnedDir(path string) error {
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
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !linuxOwned(info) || info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("unsafe systemd directory: %s", path)
	}
	return nil
}

func secureJobDir(path string) error {
	if err := os.Mkdir(path, 0700); err != nil {
		return err
	}
	if err := verifyLinuxPrivateDir(path); err != nil {
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
	return verifyLinuxPrivateFile(lock)
}

func verifyLinuxPrivateDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !linuxOwned(info) || info.Mode().Perm() != 0700 {
		return fmt.Errorf("unsafe job directory: %s", path)
	}
	return nil
}

func verifyLinuxPrivateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !linuxOwned(info) || info.Mode().Perm() != 0600 {
		return fmt.Errorf("unsafe private file: %s", path)
	}
	return nil
}

func lockJob(path string) (func(), error) {
	if err := verifyLinuxPrivateDir(path); err != nil {
		return nil, err
	}
	lock := filepath.Join(path, lifecycleLockName)
	if err := verifyLinuxPrivateFile(lock); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lock, os.O_RDWR, 0)
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
	if err := verifyLinuxPrivateDir(path); err != nil {
		return err
	}
	return verifyLinuxPrivateFile(filepath.Join(path, lifecycleLockName))
}
