package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// This deliberately tiny agent has no network or credential behavior. The
// integration test checks its output, exit status, and one local run marker.
func main() {
	cwd, err := os.Getwd()
	if err == nil {
		f, err := os.OpenFile(filepath.Join(cwd, "persist-agent-runs"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err == nil {
			_, _ = f.WriteString("run\n")
			_ = f.Close()
		}
	}
	fmt.Fprintln(os.Stdout, "persist fixture stdout")
	fmt.Fprintln(os.Stderr, "persist fixture stderr")
	os.Exit(23)
}
