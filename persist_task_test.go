package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTaskCreateError(t *testing.T) {
	err := errors.New("no")
	got := &taskCreateError{Err: err, Uncertain: true}
	if !errors.Is(got, err) || !got.Uncertain {
		t.Fatalf("unexpected create error: %#v", got)
	}
}

func TestOtherTaskSchedulerIsUnsupported(t *testing.T) {
	if newTaskScheduler() == nil {
		t.Fatal("nil scheduler")
	}
}

func TestTaskXMLRoundTripAndEscaping(t *testing.T) {
	spec := testTaskSpec()
	spec.ID = `id<&`
	spec.Executable = `C:\a&b\"x".exe`
	xmlText, err := taskXML(spec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(xmlText, `<Command>`+spec.Executable+`</Command>`) {
		t.Fatalf("unescaped command: %s", xmlText)
	}
	if err := validateTaskXML(xmlText, spec); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTaskXMLRejectsScheduleAndSettingsChanges(t *testing.T) {
	spec := testTaskSpec()
	xmlText, err := taskXML(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range [][2]string{
		{"<TimeTrigger>", "<TimeTrigger><Repetition><Interval>PT1M</Interval></Repetition>"},
		{"<TimeTrigger>", "<TimeTrigger><RandomDelay>PT1M</RandomDelay>"},
		{"<TimeTrigger>", "<TimeTrigger><EndBoundary>2026-09-17T00:00:00+09:00</EndBoundary>"},
		{"</Actions>", "<Exec><Command>x</Command></Exec></Actions>"},
		{"<Enabled>true</Enabled>", "<Enabled>false</Enabled>"},
		{"<Settings><Enabled>true", "<Settings><Enabled>false"},
		{"<RunOnlyIfNetworkAvailable>false", "<RunOnlyIfNetworkAvailable>true"},
		{"</Settings>", "<RestartOnFailure><Interval>PT1M</Interval><Count>0</Count></RestartOnFailure></Settings>"},
	} {
		if err := validateTaskXML(strings.Replace(xmlText, change[0], change[1], 1), spec); err == nil {
			t.Fatalf("accepted adverse definition %q", change[1])
		}
	}
}

func TestValidateTaskXMLPreservesPathWhitespace(t *testing.T) {
	spec := testTaskSpec()
	xmlText, err := taskXML(spec)
	if err != nil {
		t.Fatal(err)
	}
	xmlText = strings.Replace(xmlText, `<WorkingDirectory>C:\work &amp; test</WorkingDirectory>`, `<WorkingDirectory> C:\work &amp; test </WorkingDirectory>`, 1)
	if err := validateTaskXML(xmlText, spec); err == nil {
		t.Fatal("accepted altered working directory")
	}
}

func testTaskSpec() taskSpec {
	return taskSpec{ID: "81c2", SID: "S-1-5-21-123", Executable: `C:\Program Files\agent-at.exe`, Directory: `C:\work & test`, At: time.Date(2026, 9, 16, 5, 4, 3, 0, time.FixedZone("UTC+9", 9*3600))}
}
