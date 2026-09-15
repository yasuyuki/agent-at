package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Synthetic reconstruction of the omissions reported by the Windows receiver,
// not a claim that these are captured native XML bytes.
func schedulerOmittedDefaults(definition string) string {
	for _, element := range []string{
		"<RunLevel>LeastPrivilege</RunLevel>", "<Enabled>true</Enabled>",
		"<StartWhenAvailable>false</StartWhenAvailable>", "<WakeToRun>false</WakeToRun>",
		"<RunOnlyIfIdle>false</RunOnlyIfIdle>", "<RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>",
	} {
		definition = strings.ReplaceAll(definition, element, "")
	}
	return `<?xml version="1.0" encoding="UTF-16"?>` + definition
}

func TestTaskXMLOmittedDefaults(t *testing.T) {
	spec := testTaskSpec()
	definition, err := taskXML(spec)
	if err != nil {
		t.Fatal(err)
	}
	omitted := schedulerOmittedDefaults(definition)
	if err := validateTaskXML(omitted, spec); err != nil {
		t.Fatal(err)
	}
	// IgnoreNew is also the documented default even though this native export
	// retained it. A nondefault limit or battery setting may never be inferred.
	if err := validateTaskXML(strings.ReplaceAll(omitted, "<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>", ""), spec); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ parent, name, good, bad string }{
		{"Principal", "RunLevel", "LeastPrivilege", "HighestAvailable"},
		{"TimeTrigger", "Enabled", "true", "false"},
		{"Settings", "Enabled", "true", "false"},
		{"Settings", "StartWhenAvailable", "false", "true"},
		{"Settings", "WakeToRun", "false", "true"},
		{"Settings", "RunOnlyIfIdle", "false", "true"},
		{"Settings", "RunOnlyIfNetworkAvailable", "false", "true"},
	} {
		t.Run(tc.parent+"/"+tc.name, func(t *testing.T) {
			for _, value := range []string{"", tc.bad, "invalid"} {
				element := "<" + tc.name + ">" + value + "</" + tc.name + ">"
				changed := strings.Replace(omitted, "</"+tc.parent+">", element+"</"+tc.parent+">", 1)
				if err := validateTaskXML(changed, spec); err == nil {
					t.Fatalf("accepted explicit %q", element)
				}
			}
			element := "<" + tc.name + ">" + tc.good + "</" + tc.name + ">"
			changed := strings.Replace(omitted, "</"+tc.parent+">", element+element+"</"+tc.parent+">", 1)
			if err := validateTaskXML(changed, spec); err == nil {
				t.Fatal("accepted duplicate")
			}
		})
	}
	for _, element := range []string{
		"<DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>",
		"<StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>",
		"<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>",
		"<UserId>" + spec.SID + "</UserId>",
		"<LogonType>InteractiveToken</LogonType>",
		"<StartBoundary>" + spec.At.Format(time.RFC3339) + "</StartBoundary>",
		"<Command>" + spec.Executable + "</Command>",
		"<Arguments>--internal-persist " + spec.ID + "</Arguments>",
	} {
		changed := strings.Replace(omitted, element, "", 1)
		if changed == omitted {
			t.Fatalf("fixture element missing: %s", element)
		}
		if err := validateTaskXML(changed, spec); err == nil {
			t.Fatalf("accepted missing %s", element)
		}
	}
}

type omittedXMLTasks struct{ *fakeTasks }

func (f omittedXMLTasks) Read(spec taskSpec) error {
	if err := f.fakeTasks.Read(spec); err != nil {
		return err
	}
	definition, err := taskXML(spec)
	if err != nil {
		return err
	}
	return validateTaskXML(schedulerOmittedDefaults(definition), spec)
}

func TestOmittedXMLRegistrationListAndRemoval(t *testing.T) {
	store, tasks, o := testJobStore(t)
	store.tasks = omittedXMLTasks{tasks}
	j, err := store.register(o, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := store.list(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "OS registered") || strings.Contains(out.String(), "inconsistent") {
		t.Fatal(out.String())
	}
	if err := store.remove(j.ID); err != nil {
		t.Fatal(err)
	}
	if len(tasks.jobs) != 0 {
		t.Fatal("task retained")
	}
	if present, err := exists(filepath.Join(store.root, j.ID)); err != nil || present {
		t.Fatalf("data retained: %v", err)
	}
}

func TestTaskXMLCOMStringEncodingDeclaration(t *testing.T) {
	spec := testTaskSpec()
	spec.Executable = `C:\日本語 & space\agent-at.exe`
	spec.Directory = `C:\作業 & space`
	body, err := taskXML(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, encoding := range []string{"UTF-16", "utf-16", "UTF-8"} {
		definition := `<?xml version="1.0" encoding="` + encoding + `"?>` + body
		// The real management boundary transports an already-decoded string
		// through JSON, not raw UTF-16 file bytes.
		wire, err := json.Marshal(map[string]any{"ok": true, "xml": definition})
		if err != nil {
			t.Fatal(err)
		}
		var response struct {
			XML string `json:"xml"`
		}
		if err = json.Unmarshal(wire, &response); err != nil {
			t.Fatal(err)
		}
		if err = validateTaskXML(response.XML, spec); err != nil {
			t.Fatalf("%s: %v", encoding, err)
		}
		altered := strings.Replace(response.XML, "<RunLevel>LeastPrivilege", "<RunLevel>HighestAvailable", 1)
		if err = validateTaskXML(altered, spec); err == nil {
			t.Fatal("encoding handling bypassed policy validation")
		}
	}
	if err := validateTaskXML(`<?xml version="1.0" encoding="unknown"?>`+body, spec); err == nil {
		t.Fatal("accepted unknown encoding")
	}
}

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
