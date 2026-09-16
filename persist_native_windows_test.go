//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// This test uses the real Windows Task Scheduler and writes only the current
// user's agent-at records. It proves registration-process exit and the later
// task invocation, but cannot prove that a user has closed a terminal.
func TestPersistentTaskSchedulerNative(t *testing.T) {
	if os.Getenv("AGENT_AT_PERSIST_NATIVE") != "1" {
		t.Skip("set AGENT_AT_PERSIST_NATIVE=1 to run the Windows Task Scheduler integration test")
	}
	root := nativeRepositoryRoot(t)
	bin, err := os.MkdirTemp("", "agent-at-native-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("failed native test fixtures retained for recovery: %s", bin)
			return
		}
		if err := os.RemoveAll(bin); err != nil {
			t.Error(err)
		}
	})
	app := filepath.Join(bin, "agent-at.exe")
	nativeGoBuild(t, root, app, ".")
	fake := filepath.Join(bin, "persist-agent.exe")
	nativeGoBuild(t, root, fake, "./testdata/persist-agent")

	t.Run("exe/process-exit-only", func(t *testing.T) {
		nativePersistentJob(t, app, fake, "exe")
	})

	if _, err := exec.LookPath("node"); err != nil {
		t.Run("cmd", func(t *testing.T) { t.Skip("node is not on PATH") })
		return
	}
	t.Run("cmd", func(t *testing.T) {
		dir, err := os.MkdirTemp(bin, "cmd-")
		if err != nil {
			t.Fatal(err)
		}
		script := filepath.Join(dir, "persist-agent.js")
		if err := os.WriteFile(script, []byte(`const fs=require('fs'); fs.appendFileSync('persist-agent-runs','run\n'); console.log('persist fixture stdout'); console.error('persist fixture stderr'); process.exit(23);`), 0600); err != nil {
			t.Fatal(err)
		}
		shim := filepath.Join(dir, "persist-agent.cmd")
		if err := os.WriteFile(shim, []byte("@echo off\r\nnode \"%~dp0persist-agent.js\" %*\r\n"), 0600); err != nil {
			t.Fatal(err)
		}
		// The persisted allowlist snapshots PATH. The shim resolves node only at
		// scheduled execution, so this also covers the saved PATH path.
		nativePersistentJob(t, app, shim, "cmd")
	})
}
