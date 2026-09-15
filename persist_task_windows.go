//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

const taskPowerShell = `$ErrorActionPreference = 'Stop'
$utf8 = New-Object System.Text.UTF8Encoding($false)
$OutputEncoding = [Console]::OutputEncoding = $utf8
[Console]::InputEncoding = $utf8
$request = [Console]::In.ReadToEnd() | ConvertFrom-Json
$service = New-Object -ComObject 'Schedule.Service'
$service.Connect()
$root = $service.GetFolder('\')
try {
  switch ($request.operation) {
    'create' { [void]$root.RegisterTask($request.name, $request.xml, 2, $null, $null, 3, $null); @{ok=$true} | ConvertTo-Json -Compress }
    'read'   { @{ok=$true; xml=$root.GetTask($request.name).Xml} | ConvertTo-Json -Compress }
    'delete' { $root.DeleteTask($request.name, 0); @{ok=$true} | ConvertTo-Json -Compress }
    default  { throw "unknown task scheduler operation" }
  }
} catch {
  $exception = $_.Exception
  while ($null -ne $exception.InnerException) { $exception = $exception.InnerException }
  @{ok=$false; hresult=$exception.HResult; error=$exception.Message} | ConvertTo-Json -Compress
  exit 1
}`

type windowsTaskScheduler struct {
	run func(operation, name, definition string) (string, error)
}

const (
	hresultFileNotFound = uint32(0x80070002)
	hresultTaskNotFound = uint32(0x8004130f)
)

// taskSchedulerError is returned only after Task Scheduler supplied an HRESULT.
// Message is diagnostic only and is never used for control flow because it is localized.
type taskSchedulerError struct {
	HRESULT uint32
	Message string
}

func (e *taskSchedulerError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("Task Scheduler HRESULT 0x%08x", e.HRESULT)
	}
	return fmt.Sprintf("Task Scheduler HRESULT 0x%08x: %s", e.HRESULT, e.Message)
}

func newTaskScheduler() taskScheduler { return windowsTaskScheduler{run: runTaskPowerShell} }

func (s windowsTaskScheduler) Create(spec taskSpec) error {
	xmlText, err := taskXML(spec)
	if err != nil {
		return &taskCreateError{Err: err}
	}
	_, err = s.run("create", taskName(spec), xmlText)
	// Once PowerShell has been invoked, a transport failure can happen after
	// COM registered the task; callers must preserve the payload for inspection.
	return createResult(err)
}

func (s windowsTaskScheduler) Read(spec taskSpec) error {
	xmlText, err := s.run("read", taskName(spec), "")
	if err != nil {
		if taskNotFound(err) {
			return os.ErrNotExist
		}
		return err
	}
	return validateTaskXML(xmlText, spec)
}

func (s windowsTaskScheduler) Delete(spec taskSpec) error {
	_, err := s.run("delete", taskName(spec), "")
	if taskNotFound(err) {
		return os.ErrNotExist
	}
	return err
}

func createResult(err error) error {
	if err == nil {
		return nil
	}
	var schedulerErr *taskSchedulerError
	// A structured COM failure proves RegisterTask returned an error and did not
	// replace a collision (TASK_CREATE). Process/JSON failures are uncertain.
	return &taskCreateError{Err: err, Uncertain: !errors.As(err, &schedulerErr)}
}

func taskNotFound(err error) bool {
	var schedulerErr *taskSchedulerError
	return errors.As(err, &schedulerErr) && (schedulerErr.HRESULT == hresultFileNotFound || schedulerErr.HRESULT == hresultTaskNotFound)
}

func windowsPowerShellPath() (string, error) {
	var dir [32768]uint16
	getSystemDirectory := syscall.NewLazyDLL("kernel32.dll").NewProc("GetSystemDirectoryW")
	n, _, callErr := getSystemDirectory.Call(uintptr(unsafe.Pointer(&dir[0])), uintptr(len(dir)))
	if n == 0 {
		return "", callErr
	}
	if n >= uintptr(len(dir)) {
		return "", errors.New("Windows system directory path is too long")
	}
	return filepath.Join(syscall.UTF16ToString(dir[:n]), "WindowsPowerShell", "v1.0", "powershell.exe"), nil
}

func runTaskPowerShell(operation, name, definition string) (string, error) {
	in, err := json.Marshal(struct {
		Operation string `json:"operation"`
		Name      string `json:"name"`
		XML       string `json:"xml,omitempty"`
	}{operation, name, definition})
	if err != nil {
		return "", err
	}
	powershell, err := windowsPowerShellPath()
	if err != nil {
		return "", fmt.Errorf("find Windows PowerShell: %w", err)
	}
	cmd := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-Command", taskPowerShell)
	cmd.Stdin = bytes.NewReader(in)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err = cmd.Run()
	if err != nil && stderr.Len() != 0 {
		err = fmt.Errorf("Task Scheduler PowerShell: %s: %w", string(bytes.TrimSpace(stderr.Bytes())), err)
	}
	return decodeTaskResponse(out.Bytes(), err)
}

func decodeTaskResponse(data []byte, runErr error) (string, error) {
	var result struct {
		OK      *bool  `json:"ok"`
		HRESULT *int64 `json:"hresult"`
		Error   string `json:"error"`
		XML     string `json:"xml"`
	}
	if err := json.Unmarshal(data, &result); err != nil || result.OK == nil {
		return "", errors.Join(errors.New("Task Scheduler returned an invalid response; registration state unknown"), runErr)
	}
	if !*result.OK && result.HRESULT != nil && uint32(*result.HRESULT)&0x80000000 != 0 {
		return "", &taskSchedulerError{HRESULT: uint32(*result.HRESULT), Message: result.Error}
	}
	if runErr != nil {
		return "", fmt.Errorf("Task Scheduler PowerShell: %w", runErr)
	}
	if !*result.OK {
		return "", errors.New("Task Scheduler returned an invalid response")
	}
	return result.XML, nil
}
