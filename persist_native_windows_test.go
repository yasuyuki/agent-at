//go:build windows

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var nativeJobID = regexp.MustCompile(`(?m)^Registered job ([0-9a-f]{32})$`)
var nativeJobDir = regexp.MustCompile(`(?m)^Private request, stdout\.log, stderr\.log and result\.json: (.+)$`)

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

func nativeRepositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository go.mod")
		}
		dir = parent
	}
}

func nativeGoBuild(t *testing.T, root, output, source string) {
	t.Helper()
	goTool := filepath.Join(runtime.GOROOT(), "bin", "go.exe")
	cmd := exec.Command(goTool, "build", "-o", output, source)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", source, err, out)
	}
}

func nativePersistentJob(t *testing.T, app, agent, label string) {
	t.Helper()
	work, err := os.MkdirTemp(filepath.Dir(app), "work-")
	if err != nil {
		t.Fatal(err)
	}
	store, err := openJobStore()
	if err != nil {
		t.Fatal(err)
	}
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			ids, err := nativeFixtureJobIDs(store, app, agent, work)
			if err != nil {
				t.Error(err)
				return
			}
			for _, id := range ids {
				remove := exec.Command(app, "--remove", id)
				if out, removeErr := remove.CombinedOutput(); removeErr != nil {
					t.Errorf("remove own job %s: %v\n%s", id, removeErr, out)
				}
			}
		})
	}
	// Register cleanup before invoking registration: read-back may fail after
	// the OS task and payload already exist, without any success output/ID.
	t.Cleanup(cleanup)
	at := time.Now().Add(25 * time.Second).Format("2006-01-02T15:04:05")
	cmd := exec.Command(app, "--persist", "--at", at, "--agent", "codex", "--agent-path", agent, "--cd", work, "native fixture "+label)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("register: %v\n%s", err, output)
	}
	idMatch := nativeJobID.FindSubmatch(output)
	dirMatch := nativeJobDir.FindSubmatch(output)
	if len(idMatch) != 2 || len(dirMatch) != 2 {
		t.Fatalf("registration did not report a job and private path:\n%s", output)
	}
	id, jobDir := string(idMatch[1]), string(dirMatch[1])

	// Registration is a completed public CLI process. Before the future trigger,
	// no private result or fake-agent marker may exist.
	if _, err := os.Stat(filepath.Join(jobDir, "result.json")); !os.IsNotExist(err) {
		t.Fatalf("registration returned after a result already existed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "persist-agent-runs")); !os.IsNotExist(err) {
		t.Fatalf("fake agent ran during registration: %v", err)
	}

	resultPath := filepath.Join(jobDir, "result.json")
	deadline := time.Now().Add(90 * time.Second)
	for {
		if _, err := os.Stat(resultPath); err == nil {
			// result.json is exclusive-written while the lifecycle lock remains
			// held. Wait for release before reading or invoking the duplicate.
			if release, lockErr := lockJob(jobDir); lockErr == nil {
				release()
				break
			}
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("scheduled result did not arrive before %s", deadline.Format(time.RFC3339))
		}
		time.Sleep(250 * time.Millisecond)
	}

	var result jobResult
	if err := readJSON(resultPath, &result); err != nil {
		t.Fatal(err)
	}
	if result.Kind != "cli_failed" || result.ExitCode != 23 {
		t.Fatalf("result = %+v, want cli_failed exit 23", result)
	}
	stdout, err := os.ReadFile(filepath.Join(jobDir, "stdout.log"))
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.ReadFile(filepath.Join(jobDir, "stderr.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stdout) != "persist fixture stdout\r\n" && string(stdout) != "persist fixture stdout\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(string(stderr), "persist fixture stderr") || !strings.Contains(string(stderr), "exit status 23") {
		t.Fatalf("stderr = %q", stderr)
	}
	marker, err := os.ReadFile(filepath.Join(work, "persist-agent-runs"))
	if err != nil || string(marker) != "run\n" {
		t.Fatalf("fixture marker = %q, %v", marker, err)
	}

	// Task Scheduler can be asked more than once. The internal entry point must
	// preserve the first start/result and must not replay a consumed request.
	before := append([]byte(nil), stdout...)
	if out, err := exec.Command(app, "--internal-persist", id).CombinedOutput(); err != nil {
		t.Fatalf("duplicate internal launch: %v\n%s", err, out)
	}
	after, err := os.ReadFile(filepath.Join(jobDir, "stdout.log"))
	if err != nil {
		t.Fatal(err)
	}
	marker, err = os.ReadFile(filepath.Join(work, "persist-agent-runs"))
	if err != nil || !bytes.Equal(before, after) || string(marker) != "run\n" {
		t.Fatalf("duplicate launch changed result: stdout=%q marker=%q err=%v", after, marker, err)
	}

	cleanup()
	if _, err := os.Stat(jobDir); !os.IsNotExist(err) {
		t.Fatalf("--remove retained private job: %v", err)
	}
	fmt.Fprintf(os.Stderr, "native persist %s job %s exercised process-exit-only scheduler execution\n", label, id)
}
