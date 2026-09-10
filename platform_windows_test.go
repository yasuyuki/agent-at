package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestPrepareConsole(t *testing.T) {
	for _, f := range []*os.File{os.Stdin, os.Stdout} {
		var mode uint32
		ok, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleMode").Call(f.Fd(), uintptr(unsafe.Pointer(&mode)))
		if ok == 0 {
			t.Skip("standard streams are not attached to a console")
		}
	}
	inputBefore, outputBefore := testConsoleCodePages()
	if inputBefore == 0 || outputBefore == 0 {
		t.Skip("test process has no console")
	}
	restore, err := prepareConsole()
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	inputUTF8, outputUTF8 := testConsoleCodePages()
	if inputUTF8 != 65001 || outputUTF8 != 65001 {
		t.Fatalf("UTF-8 code pages = input %d, output %d", inputUTF8, outputUTF8)
	}
	restore()
	inputAfter, outputAfter := testConsoleCodePages()
	if inputAfter != inputBefore || outputAfter != outputBefore {
		t.Fatalf("code pages not restored: input %d -> %d, output %d -> %d", inputBefore, inputAfter, outputBefore, outputAfter)
	}
}

func testConsoleCodePages() (uint32, uint32) {
	kernel := syscall.NewLazyDLL("kernel32.dll")
	input, _, _ := kernel.NewProc("GetConsoleCP").Call()
	if input == 0 {
		return 0, 0
	}
	output, _, _ := kernel.NewProc("GetConsoleOutputCP").Call()
	if output == 0 {
		return 0, 0
	}
	return uint32(input), uint32(output)
}

func TestPromptDirectoryACL(t *testing.T) {
	dir, err := promptTempDir()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(file, []byte("private request"), 0600); err != nil {
		t.Fatal(err)
	}
	// Inspect both objects: a protected directory alone is insufficient unless
	// its single user ACE propagates to the actual request file.
	cmd := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", `$ErrorActionPreference='Stop'; $sid=[System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value; foreach ($p in @($env:AGENT_AT_ACL_DIR, (Join-Path $env:AGENT_AT_ACL_DIR 'prompt.txt'))) { $acl=if ([System.IO.Directory]::Exists($p)) { [System.IO.Directory]::GetAccessControl($p) } else { [System.IO.File]::GetAccessControl($p) }; $rules=@($acl.GetAccessRules($true,$true,[System.Security.Principal.SecurityIdentifier])); if ($rules.Count -ne 1 -or $rules[0].IdentityReference.Value -ne $sid -or $rules[0].AccessControlType -ne 'Allow' -or $rules[0].FileSystemRights -ne 'FullControl') { throw 'Unexpected prompt ACL' }; if ($p -eq $env:AGENT_AT_ACL_DIR -and -not $acl.AreAccessRulesProtected) { throw 'Directory inherits ACL' } }`)
	cmd.Env = append(os.Environ(), "AGENT_AT_ACL_DIR="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ACL check: %v: %s", err, out)
	}
}

// Runs the actual Windows cmd.exe parser and a standard %* forwarding shim.
// Cross-compilation alone does not execute this test.
func TestCmdRoundTrip(t *testing.T) {
	o := helperOptions(t)
	data, err := os.ReadFile(o.Executable)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(o.CD, "fake.exe")
	if err = os.WriteFile(exe, data, 0700); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(o.CD, "fake & %PATH% ! ^.cmd")
	if err = os.WriteFile(shim, []byte("@echo off\r\n\"%~dp0fake.exe\" %*\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	o.Executable = shim
	o.Model = "model & %PATH% ! ^ (x)"
	o.AddDirs = append(o.AddDirs, o.CD+string(os.PathSeparator))
	for _, agent := range []string{"codex", "claude"} {
		o.Agent = agent
		for _, headless := range []bool{false, true} {
			for _, noApprove := range []bool{false, true} {
				o.Headless = headless
				o.NoApprove = noApprove
				cmd, cleanup, err := prepare(o)
				if err != nil {
					t.Fatal(err)
				}
				defer cleanup()
				var out, stderr bytes.Buffer
				cmd.Stdout = &out
				cmd.Stderr = &stderr
				if err = cmd.Run(); err != nil {
					t.Fatalf("%v: %s", err, stderr.String())
				}
				var got observation
				if err = json.Unmarshal(out.Bytes(), &got); err != nil {
					t.Fatalf("%v: %s", err, out.String())
				}
				if got.Dir != o.CD {
					t.Errorf("cwd changed: %q", got.Dir)
				}
				if headless {
					if got.Input != o.Prompt {
						t.Error("stdin changed")
					}
				} else if got.Prompt != o.Prompt {
					t.Error("prompt file changed")
				}
				if !strings.Contains(strings.Join(got.Args, "|"), "--model|"+o.Model) {
					t.Errorf("model changed: %q", got.Args)
				}
				found := false
				for _, a := range got.Args {
					if a == o.AddDirs[1] {
						found = true
					}
				}
				if !found {
					t.Errorf("trailing slash changed: %q", got.Args)
				}
				if strings.Contains(strings.Join(got.Args, "|"), approvalArgument(agent)) == noApprove {
					t.Error("approval flag changed")
				}
				cleanup()
				if got.File != "" {
					if _, err = os.Stat(filepath.Dir(got.File)); !os.IsNotExist(err) {
						t.Error("temporary directory remains")
					}
				}
			}
		}
	}
}
func TestConsoleLaunchAndCleanup(t *testing.T) {
	o := helperOptions(t)
	o.Close = true
	o.Prompt = strings.Repeat(o.Prompt, 2000)
	report := filepath.Join(t.TempDir(), "result.json")
	t.Setenv("AGENT_AT_TEST_REPORT", report)
	// Launch the same test executable through the production console protocol.
	if code := launchConsole(o); code != 0 {
		t.Fatalf("launch: %d", code)
	}
	deadline := time.After(30 * time.Second)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		b, err := os.ReadFile(report)
		var got observation
		if err == nil && json.Unmarshal(b, &got) == nil && got.File != "" {
			if got.InputCP != 65001 || got.OutputCP != 65001 {
				t.Fatalf("child code pages: %d/%d", got.InputCP, got.OutputCP)
			}
			if got.Prompt != o.Prompt {
				t.Fatal("console prompt changed")
			}
			if _, err := os.Stat(filepath.Dir(got.File)); os.IsNotExist(err) {
				return
			}
		}
		select {
		case <-deadline:
			t.Fatal("console did not execute and clean its prompt")
		case <-tick.C:
		}
	}
}

func TestCmdExitCode(t *testing.T) {
	o := helperOptions(t)
	data, err := os.ReadFile(o.Executable)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(o.CD, "fake.exe"), data, 0700); err != nil {
		t.Fatal(err)
	}
	o.Executable = filepath.Join(o.CD, "fake.cmd")
	if err = os.WriteFile(o.Executable, []byte("@echo off\r\n\"%~dp0fake.exe\" %*\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	o.Headless = true
	t.Setenv("AGENT_AT_TEST_FAIL", "1")
	if code := runAgent(o, nil); code != 23 {
		t.Fatalf("cmd exit: %d", code)
	}
}

func TestCmdResume(t *testing.T) {
	o := helperOptions(t)
	data, err := os.ReadFile(o.Executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(o.CD, "fake.exe"), data, 0700); err != nil {
		t.Fatal(err)
	}
	o.Executable = filepath.Join(o.CD, "fake.cmd")
	if err := os.WriteFile(o.Executable, []byte("@echo off\r\n\"%~dp0fake.exe\" %*\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	o.Resume = "01912345-6789-7abc-8def-0123456789ab"
	o.Prompt = "resume"
	for _, agent := range []string{"codex", "claude"} {
		o.Agent = agent
		for _, headless := range []bool{false, true} {
			o.Headless = headless
			cmd, cleanup, err := prepare(o)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			var out, stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("%v: %s", err, stderr.String())
			}
			var got observation
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			prompt := got.Prompt
			if headless {
				prompt = got.Input
			}
			if prompt != "resume" || got.File != "" {
				t.Fatalf("changed resume: %+v", got)
			}
			if agent == "claude" {
				if !strings.Contains(strings.Join(got.Args, "|"), "--resume="+o.Resume+"|--") {
					t.Fatalf("changed Claude session: %q", got.Args)
				}
			} else if len(got.Args) < 4 || got.Args[len(got.Args)-3] != "--" || got.Args[len(got.Args)-2] != o.Resume || got.Args[len(got.Args)-4] != "resume" {
				t.Fatalf("changed session: %q", got.Args)
			}
		}
	}
}
