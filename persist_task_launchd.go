//go:build linux || darwin

package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// launchdTaskScheduler is intentionally usable in host tests.  Its platform
// constructor is in persist_task_darwin.go.
type launchdTaskScheduler struct {
	home string
	uid  string
	run  func(args ...string) ([]byte, error)
}

func runLaunchctl(args ...string) ([]byte, error) {
	cmd := exec.Command("launchctl", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && stderr.Len() != 0 {
		return out, fmt.Errorf("launchctl: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return out, err
}

func (s launchdTaskScheduler) Create(spec taskSpec) error {
	plist, err := s.plist(spec)
	if err != nil {
		return &taskCreateError{Err: err}
	}
	if err := s.ensureDir(); err != nil {
		return &taskCreateError{Err: err}
	}
	path := s.path(spec)
	if err := writeLaunchdPlist(path, plist); err != nil {
		return &taskCreateError{Err: err, Uncertain: !os.IsExist(err)}
	}
	// A process failure after launchctl was invoked might have occurred after
	// launchd accepted the job. Preserve both plist and private payload.
	if _, err := s.run("bootstrap", s.domain(), path); err != nil {
		return &taskCreateError{Err: err, Uncertain: true}
	}
	return nil
}

func (s launchdTaskScheduler) Read(spec taskSpec) error {
	want, err := s.plist(spec)
	if err != nil {
		return err
	}
	if err := readExactLaunchdPlist(s.path(spec), want); err != nil {
		return err
	}
	// print has no stable machine-readable schema. A successful invocation only
	// proves launchd currently knows this label; the exact private plist above
	// is the authoritative definition and is deliberately not inferred here.
	_, err = s.run("print", s.service(spec))
	return err
}

func (s launchdTaskScheduler) Delete(spec taskSpec) error {
	want, err := s.plist(spec)
	if err != nil {
		return err
	}
	if err := readExactLaunchdPlist(s.path(spec), want); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// launchctl diagnostics and exit codes are not a stable ownership API.
			// A successful print proves a live service whose source is missing;
			// any other result remains ambiguous and is retained for inspection.
			if _, printErr := s.run("print", s.service(spec)); printErr == nil {
				return errors.New("LaunchAgent service exists but private plist is missing")
			} else {
				return fmt.Errorf("LaunchAgent plist is missing; service state is unknown: %w", printErr)
			}
		}
		return err
	}
	if _, err := s.run("bootout", s.service(spec)); err != nil {
		return err
	}
	if err := os.Remove(s.path(spec)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s launchdTaskScheduler) domain() string               { return "gui/" + s.uid }
func (s launchdTaskScheduler) service(spec taskSpec) string { return s.domain() + "/" + taskName(spec) }
func (s launchdTaskScheduler) dir() string                  { return filepath.Join(s.home, "Library", "LaunchAgents") }
func (s launchdTaskScheduler) path(spec taskSpec) string {
	return filepath.Join(s.dir(), taskName(spec)+".plist")
}
func (s launchdTaskScheduler) plist(spec taskSpec) (string, error) {
	return launchdPlistForHome(spec, s.home)
}

func (s launchdTaskScheduler) ensureDir() error {
	if s.home == "" || !filepath.IsAbs(s.home) || !validLaunchdValue(s.home) || !validLaunchdUID(s.uid) {
		return errors.New("resolve LaunchAgent directory")
	}
	if err := ensureLaunchdParent(filepath.Join(s.home, "Library")); err != nil {
		return err
	}
	return ensureLaunchdDir(s.dir())
}

func ensureLaunchdParent(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !launchdOwned(info) || info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("unsafe LaunchAgent parent directory: %s", path)
	}
	return nil
}

func ensureLaunchdDir(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err = os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !launchdOwned(info) || info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("unsafe LaunchAgent directory: %s", path)
	}
	return nil
}

func writeLaunchdPlist(path, body string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.WriteString(body); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return verifyLaunchdPlist(path)
}

func verifyLaunchdPlist(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !launchdOwned(info) || info.Mode().Perm() != 0600 {
		return fmt.Errorf("unsafe LaunchAgent plist: %s", path)
	}
	return nil
}

func readExactLaunchdPlist(path, want string) error {
	if err := verifyLaunchdPlist(path); err != nil {
		return err
	}
	got, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(got) != want {
		return errors.New("LaunchAgent plist differs from registered task")
	}
	return nil
}

func launchdPlist(spec taskSpec) (string, error) {
	return launchdPlistForHome(spec, "")
}

func launchdPlistForHome(spec taskSpec, home string) (string, error) {
	if !jobIDPattern.MatchString(spec.ID) || !validLaunchdUID(spec.SID) || !filepath.IsAbs(spec.Executable) || !filepath.IsAbs(spec.Directory) || spec.At.IsZero() {
		return "", errors.New("incomplete task specification")
	}
	if home != "" && (!filepath.IsAbs(home) || !validLaunchdValue(home)) {
		return "", errors.New("invalid HOME")
	}
	for _, value := range []string{spec.ID, spec.SID, spec.Executable, spec.Directory} {
		if !validLaunchdValue(value) {
			return "", errors.New("invalid LaunchAgent value")
		}
	}
	escape := func(value string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(value))
		return b.String()
	}
	month, day := spec.At.Month(), spec.At.Day()
	hour, minute, _ := spec.At.Clock()
	environment := ""
	if home != "" {
		environment = `<key>EnvironmentVariables</key><dict><key>HOME</key><string>` + escape(home) + `</string></dict>`
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>Label</key><string>` + escape(taskName(spec)) + `</string><key>ProgramArguments</key><array><string>` + escape(spec.Executable) + `</string><string>--internal-persist</string><string>` + escape(spec.ID) + `</string></array><key>WorkingDirectory</key><string>` + escape(spec.Directory) + `</string>` + environment + `<key>ProcessType</key><string>Background</string><key>RunAtLoad</key><true/><key>StartCalendarInterval</key><dict><key>Month</key><integer>` + strconv.Itoa(int(month)) + `</integer><key>Day</key><integer>` + strconv.Itoa(day) + `</integer><key>Hour</key><integer>` + strconv.Itoa(hour) + `</integer><key>Minute</key><integer>` + strconv.Itoa(minute) + `</integer></dict></dict></plist>
`, nil
}

func launchdOwned(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Geteuid()
}

func validLaunchdUID(uid string) bool {
	if uid == "" || strings.ContainsAny(uid, "+-") {
		return false
	}
	_, err := strconv.ParseUint(uid, 10, 32)
	return err == nil
}

func validLaunchdValue(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// preparePersistentDispatch compensates for launchd's calendar trigger having
// no year or seconds. At's saved fixed offset supplies the immutable calendar.
// skip means exit without calling store.execute.
func preparePersistentDispatch(j persistentJob, now time.Time) (wait time.Duration, skip bool) {
	due := j.Request.At
	if due.IsZero() {
		return 0, true
	}
	zone := time.FixedZone("agent-at saved offset", offsetSeconds(due))
	due = due.In(zone)
	now = now.In(zone)
	if now.Year() != due.Year() {
		return 0, true
	}
	if !now.Before(due) {
		return 0, false
	} // same-year catch-up is intentional.
	if now.Month() != due.Month() || now.Day() != due.Day() || now.Hour() != due.Hour() || now.Minute() != due.Minute() {
		return 0, true
	}
	wait = due.Sub(now)
	if wait <= 0 || wait >= time.Minute {
		return 0, true
	}
	return wait, false
}

func offsetSeconds(t time.Time) int { _, offset := t.Zone(); return offset }
