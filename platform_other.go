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
func launchConsole(o options) int { return runCodex(o, nil) }
func consoleChild([]string) int {
	fmt.Fprintln(os.Stderr, "codex-at: internal console mode is Windows-only")
	return 2
}
