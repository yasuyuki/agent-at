//go:build linux || darwin

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testLaunchdSpec() taskSpec {
	return taskSpec{ID: "81c2a5d761ee4fc39628537ea1220b8d", SID: "501", Executable: "/Applications/agent at/agent-at", Directory: "/Users/me/work dir", At: time.Date(2026, 9, 16, 5, 4, 39, 0, time.FixedZone("UTC+9", 9*3600))}
}

func TestLaunchdPlistContainsOnlyTheScheduledInvocation(t *testing.T) {
	spec := testLaunchdSpec()
	plist, err := launchdPlistForHome(spec, "/Users/me")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<string>agent-at-501-81c2a5d761ee4fc39628537ea1220b8d</string>", "<string>/Applications/agent at/agent-at</string>",
		"<string>--internal-persist</string>", "<string>81c2a5d761ee4fc39628537ea1220b8d</string>", "<string>/Users/me/work dir</string>",
		"<key>HOME</key><string>/Users/me</string>", "<key>RunAtLoad</key><true/>",
		"<key>Month</key><integer>9</integer>", "<key>Day</key><integer>16</integer>",
		"<key>Hour</key><integer>5</integer>", "<key>Minute</key><integer>4</integer>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist missing %q: %s", want, plist)
		}
	}
	for _, forbidden := range []string{"<key>Year</key>", "<key>Second</key>", "<key>KeepAlive</key>", "<key>Program</key>"} {
		if strings.Contains(plist, forbidden) {
			t.Fatalf("plist unexpectedly contains %s", forbidden)
		}
	}
}

func TestLaunchdSchedulerCreateReadDelete(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "Library"), 0700); err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	s := launchdTaskScheduler{home: home, uid: "501", run: func(args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		return nil, nil
	}}
	spec := testLaunchdSpec()
	if err := s.Create(spec); err != nil {
		t.Fatal(err)
	}
	if err := s.Read(spec); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(spec); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(s.path(spec)); !os.IsNotExist(err) {
		t.Fatalf("plist after delete: %v", err)
	}
	got := strings.Join(func() []string {
		var all []string
		for _, c := range calls {
			all = append(all, strings.Join(c, " "))
		}
		return all
	}(), "\n")
	for _, command := range []string{"bootstrap gui/501", "print gui/501/agent-at-501-81c2a5d761ee4fc39628537ea1220b8d", "bootout gui/501/agent-at-501-81c2a5d761ee4fc39628537ea1220b8d"} {
		if !strings.Contains(got, command) {
			t.Fatalf("missing %q in %q", command, got)
		}
	}
}

func TestLaunchdCreateFailureAfterBootstrapIsUncertain(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "Library"), 0700); err != nil {
		t.Fatal(err)
	}
	s := launchdTaskScheduler{home: home, uid: "501", run: func(args ...string) ([]byte, error) { return nil, errors.New("interrupted") }}
	err := s.Create(testLaunchdSpec())
	var createErr *taskCreateError
	if !errors.As(err, &createErr) || !createErr.Uncertain {
		t.Fatalf("Create error = %#v", err)
	}
	if _, statErr := os.Lstat(s.path(testLaunchdSpec())); statErr != nil {
		t.Fatalf("plist was removed: %v", statErr)
	}
}

func TestLaunchdDeleteRefusesChangedPlist(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "Library"), 0700); err != nil {
		t.Fatal(err)
	}
	called := false
	s := launchdTaskScheduler{home: home, uid: "501", run: func(args ...string) ([]byte, error) { called = true; return nil, nil }}
	spec := testLaunchdSpec()
	if err := s.Create(spec); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.path(spec), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	called = false
	if err := s.Delete(spec); err == nil {
		t.Fatal("Delete accepted changed plist")
	}
	if called {
		t.Fatal("Delete called launchctl after plist verification failed")
	}
}

func TestLaunchdDeleteRefusesLiveServiceWithoutPlist(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "Library"), 0700); err != nil {
		t.Fatal(err)
	}
	s := launchdTaskScheduler{home: home, uid: "501", run: func(args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "print" {
			return nil, nil
		}
		return nil, errors.New("unexpected launchctl operation")
	}}
	if err := s.Delete(testLaunchdSpec()); err == nil || !strings.Contains(err.Error(), "service exists") {
		t.Fatalf("Delete error = %v", err)
	}
}

func TestPreparePersistentDispatch(t *testing.T) {
	zone := time.FixedZone("saved", 9*3600)
	due := time.Date(2026, 9, 16, 5, 4, 39, 0, zone)
	j := persistentJob{Request: options{At: due}}
	for _, tc := range []struct {
		name string
		now  time.Time
		wait time.Duration
		skip bool
	}{
		{"same minute waits", due.Add(-20 * time.Second), 20 * time.Second, false},
		{"same-year catch-up", due.Add(time.Hour), 0, false},
		{"earlier day", due.Add(-time.Hour), 0, true},
		{"earlier year", due.AddDate(-1, 0, 0), 0, true},
		{"later year", due.AddDate(1, 0, 0), 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wait, skip := preparePersistentDispatch(j, tc.now)
			if wait != tc.wait || skip != tc.skip {
				t.Fatalf("got (%s, %t), want (%s, %t)", wait, skip, tc.wait, tc.skip)
			}
		})
	}
}
