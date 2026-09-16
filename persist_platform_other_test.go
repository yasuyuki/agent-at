//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecureJobDirAndLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "job")
	if err := secureJobDir(path); err != nil {
		t.Fatal(err)
	}
	if err := secureJobDir(path); err == nil {
		t.Fatal("secureJobDir accepted an existing directory")
	}
	if _, err := os.Stat(filepath.Join(path, lifecycleLockName)); err != nil {
		t.Fatalf("lifecycle lock missing: %v", err)
	}
	release, err := lockJob(path)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := lockJob(path); err == nil {
		t.Fatal("second lockJob acquired an exclusive lock")
	}
}
