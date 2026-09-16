package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// taskSpec is the small, non-secret part of a persisted job needed by the OS.
// The job payload itself remains in the caller's private job directory.
type taskSpec struct {
	ID, SID, Executable, Directory string
	At                             time.Time
}

type taskScheduler interface {
	Create(taskSpec) error
	Read(taskSpec) error
	Delete(taskSpec) error
}

// taskName includes the owner identity and the random job ID, keeping this
// product's tasks distinct from tasks created by other users or products.
func taskName(spec taskSpec) string { return "agent-at-" + spec.SID + "-" + spec.ID }

// taskCreateError tells callers whether it is safe to remove their new job
// payload after Create fails. Uncertain means Windows may have registered it.
type taskCreateError struct {
	Err       error
	Uncertain bool
}

func (e *taskCreateError) Error() string {
	if e == nil || e.Err == nil {
		return "create scheduled task failed"
	}
	return fmt.Sprintf("create scheduled task: %v", e.Err)
}

func (e *taskCreateError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

var errTaskSchedulerUnsupported = errors.New("persistent task scheduling is requires Windows, Linux systemd user timers, or macOS launchd")

type xmlNode struct {
	XMLName xml.Name
	Text    string    `xml:",chardata"`
	Nodes   []xmlNode `xml:",any"`
}

func (n xmlNode) child(name string) []xmlNode {
	var result []xmlNode
	for _, node := range n.Nodes {
		if node.XMLName.Local == name {
			result = append(result, node)
		}
	}
	return result
}

func one(n xmlNode, name string) (xmlNode, bool) {
	nodes := n.child(name)
	if len(nodes) != 1 {
		return xmlNode{}, false
	}
	return nodes[0], true
}

// text intentionally retains whitespace. It is significant in executable and
// working-directory paths and must not be normalized during ownership checks.
func text(n xmlNode, name string) (string, bool) {
	node, ok := one(n, name)
	return node.Text, ok
}

// Task Scheduler omits values equal to its defaults when exporting XML.
// Only absence receives a default: empty, duplicate or structured values do not.
func taskValue(n xmlNode, name, omitted string) (string, bool) {
	nodes := n.child(name)
	if len(nodes) == 0 {
		return omitted, true
	}
	if len(nodes) != 1 || len(nodes[0].Nodes) != 0 {
		return "", false
	}
	return nodes[0].Text, true
}

func taskXML(spec taskSpec) (string, error) {
	if strings.TrimSpace(spec.ID) == "" || strings.TrimSpace(spec.SID) == "" || spec.Executable == "" || spec.Directory == "" || spec.At.IsZero() {
		return "", errors.New("incomplete task specification")
	}
	escape := func(value string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(value))
		return b.String()
	}
	boundary := spec.At.Format("2006-01-02T15:04:05Z07:00")
	return `<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"><Principals><Principal id="Author"><UserId>` + escape(spec.SID) + `</UserId><LogonType>InteractiveToken</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals><Triggers><TimeTrigger><StartBoundary>` + escape(boundary) + `</StartBoundary><Enabled>true</Enabled></TimeTrigger></Triggers><Settings><Enabled>true</Enabled><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><AllowHardTerminate>true</AllowHardTerminate><StartWhenAvailable>false</StartWhenAvailable><RunOnlyIfIdle>false</RunOnlyIfIdle><RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable><WakeToRun>false</WakeToRun><ExecutionTimeLimit>PT0S</ExecutionTimeLimit></Settings><Actions Context="Author"><Exec><Command>` + escape(spec.Executable) + `</Command><Arguments>--internal-persist ` + escape(spec.ID) + `</Arguments><WorkingDirectory>` + escape(spec.Directory) + `</WorkingDirectory></Exec></Actions></Task>`, nil
}

func validateTaskXML(definition string, spec taskSpec) error {
	var task xmlNode
	decoder := xml.NewDecoder(strings.NewReader(definition))
	// RegisteredTask.XML is a COM string, already decoded by PowerShell and
	// transported as JSON into this Go string. Its UTF-16 declaration describes
	// the original COM representation, not the UTF-8 bytes read here. Do not
	// decode those bytes as UTF-16 a second time, or relax XML validation.
	decoder.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		if strings.EqualFold(charset, "UTF-16") {
			return input, nil
		}
		return nil, fmt.Errorf("unsupported task XML encoding %q", charset)
	}
	if err := decoder.Decode(&task); err != nil {
		return fmt.Errorf("parse task definition: %w", err)
	}
	if task.XMLName.Local != "Task" {
		return errors.New("unexpected task definition")
	}
	principals, ok := one(task, "Principals")
	if !ok {
		return errors.New("task principal is missing or ambiguous")
	}
	principal, ok := one(principals, "Principal")
	if !ok {
		return errors.New("task principal is missing or ambiguous")
	}
	if got, ok := text(principal, "UserId"); !ok || got != spec.SID {
		return errors.New("task owner differs")
	}
	if got, ok := text(principal, "LogonType"); !ok || got != "InteractiveToken" {
		return errors.New("task logon type differs")
	}
	// Default LUA is documented under Security Contexts for Tasks; the native
	// return also confirmed Principal.RunLevel == 0 with this element omitted.
	if got, ok := taskValue(principal, "RunLevel", "LeastPrivilege"); !ok || got != "LeastPrivilege" {
		return errors.New("task run level differs")
	}
	triggers, ok := one(task, "Triggers")
	if !ok || len(triggers.Nodes) != 1 || triggers.Nodes[0].XMLName.Local != "TimeTrigger" {
		return errors.New("task must have exactly one time trigger")
	}
	trigger := triggers.Nodes[0]
	if len(trigger.child("Repetition")) != 0 || len(trigger.child("RandomDelay")) != 0 || len(trigger.child("EndBoundary")) != 0 {
		return errors.New("task trigger contains a forbidden schedule change")
	}
	wantTime := spec.At.Format("2006-01-02T15:04:05Z07:00")
	if got, ok := text(trigger, "StartBoundary"); !ok || got != wantTime {
		return errors.New("task time differs")
	}
	if got, ok := taskValue(trigger, "Enabled", "true"); !ok || got != "true" {
		return errors.New("task trigger is disabled")
	}
	actions, ok := one(task, "Actions")
	if !ok || len(actions.Nodes) != 1 || actions.Nodes[0].XMLName.Local != "Exec" {
		return errors.New("task must have exactly one exec action")
	}
	execAction := actions.Nodes[0]
	if got, ok := text(execAction, "Command"); !ok || got != spec.Executable {
		return errors.New("task executable differs")
	}
	if got, ok := text(execAction, "Arguments"); !ok || got != "--internal-persist "+spec.ID {
		return errors.New("task arguments differ")
	}
	if got, ok := text(execAction, "WorkingDirectory"); !ok || got != spec.Directory {
		return errors.New("task directory differs")
	}
	settings, ok := one(task, "Settings")
	if !ok {
		return errors.New("task settings missing")
	}
	want := map[string]string{"Enabled": "true", "MultipleInstancesPolicy": "IgnoreNew", "DisallowStartIfOnBatteries": "false", "StopIfGoingOnBatteries": "false", "StartWhenAvailable": "false", "RunOnlyIfIdle": "false", "RunOnlyIfNetworkAvailable": "false", "WakeToRun": "false", "ExecutionTimeLimit": "PT0S"}
	// Microsoft Task Scheduler schema, settingsType. Defaults are OS values,
	// not desired values: omitted battery flags and the 72-hour time limit must
	// therefore still fail this job's contract.
	defaults := map[string]string{"Enabled": "true", "MultipleInstancesPolicy": "IgnoreNew", "DisallowStartIfOnBatteries": "true", "StopIfGoingOnBatteries": "true", "StartWhenAvailable": "false", "RunOnlyIfIdle": "false", "RunOnlyIfNetworkAvailable": "false", "WakeToRun": "false", "ExecutionTimeLimit": "PT72H"}
	for name, value := range want {
		if got, ok := taskValue(settings, name, defaults[name]); !ok || got != value {
			return fmt.Errorf("task setting %s differs", name)
		}
	}
	if len(settings.child("RestartOnFailure")) != 0 {
		return errors.New("task retry setting is not allowed")
	}
	return nil
}
