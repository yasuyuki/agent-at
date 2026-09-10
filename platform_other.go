//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func platformCommand(path string, args []string) (*exec.Cmd, error) {
	return exec.Command(path, args...), nil
}
func promptTempDir() (string, error) { return os.MkdirTemp("", "agent-at-") }
func launchConsole(o options) int    { return runAgent(o, nil) }
func consoleChild([]string) int {
	fmt.Fprintln(os.Stderr, "agent-at: internal console mode is Windows-only")
	return 2
}

func prepareConsole() (func(), error) { return func() {}, nil }
