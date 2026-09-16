//go:build linux || darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// Opt-in: creates only disposable fake-agent reservations in this user's OS
// scheduler. It never uses real authentication or closes a user's terminal.
func TestPersistentUnixSchedulerNative(t *testing.T) {
	if os.Getenv("AGENT_AT_PERSIST_NATIVE") != "1" {
		t.Skip("set AGENT_AT_PERSIST_NATIVE=1 to exercise the real user scheduler")
	}
	if runtime.GOOS == "linux" {
		if out, err := exec.Command("systemctl", "--user", "show-environment").CombinedOutput(); err != nil {
			t.Fatalf("systemd user manager unavailable: %v\n%s", err, out)
		}
	}
	root := nativeRepositoryRoot(t)
	bin, err := os.MkdirTemp("", "agent-at-native-空白 $%-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("native fixtures retained: %s", bin)
			return
		}
		if err := os.RemoveAll(bin); err != nil {
			t.Error(err)
		}
	})
	app := filepath.Join(bin, "agent-at")
	fake := filepath.Join(bin, "persist-agent")
	nativeGoBuild(t, root, app, ".")
	nativeGoBuild(t, root, fake, "./testdata/persist-agent")
	nativePersistentJob(t, app, fake, runtime.GOOS+"/process-exit-only")
}
