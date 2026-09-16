//go:build linux

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type linuxTaskScheduler struct {
	unitDir string
	run     func(args ...string) ([]byte, error)
}

const systemdExecHelper = "/usr/bin/env"

func newTaskScheduler() taskScheduler {
	home, err := linuxHome()
	if err != nil {
		return linuxTaskScheduler{run: runSystemctl, unitDir: ""}
	}
	return linuxTaskScheduler{unitDir: filepath.Join(home, ".config", "systemd", "user"), run: runSystemctl}
}

func runSystemctl(args ...string) ([]byte, error) {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && stderr.Len() != 0 {
		return out, fmt.Errorf("systemctl: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return out, err
}

func (s linuxTaskScheduler) Create(spec taskSpec) error {
	service, timer, err := linuxUnitText(spec)
	if err != nil {
		return &taskCreateError{Err: err}
	}
	if err := s.ensureUnitDir(); err != nil {
		return &taskCreateError{Err: err}
	}
	servicePath, timerPath := s.servicePath(spec), s.timerPath(spec)
	if err := writeLinuxUnit(servicePath, service); err != nil {
		return &taskCreateError{Err: err, Uncertain: !os.IsExist(err)}
	}
	if err := writeLinuxUnit(timerPath, timer); err != nil {
		if !os.IsExist(err) {
			return &taskCreateError{Err: err, Uncertain: true}
		}
		cleanupErr := os.Remove(servicePath)
		return &taskCreateError{Err: errors.Join(err, cleanupErr), Uncertain: cleanupErr != nil}
	}
	// Any failure after the manager is contacted can leave it with either an
	// enabled timer or an older cached definition.  Preserve the payload and
	// both units for inspection rather than guessing which cleanup is safe.
	if _, err := s.run("daemon-reload"); err != nil {
		return &taskCreateError{Err: err, Uncertain: true}
	}
	if _, err := s.run("enable", "--now", s.timerName(spec)); err != nil {
		return &taskCreateError{Err: err, Uncertain: true}
	}
	return nil
}

func (s linuxTaskScheduler) ReadForRemoval(spec taskSpec) error { return s.read(spec, false) }

func (s linuxTaskScheduler) Read(spec taskSpec) error {
	return s.read(spec, true)
}

func (s linuxTaskScheduler) read(spec taskSpec, requireEnabled bool) error {
	service, timer, err := linuxUnitText(spec)
	if err != nil {
		return err
	}
	if err := readExactLinuxUnit(s.servicePath(spec), service); err != nil {
		return err
	}
	if err := readExactLinuxUnit(s.timerPath(spec), timer); err != nil {
		return err
	}
	if err := s.validateShow(spec, s.serviceName(spec), s.servicePath(spec), false, requireEnabled); err != nil {
		return err
	}
	return s.validateShow(spec, s.timerName(spec), s.timerPath(spec), true, requireEnabled)
}

func (s linuxTaskScheduler) Delete(spec taskSpec) error {
	// Re-read immediately before disable/delete so a changed or collided unit
	// is never treated as ours merely because an earlier management read passed.
	if err := s.read(spec, false); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		// A prior partial registration can leave one exact managed file.  Remove
		// that owned half instead of leaving an orphan, but never touch a changed
		// or symlinked sibling.
		service, timer, textErr := linuxUnitText(spec)
		if textErr != nil {
			return textErr
		}
		serviceErr := readExactLinuxUnit(s.servicePath(spec), service)
		timerErr := readExactLinuxUnit(s.timerPath(spec), timer)
		if serviceErr != nil && !errors.Is(serviceErr, os.ErrNotExist) {
			return serviceErr
		}
		if timerErr != nil && !errors.Is(timerErr, os.ErrNotExist) {
			return timerErr
		}
		serviceOK, timerOK := serviceErr == nil, timerErr == nil
		if !serviceOK && !timerOK {
			return os.ErrNotExist
		}
		if timerOK {
			if _, runErr := s.run("disable", "--now", s.timerName(spec)); runErr != nil {
				return runErr
			}
			if removeErr := os.Remove(s.timerPath(spec)); removeErr != nil && !os.IsNotExist(removeErr) {
				return removeErr
			}
		}
		if serviceOK {
			if removeErr := os.Remove(s.servicePath(spec)); removeErr != nil && !os.IsNotExist(removeErr) {
				return removeErr
			}
		}
		_, reloadErr := s.run("daemon-reload")
		return reloadErr
	}
	if _, err := s.run("disable", "--now", s.timerName(spec)); err != nil {
		return err
	}
	if err := os.Remove(s.timerPath(spec)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(s.servicePath(spec)); err != nil && !os.IsNotExist(err) {
		return err
	}
	_, err := s.run("daemon-reload")
	return err
}

func (s linuxTaskScheduler) ensureUnitDir() error {
	if s.unitDir == "" || !filepath.IsAbs(s.unitDir) {
		return errors.New("resolve systemd user unit directory")
	}
	config, systemd := filepath.Dir(filepath.Dir(s.unitDir)), filepath.Dir(s.unitDir)
	for _, path := range []string{config, systemd} {
		if err := ensureLinuxOwnedDir(path); err != nil {
			return err
		}
	}
	return ensureLinuxOwnedDir(s.unitDir)
}

func (s linuxTaskScheduler) serviceName(spec taskSpec) string { return taskName(spec) + ".service" }
func (s linuxTaskScheduler) timerName(spec taskSpec) string   { return taskName(spec) + ".timer" }
func (s linuxTaskScheduler) servicePath(spec taskSpec) string {
	return filepath.Join(s.unitDir, s.serviceName(spec))
}
func (s linuxTaskScheduler) timerPath(spec taskSpec) string {
	return filepath.Join(s.unitDir, s.timerName(spec))
}

func writeLinuxUnit(path, text string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.WriteString(text); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return verifyLinuxPrivateFile(path)
}

func readExactLinuxUnit(path, want string) error {
	if err := verifyLinuxPrivateFile(path); err != nil {
		if os.IsNotExist(err) {
			return os.ErrNotExist
		}
		return err
	}
	got, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(got) != want {
		return errors.New("systemd unit differs from registered task")
	}
	return nil
}

func (s linuxTaskScheduler) validateShow(spec taskSpec, name, path string, timer, requireEnabled bool) error {
	out, err := s.run("show", "--property=Id,Names,LoadState,FragmentPath,DropInPaths,ActiveState,SubState,UnitFileState,ExecStart", name)
	if err != nil {
		return err
	}
	properties := systemdProperties(string(out))
	if properties["Id"] != name || properties["Names"] != name || properties["LoadState"] != "loaded" || properties["FragmentPath"] != path || nonEmptySystemdList(properties["DropInPaths"]) {
		return errors.New("systemd effective unit differs from registered task")
	}
	if timer {
		if requireEnabled && properties["UnitFileState"] != "enabled" {
			return errors.New("systemd timer is not enabled")
		}
		if requireEnabled && (properties["ActiveState"] != "active" || (properties["SubState"] != "waiting" && properties["SubState"] != "elapsed")) {
			return errors.New("systemd timer is not enabled and waiting")
		}
		return nil
	}
	// FragmentPath and the exact 0600 source file above establish the literal
	// argv; no drop-in can alter it.  ExecStart is deliberately checked only
	// for presence because `systemctl show` serializes quoted, percent, and
	// dollar-containing argv in a version-dependent display format.
	if properties["ExecStart"] == "" {
		return errors.New("systemd service action differs from registered task")
	}
	return nil
}

func systemdProperties(text string) map[string]string {
	properties := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			properties[key] = value
		}
	}
	return properties
}

func nonEmptySystemdList(value string) bool { return value != "" && value != "-" }

func linuxUnitText(spec taskSpec) (string, string, error) {
	if !jobIDPattern.MatchString(spec.ID) || !linuxNumericID(spec.SID) || !filepath.IsAbs(spec.Executable) || !filepath.IsAbs(spec.Directory) || spec.At.IsZero() || !validSystemdText(spec.Executable) || !validSystemdText(spec.Directory) {
		return "", "", errors.New("incomplete task specification")
	}
	home, err := linuxHome()
	if err != nil {
		return "", "", err
	}
	serviceName := taskName(spec) + ".service"
	service := "[Unit]\nDescription=agent-at persistent job " + spec.ID + "\n\n[Service]\nType=oneshot\nUMask=0077\nTimeoutStartSec=infinity\nKillMode=control-group\nEnvironment=" + systemdQuote("HOME="+home, false) + "\nWorkingDirectory=" + systemdDirectiveValue(spec.Directory) + "\nExecStart=" + systemdExecHelper + " -- " + systemdQuote(spec.Executable, true) + " --internal-persist " + spec.ID + "\n"
	timer := "[Unit]\nDescription=agent-at persistent timer " + spec.ID + "\n\n[Timer]\nOnCalendar=" + spec.At.UTC().Format("2006-01-02 15:04:05 UTC") + "\nAccuracySec=1s\nPersistent=false\nWakeSystem=false\nUnit=" + serviceName + "\n\n[Install]\nWantedBy=timers.target\n"
	return service, timer, nil
}

func linuxNumericID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func validSystemdText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

// systemdQuote writes one systemd command-line word.  It has no shell
// semantics: quoted whitespace stays within the word, percent and dollar are
// made literal, and controls cannot turn into a directive or another argument.
func systemdQuote(value string, expandDollar bool) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case '%':
			b.WriteString("%%")
		case '$':
			if expandDollar {
				b.WriteString("$$")
			} else {
				b.WriteRune(r)
			}
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, "\\x%02x", r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// systemdDirectiveValue is for single-value directives such as
// WorkingDirectory=.  Unlike ExecStart=, these do not use command-line quote
// removal, so special bytes are encoded rather than surrounded by quotes.
func systemdDirectiveValue(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '%':
			b.WriteString("%%")
		case '\\', '"', '\'', ' ', '\t', '\n', '\r':
			fmt.Fprintf(&b, "\\x%02x", r)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, "\\x%02x", r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}
