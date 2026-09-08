package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
	cmd := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", `$ErrorActionPreference='Stop'; $sid=[System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value; foreach ($p in @($env:CODEX_AT_ACL_DIR, (Join-Path $env:CODEX_AT_ACL_DIR 'prompt.txt'))) { $acl=if ([System.IO.Directory]::Exists($p)) { [System.IO.Directory]::GetAccessControl($p) } else { [System.IO.File]::GetAccessControl($p) }; $rules=@($acl.GetAccessRules($true,$true,[System.Security.Principal.SecurityIdentifier])); if ($rules.Count -ne 1 -or $rules[0].IdentityReference.Value -ne $sid -or $rules[0].AccessControlType -ne 'Allow' -or $rules[0].FileSystemRights -ne 'FullControl') { throw 'Unexpected prompt ACL' }; if ($p -eq $env:CODEX_AT_ACL_DIR -and -not $acl.AreAccessRulesProtected) { throw 'Directory inherits ACL' } }`)
	cmd.Env = append(os.Environ(), "CODEX_AT_ACL_DIR="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ACL check: %v: %s", err, out)
	}
}

// Runs the actual Windows cmd.exe parser and a standard %* forwarding shim.
// Cross-compilation alone does not execute this test.
func TestCmdRoundTrip(t *testing.T) {
	o := helperOptions(t)
	data, err := os.ReadFile(o.Codex)
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
	o.Codex = shim
	o.Model = "model & %PATH% ! ^ (x)"
	o.AddDirs = append(o.AddDirs, o.CD+string(os.PathSeparator))
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
			if strings.Contains(strings.Join(got.Args, "|"), "--approve-for-me") == noApprove {
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

func TestConsoleLaunchAndCleanup(t *testing.T) {
	o := helperOptions(t)
	o.Close = true
	o.Prompt = strings.Repeat(o.Prompt, 2000)
	report := filepath.Join(t.TempDir(), "result.json")
	t.Setenv("CODEX_AT_TEST_REPORT", report)
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
	data, err := os.ReadFile(o.Codex)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(o.CD, "fake.exe"), data, 0700); err != nil {
		t.Fatal(err)
	}
	o.Codex = filepath.Join(o.CD, "fake.cmd")
	if err = os.WriteFile(o.Codex, []byte("@echo off\r\n\"%~dp0fake.exe\" %*\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	o.Headless = true
	t.Setenv("CODEX_AT_TEST_FAIL", "1")
	if code := runCodex(o, nil); code != 23 {
		t.Fatalf("cmd exit: %d", code)
	}
}
