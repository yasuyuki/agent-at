//go:build !windows && !linux && !darwin

package main

import "fmt"

func persistentIdentity() (string, string, error) {
	return "", "", fmt.Errorf("persistent schedules are supported on Windows, Linux or macOS")
}
