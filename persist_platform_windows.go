//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

const lifecycleLockName = ".agent-at-lifecycle.lock"

const (
	fileAttributeReparsePoint = 0x00000400
	invalidFileAttributes     = 0xffffffff
	ownerSecurityInformation  = 0x00000001
	daclSecurityInformation   = 0x00000004
	seDaclProtected           = 0x1000
	accessAllowedAceType      = 0x00
	objectInheritAce          = 0x01
	containerInheritAce       = 0x02
	fullControlMask           = 0x001f01ff
	aclSizeInformation        = 2
	fileShareRead             = 0x00000001
	fileShareWrite            = 0x00000002
	fileShareDelete           = 0x00000004
	genericRead               = 0x80000000
	genericWrite              = 0x40000000
	openExisting              = 3
	createNew                 = 1
	fileAttributeNormal       = 0x00000080
	lockfileFailImmediately   = 0x00000001
	lockfileExclusiveLock     = 0x00000002
)

var (
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
)

type knownFolderID struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

type aclSize struct {
	aceCount      uint32
	aclBytesInUse uint32
	aclBytesFree  uint32
}

func currentUserSID() (string, error) {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return "", err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String()
}

func protectedSecurityAttributes(sid string) (*syscall.SecurityAttributes, func(), error) {
	sddl, err := syscall.UTF16PtrFromString("D:P(A;OICI;FA;;;" + sid + ")")
	if err != nil {
		return nil, nil, err
	}
	var descriptor uintptr
	ok, _, callErr := advapi32.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW").Call(uintptr(unsafe.Pointer(sddl)), 1, uintptr(unsafe.Pointer(&descriptor)), 0)
	if ok == 0 {
		return nil, nil, windowsCallError(callErr)
	}
	attrs := &syscall.SecurityAttributes{SecurityDescriptor: descriptor}
	attrs.Length = uint32(unsafe.Sizeof(*attrs))
	return attrs, func() { syscall.LocalFree(syscall.Handle(descriptor)) }, nil
}

func createProtectedDirectory(path, sid string) error {
	attrs, release, err := protectedSecurityAttributes(sid)
	if err != nil {
		return err
	}
	defer release()
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return syscall.CreateDirectory(p, attrs)
}

func persistentIdentity() (string, string, error) {
	sid, err := currentUserSID()
	if err != nil {
		return "", "", err
	}
	base, err := localAppData()
	if err != nil {
		return "", "", err
	}
	if err := rejectReparsePoint(base); err != nil {
		return "", "", err
	}
	root := filepath.Join(base, "agent-at", "jobs")
	if err := ensureProtectedDirectory(filepath.Join(base, "agent-at"), sid); err != nil {
		return "", "", err
	}
	if err := ensureProtectedDirectory(root, sid); err != nil {
		return "", "", err
	}
	return root, sid, nil
}

func localAppData() (string, error) {
	// FOLDERID_LocalAppData, queried rather than trusted from LOCALAPPDATA.
	id := knownFolderID{data1: 0xf1b32785, data2: 0x6fba, data3: 0x4fcf, data4: [8]byte{0x9d, 0x55, 0x7b, 0x8e, 0x7f, 0x15, 0x70, 0x91}}
	var value *uint16
	hr, _, _ := shell32.NewProc("SHGetKnownFolderPath").Call(uintptr(unsafe.Pointer(&id)), 0, 0, uintptr(unsafe.Pointer(&value)))
	if hr != 0 {
		return "", fmt.Errorf("SHGetKnownFolderPath returned HRESULT 0x%08x", uint32(hr))
	}
	defer ole32.NewProc("CoTaskMemFree").Call(uintptr(unsafe.Pointer(value)))
	return utf16PointerToString(value), nil
}

func utf16PointerToString(value *uint16) string {
	if value == nil {
		return ""
	}
	n := 0
	for *(*uint16)(unsafe.Add(unsafe.Pointer(value), n*2)) != 0 {
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice(value, n))
}

func ensureProtectedDirectory(path, sid string) error {
	if err := rejectReparsePoint(path); err == nil {
		return validateProtectedDirectory(path, sid)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := createProtectedDirectory(path, sid); err == nil {
		return nil
	} else if err != syscall.ERROR_ALREADY_EXISTS {
		return err
	}
	if err := rejectReparsePoint(path); err != nil {
		return err
	}
	return validateProtectedDirectory(path, sid)
}

func secureJobDir(path string) error {
	sid, err := currentUserSID()
	if err != nil {
		return err
	}
	if err := createProtectedDirectory(path, sid); err != nil {
		return err
	}
	if err := createLifecycleLock(filepath.Join(path, lifecycleLockName), sid); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func lockJob(path string) (func(), error) {
	sid, err := currentUserSID()
	if err != nil {
		return nil, err
	}
	if err := rejectReparsePoint(path); err != nil {
		return nil, err
	}
	if err := validateProtectedDirectory(path, sid); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(path, lifecycleLockName)
	if err := rejectReparsePoint(lockPath); err != nil {
		return nil, err
	}
	f, err := openExistingLifecycleLock(lockPath)
	if err != nil {
		return nil, err
	}
	var overlapped syscall.Overlapped
	ok, _, callErr := kernel32.NewProc("LockFileEx").Call(f.Fd(), lockfileExclusiveLock|lockfileFailImmediately, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if ok == 0 {
		_ = f.Close()
		return nil, windowsCallError(callErr)
	}
	return func() {
		_, _, _ = kernel32.NewProc("UnlockFileEx").Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
		_ = f.Close()
	}, nil
}

func createLifecycleLock(path, sid string) error {
	attrs, release, err := protectedSecurityAttributes(sid)
	if err != nil {
		return err
	}
	defer release()
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	h, err := syscall.CreateFile(p, genericRead|genericWrite, 0, attrs, createNew, fileAttributeNormal, 0)
	if err != nil {
		return err
	}
	return syscall.CloseHandle(h)
}

func openExistingLifecycleLock(path string) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(p, genericRead|genericWrite, fileShareRead|fileShareWrite|fileShareDelete, nil, openExisting, fileAttributeNormal, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), lifecycleLockName), nil
}

func rejectReparsePoint(path string) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attrs, _, callErr := kernel32.NewProc("GetFileAttributesW").Call(uintptr(unsafe.Pointer(p)))
	if uint32(attrs) == invalidFileAttributes {
		return windowsCallError(callErr)
	}
	if uint32(attrs)&fileAttributeReparsePoint != 0 {
		return fmt.Errorf("refusing reparse point: %s", path)
	}
	return nil
}

func validateProtectedDirectory(path, sid string) error {
	expected, err := syscall.StringToSid(sid)
	if err != nil {
		return err
	}
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var owner, dacl, descriptor uintptr
	result, _, _ := advapi32.NewProc("GetNamedSecurityInfoW").Call(uintptr(unsafe.Pointer(p)), 1, ownerSecurityInformation|daclSecurityInformation, uintptr(unsafe.Pointer(&owner)), 0, uintptr(unsafe.Pointer(&dacl)), 0, uintptr(unsafe.Pointer(&descriptor)))
	if result != 0 {
		return syscall.Errno(result)
	}
	defer syscall.LocalFree(syscall.Handle(descriptor))
	equal, _, callErr := advapi32.NewProc("EqualSid").Call(owner, uintptr(unsafe.Pointer(expected)))
	if equal == 0 {
		if callErr != syscall.Errno(0) {
			return windowsCallError(callErr)
		}
		return fmt.Errorf("directory owner is not the current user: %s", path)
	}
	var control uint16
	var revision uint32
	ok, _, callErr := advapi32.NewProc("GetSecurityDescriptorControl").Call(descriptor, uintptr(unsafe.Pointer(&control)), uintptr(unsafe.Pointer(&revision)))
	if ok == 0 {
		return windowsCallError(callErr)
	}
	if control&seDaclProtected == 0 || dacl == 0 {
		return fmt.Errorf("directory does not have a protected DACL: %s", path)
	}
	var size aclSize
	ok, _, callErr = advapi32.NewProc("GetAclInformation").Call(dacl, uintptr(unsafe.Pointer(&size)), unsafe.Sizeof(size), aclSizeInformation)
	if ok == 0 {
		return windowsCallError(callErr)
	}
	if size.aceCount != 1 {
		return fmt.Errorf("directory DACL is not exclusively current-user access: %s", path)
	}
	var acePointer unsafe.Pointer
	ok, _, callErr = advapi32.NewProc("GetAce").Call(dacl, 0, uintptr(unsafe.Pointer(&acePointer)))
	if ok == 0 {
		return windowsCallError(callErr)
	}
	if *(*byte)(acePointer) != accessAllowedAceType || *(*byte)(unsafe.Add(acePointer, 1)) != objectInheritAce|containerInheritAce || *(*uint32)(unsafe.Add(acePointer, 4)) != fullControlMask {
		return fmt.Errorf("directory DACL is not exclusively current-user full control: %s", path)
	}
	aceSID := (*syscall.SID)(unsafe.Add(acePointer, 8))
	equal, _, callErr = advapi32.NewProc("EqualSid").Call(uintptr(unsafe.Pointer(aceSID)), uintptr(unsafe.Pointer(expected)))
	if equal == 0 {
		if callErr != syscall.Errno(0) {
			return windowsCallError(callErr)
		}
		return fmt.Errorf("directory DACL grants a different user: %s", path)
	}
	return nil
}

func windowsCallError(err error) error {
	if err != nil && err != syscall.Errno(0) {
		return err
	}
	return syscall.EINVAL
}
