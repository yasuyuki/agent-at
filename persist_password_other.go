//go:build !windows

package main

import "errors"

// Only the Windows backend has a saved-credential logon mode. --logon password
// is rejected during option validation on other platforms, so this exists to
// keep the shared registration path buildable.
func promptLogonPassword() (string, error) {
	return "", errors.New("password logon requires native Windows")
}
