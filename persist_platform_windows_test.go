//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentIdentityCreatesProtectedRoot(t *testing.T) {
	root, sid, err := persistentIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if sid == "" || root == "" {
		t.Fatalf("identity = %q, %q", root, sid)
	}
	if err := validateProtectedDirectory(root, sid); err != nil {
		t.Fatal(err)
	}
}

func TestSecureJobDirAndLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "job")
	if err := secureJobDir(path); err != nil {
		t.Fatal(err)
	}
	if err := secureJobDir(path); err == nil {
		t.Fatal("secureJobDir accepted existing directory")
	}
	release, err := lockJob(path)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := lockJob(path); err == nil {
		t.Fatal("second lockJob acquired exclusive lock")
	}
}

func TestLockAllowsRemovalWhileHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "job")
	if err := secureJobDir(path); err != nil {
		t.Fatal(err)
	}
	release, err := lockJob(path)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	// lockJob deliberately grants FILE_SHARE_DELETE. A remover that has the
	// lifecycle lock can delete the entry while keeping that lock held; a
	// racing runner then finds no lock file and cannot recreate one.
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("job remains after removal: %v", err)
	}
}

func TestPersistentDirectoryRejectsUnprotectedOrOtherOwner(t *testing.T) {
	sid, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "job")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := validateProtectedDirectory(path, sid); err == nil {
		t.Fatal("accepted inherited/unprotected DACL")
	}
	protected := filepath.Join(t.TempDir(), "protected")
	if err := secureJobDir(protected); err != nil {
		t.Fatal(err)
	}
	if err := validateProtectedDirectory(protected, "S-1-5-18"); err == nil {
		t.Fatal("accepted different owner")
	}
}
