//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

const (
	enableProcessedInput = 0x0001
	enableLineInput      = 0x0002
	enableEchoInput      = 0x0004
)

// promptLogonPassword reads this account's Windows password from the console with
// echo disabled. The console is deliberately the only accepted source: a flag,
// a file or an environment variable would leave the credential in a command
// line, a shell history or a script. The value goes straight to Task Scheduler,
// which stores it; agent-at never writes it to the job record or to any log.
func promptLogonPassword() (string, error) {
	handle := syscall.Handle(os.Stdin.Fd())
	var mode uint32
	setConsoleMode := kernel32.NewProc("SetConsoleMode")
	ok, _, callErr := kernel32.NewProc("GetConsoleMode").Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	if ok == 0 {
		return "", errors.Join(errors.New("--logon password needs an interactive console for the Windows password"), windowsCallError(callErr))
	}
	if ok, _, callErr = setConsoleMode.Call(uintptr(handle), uintptr(mode&^enableEchoInput|enableLineInput|enableProcessedInput)); ok == 0 {
		return "", windowsCallError(callErr)
	}
	defer setConsoleMode.Call(uintptr(handle), uintptr(mode))
	account, err := currentAccountName()
	if err != nil {
		return "", err
	}
	fmt.Fprintf(os.Stderr, "Windows password for %s: ", account)
	buffer := make([]uint16, 512)
	var read uint32
	ok, _, callErr = kernel32.NewProc("ReadConsoleW").Call(uintptr(handle), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), uintptr(unsafe.Pointer(&read)), 0)
	fmt.Fprintln(os.Stderr)
	if ok == 0 {
		return "", windowsCallError(callErr)
	}
	if int(read) > len(buffer) {
		return "", errors.New("console returned more input than requested")
	}
	password := strings.TrimRight(syscall.UTF16ToString(buffer[:read]), "\r\n")
	// Best effort: the decoded string cannot be cleared, the read buffer can.
	for i := range buffer {
		buffer[i] = 0
	}
	if password == "" {
		return "", errors.New("the Windows password must not be empty")
	}
	if !utf8.ValidString(password) || strings.ContainsRune(password, 0) {
		return "", errors.New("unsupported characters in the Windows password")
	}
	return password, nil
}
