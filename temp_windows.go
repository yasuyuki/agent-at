package main

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"syscall"
)

// Windows ignores Unix mode bits. Protect the directory at creation time,
// including against permissions inherited from a shared TEMP location.
func promptTempDir() (string, error) {
	sid, err := currentUserSID()
	if err != nil {
		return "", err
	}
	for {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", err
		}
		dir := filepath.Join(os.TempDir(), "agent-at-"+hex.EncodeToString(random[:]))
		err = createProtectedDirectory(dir, sid)
		if err == syscall.ERROR_ALREADY_EXISTS {
			continue
		}
		if err != nil {
			return "", err
		}
		return dir, nil
	}
}
