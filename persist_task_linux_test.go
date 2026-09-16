//go:build linux

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func linuxTestSpec() taskSpec {
	return taskSpec{ID: "81c281c281c281c281c281c281c281c2", SID: linuxUID(), Executable: "/tmp/a $b%\\\"c", Directory: "/tmp/d $e%\\\"f", At: time.Date(2026, 9, 16, 5, 4, 3, 0, time.FixedZone("UTC+9", 9*3600))}
}

func TestLinuxUnitEscapesLiteralSpecialCharacters(t *testing.T) {
	home := t.TempDir() + " $%"
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	service, timer, err := linuxUnitText(linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"$$", "%%", `\\`, `\\\"`, `Environment="HOME=`} {
		if !strings.Contains(service, want) {
			t.Fatalf("service missing %q: %s", want, service)
		}
	}
	if !strings.Contains(timer, "OnCalendar=2026-09-15 20:04:03 UTC") || !strings.Contains(timer, "Persistent=false") || !strings.Contains(timer, "WakeSystem=false") {
		t.Fatalf("timer = %s", timer)
	}
}

func TestLinuxCreateCollisionAndUncertainManagerFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s := linuxTaskScheduler{unitDir: dir, run: func(args ...string) ([]byte, error) { return nil, nil }}
	spec := linuxTestSpec()
	if err := os.WriteFile(s.servicePath(spec), []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}
	err := s.Create(spec)
	var createErr *taskCreateError
	if !errors.As(err, &createErr) || createErr.Uncertain {
		t.Fatalf("collision = %#v", err)
	}
	if _, err := os.Stat(s.timerPath(spec)); !os.IsNotExist(err) {
		t.Fatalf("timer exists after collision: %v", err)
	}
	_ = os.Remove(s.servicePath(spec))
	s.run = func(args ...string) ([]byte, error) { return nil, errors.New("manager lost") }
	err = s.Create(spec)
	if !errors.As(err, &createErr) || !createErr.Uncertain {
		t.Fatalf("manager failure = %#v", err)
	}
	for _, path := range []string{s.servicePath(spec), s.timerPath(spec)} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("partial unit not retained %s: %v", path, statErr)
		}
	}
}

func TestLinuxReadRejectsAlteredUnitAndDropIn(t *testing.T) {
	dir := t.TempDir()
	spec := linuxTestSpec()
	service, timer, err := linuxUnitText(spec)
	if err != nil {
		t.Fatal(err)
	}
	s := linuxTaskScheduler{unitDir: dir}
	if err := os.WriteFile(s.servicePath(spec), []byte(service+"# altered\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.timerPath(spec), []byte(timer), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Read(spec); err == nil {
		t.Fatal("altered service was accepted")
	}
	if err := os.WriteFile(s.servicePath(spec), []byte(service), 0600); err != nil {
		t.Fatal(err)
	}
	s.run = func(args ...string) ([]byte, error) {
		name := args[len(args)-1]
		path := s.servicePath(spec)
		state := "inactive\ndead\n"
		if name == s.timerName(spec) {
			path, state = s.timerPath(spec), "active\nwaiting\n"
		}
		return []byte("Id=" + name + "\nNames=" + name + "\nLoadState=loaded\nFragmentPath=" + path + "\nDropInPaths=/tmp/evil.conf\nActiveState=" + strings.Split(state, "\n")[0] + "\nSubState=" + strings.Split(state, "\n")[1] + "\nUnitFileState=enabled\nExecStart=" + spec.Executable + " --internal-persist " + spec.ID + "\n"), nil
	}
	if err := s.Read(spec); err == nil {
		t.Fatal("drop-in was accepted")
	}
}

func TestLinuxIdentityRejectsSymlinkState(t *testing.T) {
	home, target := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Symlink(target, filepath.Join(home, ".local")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := persistentIdentity(); err == nil {
		t.Fatal("symlinked private state was accepted")
	}
}

func linuxShow(s linuxTaskScheduler, spec taskSpec, name, unitState, active, sub string) []byte {
	path := s.servicePath(spec)
	execStart := "ExecStart=/fixture --internal-persist " + spec.ID + "\n"
	if name == s.timerName(spec) {
		path, execStart = s.timerPath(spec), "ExecStart=\n"
	}
	return []byte("Id=" + name + "\nNames=" + name + "\nLoadState=loaded\nFragmentPath=" + path + "\nDropInPaths=\nUnitFileState=" + unitState + "\nActiveState=" + active + "\nSubState=" + sub + "\n" + execStart)
}

func TestLinuxCreateReadDeleteLifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	spec := linuxTestSpec()
	var calls []string
	s := linuxTaskScheduler{unitDir: dir}
	s.run = func(args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		if args[0] != "show" {
			return nil, nil
		}
		name := args[len(args)-1]
		if name == s.timerName(spec) {
			return linuxShow(s, spec, name, "enabled", "active", "waiting"), nil
		}
		return linuxShow(s, spec, name, "static", "inactive", "dead"), nil
	}
	if err := s.Create(spec); err != nil {
		t.Fatal(err)
	}
	if err := s.Read(spec); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(spec); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.timerPath(spec)); !os.IsNotExist(err) {
		t.Fatalf("timer retained: %v", err)
	}
	if _, err := os.Stat(s.servicePath(spec)); !os.IsNotExist(err) {
		t.Fatalf("service retained: %v", err)
	}
	want := []string{"daemon-reload", "enable --now " + s.timerName(spec), "disable --now " + s.timerName(spec), "daemon-reload"}
	for _, item := range want {
		if !containsCall(calls, item) {
			t.Fatalf("missing %q in %v", item, calls)
		}
	}
}

func TestLinuxReadAcceptsElapsedAndDeleteCleansOwnedHalf(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	spec := linuxTestSpec()
	service, _, err := linuxUnitText(spec)
	if err != nil {
		t.Fatal(err)
	}
	s := linuxTaskScheduler{unitDir: dir, run: func(args ...string) ([]byte, error) { return nil, nil }}
	if err := os.WriteFile(s.servicePath(spec), []byte(service), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(spec); err != nil {
		t.Fatalf("remove owned service half: %v", err)
	}
	if _, err := os.Stat(s.servicePath(spec)); !os.IsNotExist(err) {
		t.Fatalf("service retained: %v", err)
	}

	if err := s.Create(spec); err != nil {
		t.Fatal(err)
	}
	s.run = func(args ...string) ([]byte, error) {
		if args[0] != "show" {
			return nil, nil
		}
		name := args[len(args)-1]
		if name == s.timerName(spec) {
			return linuxShow(s, spec, name, "enabled", "active", "elapsed"), nil
		}
		return linuxShow(s, spec, name, "static", "inactive", "dead"), nil
	}
	if err := s.Read(spec); err != nil {
		t.Fatalf("elapsed timer rejected: %v", err)
	}
}

func TestLinuxUnitDirectoryAllowsOwnedNonWritable0755(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := (linuxTaskScheduler{unitDir: dir}).ensureUnitDir(); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxExecutionLockWaitsForRegistration(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := secureJobDir(filepath.Join(dir, "job")); err != nil {
		t.Fatal(err)
	}
	job := filepath.Join(dir, "job")
	release, err := lockJob(job)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		unlock, lockErr := lockJobForExecution(job)
		if lockErr == nil {
			unlock()
		}
		done <- lockErr
	}()
	select {
	case err := <-done:
		t.Fatalf("execution acquired early: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("execution lock did not acquire")
	}
}

func TestLinuxGeneratedUnitsPassSystemdVerify(t *testing.T) {
	if _, err := exec.LookPath("systemd-analyze"); err != nil {
		t.Skip("systemd-analyze is unavailable")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	spec := linuxTestSpec()
	service, timer, err := linuxUnitText(spec)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	servicePath, timerPath := filepath.Join(dir, taskName(spec)+".service"), filepath.Join(dir, taskName(spec)+".timer")
	if err := os.WriteFile(servicePath, []byte(service), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(timerPath, []byte(timer), 0600); err != nil {
		t.Fatal(err)
	}
	runtimeDir := t.TempDir()
	if err := os.Chmod(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("systemd-analyze", "verify", "--user", servicePath, timerPath)
	cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+runtimeDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("systemd-analyze verify: %v\n%s", err, out)
	}
}

func containsCall(calls []string, want string) bool {
	for _, call := range calls {
		if call == want {
			return true
		}
	}
	return false
}

func TestLinuxPublicRemoveDisabledTimer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store, _, o := testJobStore(t)
	store.sid = linuxUID()
	scheduler := linuxTaskScheduler{unitDir: filepath.Join(home, ".config", "systemd", "user")}
	disabled := false
	scheduler.run = func(args ...string) ([]byte, error) {
		if args[0] != "show" {
			return nil, nil
		}
		name := args[len(args)-1]
		id := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(name, "agent-at-"+store.sid+"-"), ".service"), ".timer")
		spec := taskSpec{ID: id, SID: store.sid}
		if strings.HasSuffix(name, ".timer") {
			if disabled {
				return linuxShow(scheduler, spec, name, "disabled", "inactive", "dead"), nil
			}
			return linuxShow(scheduler, spec, name, "enabled", "active", "waiting"), nil
		}
		return linuxShow(scheduler, spec, name, "static", "inactive", "dead"), nil
	}
	store.tasks = scheduler
	j, err := store.register(o, nil)
	if err != nil {
		t.Fatal(err)
	}
	disabled = true
	if err := store.remove(j.ID); err != nil {
		t.Fatalf("public removal of disabled owned timer: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.root, j.ID)); !os.IsNotExist(err) {
		t.Fatal("job retained", err)
	}
}

func TestLinuxPartialDeleteRefusesTamperedSibling(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	spec := linuxTestSpec()
	s := linuxTaskScheduler{unitDir: t.TempDir(), run: func(...string) ([]byte, error) { t.Fatal("contacted manager for tampered pair"); return nil, nil }}
	if err := os.WriteFile(s.timerPath(spec), []byte("other task"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(spec); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial tamper reported safe absence: %v", err)
	}
}
