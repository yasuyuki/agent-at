//go:build !darwin

package main

// Non-launchd schedulers represent the complete timestamp themselves.
func persistentDispatch(jobStore, string) (bool, error) { return false, nil }
