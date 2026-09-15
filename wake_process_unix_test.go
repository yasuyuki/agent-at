//go:build !windows

package main

import (
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestWakeProcessGroupCleanup(t *testing.T) {
	for _, naturalExit := range []bool{false, true} {
		t.Run(map[bool]string{false: "terminate", true: "natural-exit-release"}[naturalExit], func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			script := "sleep 30 & echo ready; wait"
			if naturalExit {
				script = "sleep 30 & echo ready"
			}
			cmd := exec.Command("/bin/sh", "-c", script)
			cmd.Stdout = w
			terminate, release, err := startWakeProcess(cmd)
			w.Close()
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			ready := make([]byte, 6)
			if _, err := io.ReadFull(r, ready); err != nil {
				t.Fatal(err)
			}
			if string(ready) != "ready\n" {
				t.Fatalf("unexpected readiness %q", ready)
			}
			if !naturalExit {
				if err := terminate(); err != nil {
					t.Fatal(err)
				}
			}
			_ = cmd.Wait()
			release()
			// The descendant inherited stdout: EOF proves it no longer holds
			// that handle even when the shell exited before cleanup.
			done := make(chan error, 1)
			go func() { _, err := io.Copy(io.Discard, r); done <- err }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("descendant still holds stdout")
			}
		})
	}
}
