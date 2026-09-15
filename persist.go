package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const jobVersion = 1

var jobIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func validatePersist(o options, set map[string]bool, positional int, platform string) (bool, error) {
	management := set["list"] || set["remove"]
	if management {
		if (set["list"] && set["remove"]) || (set["list"] && !o.List) || (set["remove"] && !jobIDPattern.MatchString(o.Remove)) {
			return true, errors.New("use --list or --remove with a valid job ID")
		}
		if len(set) != 1 || positional != 0 {
			return true, errors.New("--list and --remove are exclusive with all reservation inputs")
		}
	}
	if (management || o.Persist) && platform != "windows" {
		return management, errors.New("persistent jobs are supported only by native Windows agent-at (no WSL bridge)")
	}
	if o.Persist {
		if !set["at"] {
			return false, errors.New("--persist requires --at, including resume")
		}
		if (set["headless"] && !o.Headless) || o.NewConsole || o.Close {
			return false, errors.New("--persist requires headless execution without new-console or close-on-exit")
		}
	}
	return management, nil
}

// Only nonsecret execution dependencies are snapshotted. Other environment
// variables come from the OS at execution; credentials are read in their homes.
var persistentEnvKeys = []string{"PATH", "HOME", "USERPROFILE", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "ANTHROPIC_CONFIG_DIR", "XDG_CONFIG_HOME", "APPDATA"}

func captureJobEnvironment(env []string) (map[string]string, error) {
	values := map[string]string{}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		key = strings.ToUpper(key)
		for _, allowed := range persistentEnvKeys {
			if key == allowed {
				if value != "" && key != "PATH" {
					var err error
					value, err = filepath.Abs(value)
					if err != nil {
						return nil, err
					}
				}
				if key == "PATH" {
					parts := filepath.SplitList(value)
					for i, part := range parts {
						// Freeze relative PATH entries against the registration cwd.
						var err error
						parts[i], err = filepath.Abs(part)
						if err != nil {
							return nil, err
						}
					}
					value = strings.Join(parts, string(os.PathListSeparator))
				}
				values[key] = value
			}
		}
	}
	return values, nil
}

func jobEnvironment(base []string, fixed map[string]string) []string {
	var env []string
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if _, found := fixed[strings.ToUpper(key)]; !found {
			env = append(env, entry)
		}
	}
	keys := make([]string, 0, len(fixed))
	for key := range fixed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env = append(env, key+"="+fixed[key])
	}
	return env
}

type persistentJob struct {
	Version     int               `json:"version"`
	ID          string            `json:"id"`
	SID         string            `json:"sid"`
	Launcher    string            `json:"launcher"`
	Request     options           `json:"request"`
	Environment map[string]string `json:"environment"`
}

type jobResult struct {
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	ExitCode int       `json:"exit_code"`
	Kind     string    `json:"kind"`
}

type jobStore struct {
	root, sid, launcher string
	tasks               taskScheduler
}

func (s jobStore) dir(id string) (string, error) {
	if !jobIDPattern.MatchString(id) {
		return "", errors.New("invalid job ID")
	}
	return filepath.Join(s.root, id), nil
}

func (s jobStore) spec(j persistentJob) taskSpec {
	return taskSpec{ID: j.ID, SID: s.sid, Executable: j.Launcher, Directory: filepath.Dir(j.Launcher), At: j.Request.At}
}

func readJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if err = d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}

// Exclusive creation plus Sync precedes any OS registration or child launch.
// A damaged/partial start record still prevents a later request.
func createJSON(path string, v any) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	writeErr := json.NewEncoder(f).Encode(v)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func (s jobStore) load(id string) (persistentJob, error) {
	var j persistentJob
	dir, err := s.dir(id)
	if err != nil {
		return j, err
	}
	if err = readJSON(filepath.Join(dir, "request.json"), &j); err != nil {
		return j, err
	}
	if j.Version != jobVersion {
		return j, fmt.Errorf("unsupported job schema %d; record retained", j.Version)
	}
	if j.ID != id || j.SID != s.sid {
		return j, errors.New("job ID/owner mismatch")
	}
	o := j.Request
	if !filepath.IsAbs(j.Launcher) || !filepath.IsAbs(o.Executable) || !filepath.IsAbs(o.CD) || o.At.IsZero() || !o.Headless || o.NewConsole || o.Close || o.List || o.Remove != "" || (o.Agent != "codex" && o.Agent != "claude") {
		return j, errors.New("invalid saved execution request")
	}
	for _, value := range append([]string{j.Launcher, o.Executable, o.CD, o.Prompt, o.Model, o.Resume}, o.AddDirs...) {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return j, errors.New("invalid saved text/path")
		}
	}
	if strings.TrimSpace(o.Prompt) == "" {
		return j, errors.New("empty saved prompt")
	}
	for _, dir := range o.AddDirs {
		if !filepath.IsAbs(dir) {
			return j, errors.New("relative saved add-dir")
		}
	}
	if o.Wake {
		if o.Resume != "" || len(o.AddDirs) != 0 || o.Prompt != wakePrompt(o.WakeText) {
			return j, errors.New("invalid saved wake request")
		}
		if err := validateWake(o, map[string]bool{}, 0); err != nil {
			return j, err
		}
	} else if o.Resume != "" && (o.Resume == "-" || strings.TrimSpace(o.Resume) == "" || strings.ContainsAny(o.Resume, "\r\n") || o.Prompt != "resume") {
		return j, errors.New("invalid saved resume")
	}
	for key, value := range j.Environment {
		allowed := false
		for _, k := range persistentEnvKeys {
			if key == k {
				allowed = true
			}
		}
		if !allowed || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return j, errors.New("invalid saved environment")
		}
		if key != "PATH" && value != "" && !filepath.IsAbs(value) {
			return j, errors.New("relative saved environment path")
		}
		if key == "PATH" {
			for _, p := range filepath.SplitList(value) {
				if !filepath.IsAbs(p) {
					return j, errors.New("relative saved PATH")
				}
			}
		}
	}
	return j, nil
}

func (s jobStore) register(o options, env []string) (persistentJob, error) {
	var j persistentJob
	fixed, err := captureJobEnvironment(env)
	if err != nil {
		return j, err
	}
	// Validate argv/shim limits without starting a CLI. No temporary cwd is
	// retained, and no authentication or model request occurs on registration.
	args := agentArgs(o, "-")
	if o.Wake {
		args = wakeArgs(o)
	}
	if _, err := platformCommand(o.Executable, args); err != nil {
		return j, err
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return j, err
	}
	j = persistentJob{Version: jobVersion, ID: hex.EncodeToString(id[:]), SID: s.sid, Launcher: s.launcher, Request: o, Environment: fixed}
	dir, _ := s.dir(j.ID)
	if err = secureJobDir(dir); err != nil {
		return j, err
	}
	release, err := lockJob(dir)
	if err != nil {
		return j, errors.Join(err, os.RemoveAll(dir))
	}
	defer release()
	if err = createJSON(filepath.Join(dir, "request.json"), j); err != nil {
		return j, errors.Join(err, os.RemoveAll(dir))
	}
	if !o.At.After(time.Now()) {
		return j, errors.Join(errors.New("scheduled time elapsed before OS registration"), os.RemoveAll(dir))
	}
	if err = s.tasks.Create(s.spec(j)); err != nil {
		var uncertain *taskCreateError
		if errors.As(err, &uncertain) && uncertain.Uncertain {
			return j, fmt.Errorf("registration uncertain for job %s; retained at %s: %w", j.ID, dir, err)
		}
		return j, errors.Join(err, os.RemoveAll(dir))
	}
	if err = s.tasks.Read(s.spec(j)); err != nil {
		// Do not silently remove a task whose state/ownership cannot be read.
		return j, fmt.Errorf("registration read-back failed for job %s; task/data retained at %s: %w", j.ID, dir, err)
	}
	if !o.At.After(time.Now()) {
		return j, fmt.Errorf("job %s reached its time during registration; task/data retained at %s, execution not confirmed", j.ID, dir)
	}
	return j, nil
}

func exists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (s jobStore) execute(id string, now time.Time, launch func(persistentJob, io.Writer, io.Writer) (int, string)) (int, error) {
	dir, err := s.dir(id)
	if err != nil {
		return 2, err
	}
	release, err := lockJob(dir)
	if err != nil {
		return 1, fmt.Errorf("job running, being removed, or unavailable: %w", err)
	}
	defer release()
	j, err := s.load(id)
	if err != nil {
		return 2, err
	}
	if now.Before(j.Request.At) {
		return 2, errors.New("job is not due; no request sent")
	}
	for _, name := range []string{"started.json", "cancelled.json"} {
		present, err := exists(filepath.Join(dir, name))
		if err != nil {
			return 1, err
		}
		if present {
			return 0, nil
		} // Never overwrite a previous start/result.
	}
	if err = createJSON(filepath.Join(dir, "started.json"), struct {
		Started time.Time `json:"started"`
	}{now}); err != nil {
		return 1, err
	}
	result := jobResult{Started: now, ExitCode: 1, Kind: "launch_failed"}
	stdout, err := os.OpenFile(filepath.Join(dir, "stdout.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		var stderr *os.File
		stderr, err = os.OpenFile(filepath.Join(dir, "stderr.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			result.ExitCode, result.Kind = launch(j, stdout, stderr)
			err = errors.Join(stdout.Sync(), stderr.Sync(), stderr.Close())
		}
		err = errors.Join(err, stdout.Close())
	}
	result.Finished = time.Now()
	if err != nil {
		result.Kind = "log_failed"
		result.ExitCode = 1
	}
	if saveErr := createJSON(filepath.Join(dir, "result.json"), result); saveErr != nil {
		return 1, errors.Join(err, fmt.Errorf("result unknown: %w", saveErr))
	}
	return result.ExitCode, err
}

func launchPersistent(j persistentJob, stdout, stderr io.Writer) (int, string) {
	o := j.Request
	var cmd *exec.Cmd
	var cleanup func()
	var err error
	if o.Wake {
		cmd, cleanup, err = prepareWake(o)
	} else {
		cmd, cleanup, err = prepare(o)
	}
	defer cleanup()
	if err != nil {
		fmt.Fprintln(stderr, "agent-at: prepare:", err)
		return 1, "launch_failed"
	}
	cmd.Env = jobEnvironment(cmd.Environ(), j.Environment)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if o.Wake {
		cmd.Env = wakeEnvironment(cmd.Env)
		if err = checkWakeAuth(o.Agent, cmd.Env); err != nil {
			fmt.Fprintln(stderr, "agent-at: authentication:", err)
			return 1, "auth_failed"
		}
		code, kind := runWakeResult(context.Background(), cmd, o.WakeTimeout, stderr)
		if kind == "completed" {
			kind = "cli_success"
		} else if kind == "failed" {
			kind = "cli_failed"
		}
		return code, kind
	}
	if err = cmd.Start(); err != nil {
		fmt.Fprintln(stderr, "agent-at: launch:", err)
		return 1, "launch_failed"
	}
	if err = cmd.Wait(); err != nil {
		fmt.Fprintln(stderr, "agent-at:", err)
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), "cli_failed"
		}
		return 1, "result_unknown"
	}
	return 0, "cli_success"
}

func (s jobStore) remove(id string) error {
	dir, err := s.dir(id)
	if err != nil {
		return err
	}
	release, err := lockJob(dir)
	if err != nil {
		return fmt.Errorf("job running, being removed, or unavailable; retained: %w", err)
	}
	defer release()
	j, err := s.load(id)
	if err != nil {
		return err
	}
	// Validate ownership before removing any OS task. Missing OS registration
	// does not prevent removing this user's private record.
	if err = s.tasks.Read(s.spec(j)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	cancelled, err := exists(filepath.Join(dir, "cancelled.json"))
	if err != nil {
		return err
	}
	if !cancelled {
		if err = createJSON(filepath.Join(dir, "cancelled.json"), struct {
			At time.Time `json:"at"`
		}{time.Now()}); err != nil {
			return err
		}
	}
	if err = s.tasks.Delete(s.spec(j)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("local cancellation saved; OS deletion failed; data retained: %w", err)
	}
	return os.RemoveAll(dir)
}

func (s jobStore) list(out io.Writer) error {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !jobIDPattern.MatchString(entry.Name()) {
			continue
		}
		id := entry.Name()
		dir, _ := s.dir(id)
		j, err := s.load(id)
		if err != nil {
			fmt.Fprintf(out, "%s invalid/unsupported record; retained at %s\n", id, dir)
			continue
		}
		status := "reserved"
		started, startErr := exists(filepath.Join(dir, "started.json"))
		if startErr != nil {
			status = "state unreadable"
		} else if started {
			status = "started / result unknown"
		}
		var result jobResult
		if err := readJSON(filepath.Join(dir, "result.json"), &result); err == nil {
			status = fmt.Sprintf("%s (exit %d)", result.Kind, result.ExitCode)
		} else if !errors.Is(err, os.ErrNotExist) {
			status = "result unknown (unreadable result)"
		}
		if cancelled, _ := exists(filepath.Join(dir, "cancelled.json")); cancelled {
			status += "; cancellation/deletion pending"
		}
		osState := "OS registered"
		if err = s.tasks.Read(s.spec(j)); errors.Is(err, os.ErrNotExist) {
			osState = "OS task missing"
		} else if err != nil {
			osState = "OS task inconsistent/unreadable"
		}
		mode := "headless"
		if j.Request.Wake {
			mode = "wake"
		} else if j.Request.Resume != "" {
			mode = "headless resume"
		}
		fmt.Fprintf(out, "%s %s %s: %s; %s; %s\n", id, j.Request.At.Format(time.RFC3339), mode, status, osState, dir)
	}
	return nil
}

func openJobStore() (jobStore, error) {
	root, sid, err := persistentIdentity()
	if err != nil {
		return jobStore{}, err
	}
	exe, err := os.Executable()
	if err != nil {
		return jobStore{}, err
	}
	return jobStore{root: root, sid: sid, launcher: exe, tasks: newTaskScheduler()}, nil
}

func runPersistent(o options) int {
	s, err := openJobStore()
	if err == nil {
		switch {
		case o.List:
			err = s.list(os.Stdout)
		case o.Remove != "":
			err = s.remove(o.Remove)
			if err == nil {
				fmt.Fprintln(os.Stdout, "Removed job and saved data:", o.Remove)
			}
		default:
			var j persistentJob
			j, err = s.register(o, os.Environ())
			if err == nil {
				model := o.Model
				if model == "" {
					model = "inherit CLI configuration"
					if o.Wake {
						model = "clean CLI default (user settings skipped)"
					}
				}
				fmt.Fprintf(os.Stdout, "Registered job %s\nTask: %s\nAt: %s\nAgent: %s; model: %s\nPrivate request, stdout.log, stderr.log and result.json: %s\nRegistration succeeded; execution has not occurred. You may close this terminal.\nAlways headless. PC must be awake and the same user signed in; screen lock is OK.\nNo catch-up after power-off, sleep or sign-out. Keep executable/CLI/work paths in place.\n", j.ID, taskName(s.spec(j)), o.At.Format(time.RFC3339), o.Agent, model, filepath.Join(s.root, j.ID))
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent-at:", err)
		return 1
	}
	return 0
}

func persistentChild(args []string) int {
	if len(args) != 1 || !jobIDPattern.MatchString(args[0]) {
		fmt.Fprintln(os.Stderr, "agent-at: invalid internal job ID")
		return 2
	}
	s, err := openJobStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent-at:", err)
		return 1
	}
	code, err := s.execute(args[0], time.Now(), launchPersistent)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent-at:", err)
	}
	return code
}
