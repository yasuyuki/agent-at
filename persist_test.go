package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeTasks struct {
	jobs                          map[string]taskSpec
	createErr, readErr, deleteErr error
	beforeCreate                  func(taskSpec)
}

func (f *fakeTasks) Create(s taskSpec) error {
	if f.beforeCreate != nil {
		f.beforeCreate(s)
	}
	if f.createErr != nil {
		return f.createErr
	}
	f.jobs[s.ID] = s
	return nil
}
func (f *fakeTasks) Read(s taskSpec) error {
	if f.readErr != nil {
		return f.readErr
	}
	if _, ok := f.jobs[s.ID]; !ok {
		return os.ErrNotExist
	}
	return nil
}
func (f *fakeTasks) Delete(s taskSpec) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, ok := f.jobs[s.ID]; !ok {
		return os.ErrNotExist
	}
	delete(f.jobs, s.ID)
	return nil
}

func testJobStore(t *testing.T) (jobStore, *fakeTasks, options) {
	t.Helper()
	o := helperOptions(t)
	o.Headless, o.Persist = true, true
	o.At = time.Now().Add(time.Minute).Truncate(time.Second)
	f := &fakeTasks{jobs: map[string]taskSpec{}}
	sid := "test-user"
	if runtime.GOOS == "windows" {
		_, actual, err := persistentIdentity()
		if err != nil {
			t.Fatal(err)
		}
		sid = actual
	}
	return jobStore{root: t.TempDir(), sid: sid, launcher: o.Executable, tasks: f}, f, o
}

func TestPersistFlagContract(t *testing.T) {
	for _, tc := range []struct {
		name           string
		o              options
		set            map[string]bool
		n              int
		platform       string
		ok, management bool
	}{
		{"persist", options{Persist: true}, map[string]bool{"persist": true, "at": true}, 0, "windows", true, false},
		{"redundant headless", options{Persist: true, Headless: true}, map[string]bool{"persist": true, "at": true, "headless": true}, 0, "windows", true, false},
		{"false headless", options{Persist: true}, map[string]bool{"persist": true, "at": true, "headless": true}, 0, "windows", false, false},
		{"console", options{Persist: true, NewConsole: true}, map[string]bool{"persist": true, "at": true}, 0, "windows", false, false},
		{"close", options{Persist: true, Close: true}, map[string]bool{"persist": true, "at": true}, 0, "windows", false, false},
		{"resume needs time", options{Persist: true, Resume: "session"}, map[string]bool{"persist": true, "resume": true}, 0, "windows", false, false},
		{"list", options{List: true}, map[string]bool{"list": true}, 0, "windows", true, true},
		{"list agent", options{List: true}, map[string]bool{"list": true, "agent": true}, 0, "windows", false, true},
		{"list positional", options{List: true}, map[string]bool{"list": true}, 1, "windows", false, true},
		{"list false", options{}, map[string]bool{"list": true}, 0, "windows", false, true},
		{"both", options{List: true, Remove: strings.Repeat("a", 32)}, map[string]bool{"list": true, "remove": true}, 0, "windows", false, true},
		{"traversal", options{Remove: "../job"}, map[string]bool{"remove": true}, 0, "windows", false, true},
		{"remove", options{Remove: strings.Repeat("a", 32)}, map[string]bool{"remove": true}, 0, "windows", true, true},
		{"Linux", options{Persist: true}, map[string]bool{"persist": true, "at": true}, 0, "linux", true, false},
		{"macOS", options{Persist: true}, map[string]bool{"persist": true, "at": true}, 0, "darwin", true, false},
		{"unsupported", options{Persist: true}, map[string]bool{"persist": true, "at": true}, 0, "freebsd", false, false},
		{"Mac list", options{List: true}, map[string]bool{"list": true}, 0, "darwin", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := validatePersist(tc.o, tc.set, tc.n, tc.platform)
			if (err == nil) != tc.ok || m != tc.management {
				t.Fatalf("management=%v err=%v", m, err)
			}
		})
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		for _, args := range [][]string{{"--list"}, {"--remove", strings.Repeat("a", 32)}, {"--persist", "--at", "23:59", "--agent-path", "nonexistent", "x"}} {
			_, err := parseOptions(args, time.Now(), io.Discard)
			if err == nil || !strings.Contains(err.Error(), "native Windows") {
				t.Fatalf("%v: %v", args, err)
			}
		}
	}
}

func TestPersistRegistrationSnapshot(t *testing.T) {
	s, f, o := testJobStore(t)
	file := filepath.Join(t.TempDir(), "日本語 request &.txt")
	if err := os.WriteFile(file, []byte(o.Prompt), 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := parseOptions([]string{"--at", o.At.Format("2006-01-02T15:04:05"), "--headless", "--agent-path", o.Executable, "--cd", o.CD, "--prompt-file", file}, time.Now(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Persist = true
	f.beforeCreate = func(spec taskSpec) {
		j, err := s.load(spec.ID)
		if err != nil || j.Request.Prompt != o.Prompt {
			t.Fatalf("payload not complete before registration: %v", err)
		}
	}
	j, err := s.register(parsed, []string{"PATH=" + filepath.Dir(o.Executable), "CODEX_HOME=" + o.CD, "OPENAI_API_KEY=do-not-store", "UNRELATED_SECRET=do-not-store"})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	got, err := s.load(j.ID)
	if err != nil || got.Request.Prompt != o.Prompt || !got.Request.At.Equal(parsed.At) {
		t.Fatalf("snapshot: %+v %v", got, err)
	}
	data, _ := os.ReadFile(filepath.Join(s.root, j.ID, "request.json"))
	if bytes.Contains(data, []byte("do-not-store")) {
		t.Fatal("secret persisted")
	}
	var out bytes.Buffer
	if err = s.list(&out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), o.Prompt) || !strings.Contains(out.String(), "reserved") {
		t.Fatal(out.String())
	}
	delete(f.jobs, j.ID)
	out.Reset()
	_ = s.list(&out)
	if !strings.Contains(out.String(), "OS task missing") {
		t.Fatal(out.String())
	}
	if err = s.remove(j.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(s.root, j.ID)); !os.IsNotExist(err) {
		t.Fatal("job not removed", err)
	}
}

func TestPersistRegistrationFailures(t *testing.T) {
	for _, kind := range []string{"create", "uncertain", "readback"} {
		t.Run(kind, func(t *testing.T) {
			s, f, o := testJobStore(t)
			switch kind {
			case "create":
				f.createErr = errors.New("denied")
			case "uncertain":
				f.createErr = &taskCreateError{Err: errors.New("interrupted"), Uncertain: true}
			case "readback":
				f.readErr = errors.New("unreadable")
			}
			j, err := s.register(o, nil)
			if err == nil {
				t.Fatal("expected error")
			}
			present, _ := exists(filepath.Join(s.root, j.ID))
			if present != (kind != "create") {
				t.Fatalf("retained=%v %v", present, err)
			}
			if kind != "create" && !strings.Contains(err.Error(), j.ID) {
				t.Fatal("missing recoverable job ID")
			}
		})
	}
}

func TestPersistRegistrationSerializesRemoval(t *testing.T) {
	s, f, o := testJobStore(t)
	f.beforeCreate = func(spec taskSpec) {
		if err := s.remove(spec.ID); err == nil {
			t.Error("removed a registering job")
		}
	}
	j, err := s.register(o, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.load(j.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.remove(j.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPersistExecuteOnceAndResults(t *testing.T) {
	s, _, o := testJobStore(t)
	j, err := s.register(o, nil)
	if err != nil {
		t.Fatal(err)
	}
	launches := 0
	launch := func(_ persistentJob, out, errout io.Writer) (int, string) {
		launches++
		fmt.Fprintln(out, "safe stdout")
		fmt.Fprintln(errout, "safe stderr")
		return 23, "cli_failed"
	}
	if _, err = s.execute(j.ID, o.At.Add(-time.Second), launch); err == nil || launches != 0 {
		t.Fatal("early launch", err)
	}
	if code, err := s.execute(j.ID, o.At, launch); code != 23 || err != nil {
		t.Fatalf("%d %v", code, err)
	}
	dir := filepath.Join(s.root, j.ID)
	before, _ := os.ReadFile(filepath.Join(dir, "result.json"))
	if _, err = s.execute(j.ID, o.At.Add(time.Hour), launch); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "result.json"))
	if launches != 1 || !bytes.Equal(before, after) {
		t.Fatal("duplicate execution/result overwrite")
	}
	if err = os.Remove(filepath.Join(dir, "result.json")); err != nil {
		t.Fatal(err)
	}
	if _, err = s.execute(j.ID, o.At, launch); err != nil || launches != 1 {
		t.Fatal("replayed unknown result", err)
	}
	var out bytes.Buffer
	_ = s.list(&out)
	if !strings.Contains(out.String(), "result unknown") {
		t.Fatal(out.String())
	}
	if err = s.remove(j.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPersistCancelStartRace(t *testing.T) {
	s, _, o := testJobStore(t)
	j, err := s.register(o, nil)
	if err != nil {
		t.Fatal(err)
	}
	started, finish, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := s.execute(j.ID, o.At, func(_ persistentJob, _, _ io.Writer) (int, string) { close(started); <-finish; return 0, "cli_success" })
		done <- err
	}()
	<-started
	if err = s.remove(j.ID); err == nil {
		t.Fatal("removed running data")
	}
	var duplicate atomic.Int32
	duplicateDone := make(chan struct{})
	go func() {
		_, _ = s.execute(j.ID, o.At, func(_ persistentJob, _, _ io.Writer) (int, string) { duplicate.Add(1); return 0, "cli_success" })
		close(duplicateDone)
	}()
	if duplicate.Load() != 0 {
		t.Fatal("concurrent duplicate")
	}
	close(finish)
	<-duplicateDone
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if err = s.remove(j.ID); err != nil {
		t.Fatal(err)
	}
	_, _ = s.execute(j.ID, o.At, func(_ persistentJob, _, _ io.Writer) (int, string) { duplicate.Add(1); return 0, "cli_success" })
	if duplicate.Load() != 0 {
		t.Fatal("deleted job started")
	}
}

func TestPersistFailedDeleteCancelsLocally(t *testing.T) {
	s, f, o := testJobStore(t)
	j, err := s.register(o, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.deleteErr = errors.New("OS denied deletion")
	if err = s.remove(j.ID); err == nil {
		t.Fatal("expected OS delete error")
	}
	_, err = s.execute(j.ID, o.At, func(_ persistentJob, _, _ io.Writer) (int, string) {
		t.Error("cancelled job launched")
		return 0, "cli_success"
	})
	if err != nil {
		t.Fatal(err)
	}
	f.deleteErr = nil
	if err = s.remove(j.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPersistRejectSavedSchemaAndOwner(t *testing.T) {
	for _, kind := range []string{"schema", "owner", "id", "environment", "path", "headless", "wake", "truncated-start"} {
		t.Run(kind, func(t *testing.T) {
			s, _, o := testJobStore(t)
			j, err := s.register(o, nil)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "schema":
				j.Version++
			case "owner":
				j.SID = "other-user"
			case "id":
				j.ID = strings.Repeat("0", 32)
			case "environment":
				j.Environment = map[string]string{"API_KEY": "bad"}
			case "path":
				j.Request.Executable = "relative"
			case "headless":
				j.Request.Headless = false
			case "wake":
				j.Request.Wake = true
			}
			dirEntries, _ := os.ReadDir(s.root)
			id := dirEntries[0].Name()
			dir := filepath.Join(s.root, id)
			if kind == "truncated-start" {
				err = os.WriteFile(filepath.Join(dir, "started.json"), []byte("{"), 0600)
			} else {
				err = os.Remove(filepath.Join(dir, "request.json"))
				if err == nil {
					err = createJSON(filepath.Join(dir, "request.json"), j)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.execute(id, o.At, func(_ persistentJob, _, _ io.Writer) (int, string) {
				t.Error("invalid request launched")
				return 0, "cli_success"
			})
			if kind != "truncated-start" && err == nil {
				t.Fatal("accepted invalid record")
			}
			if _, err = os.Stat(dir); err != nil {
				t.Fatal("record not retained")
			}
		})
	}
	s, _, _ := testJobStore(t)
	for _, id := range []string{"../x", "", strings.Repeat("A", 32), "x/y"} {
		if err := s.remove(id); err == nil {
			t.Fatal(id)
		}
	}
}

func TestPersistHeadlessRealHelper(t *testing.T) {
	for _, agent := range []string{"codex", "claude"} {
		for _, resume := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/resume=%v", agent, resume), func(t *testing.T) {
				s, _, o := testJobStore(t)
				o.Agent = agent
				if resume {
					o.Resume = "session-123"
					o.Prompt = "resume"
				}
				t.Setenv("AGENT_AT_TEST_FAIL", "1")
				j, err := s.register(o, os.Environ())
				if err != nil {
					t.Fatal(err)
				}
				code, err := s.execute(j.ID, o.At, launchPersistent)
				if code != 23 || err != nil {
					t.Fatalf("%d %v", code, err)
				}
				var observed observation
				if err = readJSON(filepath.Join(s.root, j.ID, "stdout.log"), &observed); err != nil {
					t.Fatal(err)
				}
				if observed.Input != o.Prompt || observed.Dir != o.CD {
					t.Fatalf("bad execution: %+v", observed)
				}
				var result jobResult
				if err = readJSON(filepath.Join(s.root, j.ID, "result.json"), &result); err != nil || result.Kind != "cli_failed" || result.ExitCode != 23 {
					t.Fatalf("%+v %v", result, err)
				}
			})
		}
	}
}

func TestPersistEnvironmentSnapshot(t *testing.T) {
	root := t.TempDir()
	fixed, err := captureJobEnvironment([]string{"Path=" + filepath.Dir(root) + string(os.PathListSeparator) + ".", "CODEX_HOME=relative", "USERPROFILE=" + root, "API_KEY=secret"})
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(fixed["CODEX_HOME"]) || len(fixed) != 3 {
		t.Fatal(fixed)
	}
	got := jobEnvironment([]string{"PATH=wrong", "API_KEY=runtime-only", "USERPROFILE=wrong"}, fixed)
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, "wrong") || !strings.Contains(joined, "API_KEY=runtime-only") || !strings.Contains(joined, "USERPROFILE="+root) {
		t.Fatal(got)
	}
}

func TestPersistWakeRebuildsAndAuthenticatesAtExecution(t *testing.T) {
	s, _, o := testJobStore(t)
	o.Wake, o.WakeText, o.WakeTimeout = true, "ready", time.Minute
	o.Prompt, o.AddDirs = wakePrompt(o.WakeText), nil
	home := t.TempDir()
	auth := filepath.Join(home, "auth.json")
	fixed := []string{"CODEX_HOME=" + home, "HOME=" + home, "USERPROFILE=" + home}
	j, err := s.register(o, fixed) // No credentials exist yet; no auth/network probe.
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(auth, []byte(`{"tokens":{"access_token":"synthetic-fixture"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", filepath.Join(home, "wrong-at-execution"))
	code, err := s.execute(j.ID, o.At, launchPersistent)
	if err != nil || code != 0 {
		t.Fatalf("%d %v", code, err)
	}
	var observed observation
	if err = readJSON(filepath.Join(s.root, j.ID, "stdout.log"), &observed); err != nil {
		t.Fatal(err)
	}
	if observed.Dir == o.CD || observed.Input != o.Prompt {
		t.Fatalf("wake not rebuilt: %+v", observed)
	}
	if _, err = os.Stat(observed.Dir); !os.IsNotExist(err) {
		t.Fatal("wake temporary cwd retained", err)
	}
	j2, err := s.register(o, fixed)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(auth); err != nil {
		t.Fatal(err)
	}
	code, err = s.execute(j2.ID, o.At, launchPersistent)
	if err != nil || code != 1 {
		t.Fatalf("%d %v", code, err)
	}
	var result jobResult
	if err = readJSON(filepath.Join(s.root, j2.ID, "result.json"), &result); err != nil || result.Kind != "auth_failed" {
		t.Fatalf("%+v %v", result, err)
	}
}

// Match only records belonging to this invocation's unique fixture paths.
// Never sweep other user jobs merely because their registration failed.
func nativeFixtureJobIDs(store jobStore, app, agent, work string) ([]string, error) {
	entries, err := os.ReadDir(store.root)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		if !entry.IsDir() || !jobIDPattern.MatchString(entry.Name()) {
			continue
		}
		j, err := store.load(entry.Name())
		if err == nil && j.Launcher == app && j.Request.Executable == agent && j.Request.CD == work {
			ids = append(ids, j.ID)
		}
	}
	return ids, nil
}

func TestNativeFixtureCleanupFindsReadbackFailureOnly(t *testing.T) {
	store, tasks, o := testJobStore(t)
	tasks.readErr = fmt.Errorf("read-back failed")
	j, err := store.register(o, nil)
	if err == nil {
		t.Fatal("expected read-back failure")
	}
	other := o
	other.CD = t.TempDir()
	if _, err := store.register(other, nil); err == nil {
		t.Fatal("expected other read-back failure")
	}
	ids, err := nativeFixtureJobIDs(store, store.launcher, o.Executable, o.CD)
	if err != nil || len(ids) != 1 || ids[0] != j.ID {
		t.Fatalf("fixture match = %v, %v", ids, err)
	}
}
