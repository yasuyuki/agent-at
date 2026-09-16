//go:build windows

package main

func lockJobForExecution(path string) (func(), error) { return lockJob(path) }
