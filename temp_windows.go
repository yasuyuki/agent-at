package main

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// Windows ignores Unix mode bits. Protect the directory at creation time,
// including against permissions inherited from a shared TEMP location.
func promptTempDir() (string, error) {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return "", err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	sid, err := user.User.Sid.String()
	if err != nil {
		return "", err
	}
	sddl, err := syscall.UTF16PtrFromString("D:P(A;OICI;FA;;;" + sid + ")")
	if err != nil {
		return "", err
	}
	var descriptor uintptr
	ok, _, err := syscall.NewLazyDLL("advapi32.dll").NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW").Call(uintptr(unsafe.Pointer(sddl)), 1, uintptr(unsafe.Pointer(&descriptor)), 0)
	if ok == 0 {
		return "", err
	}
	defer syscall.LocalFree(syscall.Handle(descriptor))
	attrs := syscall.SecurityAttributes{SecurityDescriptor: descriptor}
	attrs.Length = uint32(unsafe.Sizeof(attrs))
	for {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", err
		}
		dir := filepath.Join(os.TempDir(), "agent-at-"+hex.EncodeToString(random[:]))
		path, err := syscall.UTF16PtrFromString(dir)
		if err != nil {
			return "", err
		}
		err = syscall.CreateDirectory(path, &attrs)
		if err == syscall.ERROR_ALREADY_EXISTS {
			continue
		}
		if err != nil {
			return "", err
		}
		return dir, nil
	}
}
