package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

type observation struct {
	Args                     []string
	InputCP, OutputCP        uint32
	Dir, Input, File, Prompt string
}

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--internal-console" {
		os.Exit(consoleChild(os.Args[2:]))
	}
	if os.Getenv("AGENT_AT_TEST_HELPER") == "1" {
		o := observation{Args: os.Args[1:]}
		o.InputCP, o.OutputCP = testConsoleCodePages()
		o.Dir, _ = os.Getwd()
		if len(o.Args) > 0 && (o.Args[0] == "exec" || o.Args[0] == "--print") {
			b, _ := io.ReadAll(os.Stdin)
			o.Input = string(b)
		} else if len(o.Args) > 0 {
			p := o.Args[len(o.Args)-1]
			const prefix = "Read the UTF-8 file at "
			const suffix = " and carry out its contents as my request."
			if strings.HasPrefix(p, prefix) && strings.HasSuffix(p, suffix) {
				o.File = strings.TrimSuffix(strings.TrimPrefix(p, prefix), suffix)
				b, err := os.ReadFile(o.File)
				if err != nil {
					os.Exit(98)
				}
				o.Prompt = string(b)
			} else {
				o.Prompt = p
			}
		}
		if report := os.Getenv("AGENT_AT_TEST_REPORT"); report != "" {
			data, _ := json.Marshal(o)
			if err := os.WriteFile(report, data, 0600); err != nil {
				os.Exit(97)
			}
		}
		_ = json.NewEncoder(os.Stdout).Encode(o)
		fmtBytes := []byte("helper stderr\n")
		_, _ = os.Stderr.Write(fmtBytes)
		if os.Getenv("AGENT_AT_TEST_FAIL") == "1" {
			os.Exit(23)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func helperOptions(t *testing.T) options {
	t.Helper()
	t.Setenv("AGENT_AT_TEST_HELPER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "日本語 space & (x) %PATH% ! ^")
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return options{CD: dir, Executable: exe, Agent: "codex", Prompt: "日本語\n\"quoted\" & | < > %PATH% ! ^ \\ end\n", AddDirs: stringsFlag{dir}}
}
func TestPrepareRoundTrip(t *testing.T) {
	for _, agent := range []string{"codex", "claude"} {
		for _, headless := range []bool{false, true} {
			for _, long := range []bool{false, true} {
				for _, approve := range []bool{false, true} {
					o := helperOptions(t)
					o.Agent = agent
					o.Headless = headless
					o.NoApprove = !approve
					if long {
						o.Prompt = strings.Repeat(o.Prompt, 2000)
					}
					o.Model = "test-model"
					cmd, cleanup, err := prepare(o)
					if err != nil {
						t.Fatal(err)
					}
					defer cleanup()
					var stdout, stderr bytes.Buffer
					cmd.Stdout = &stdout
					cmd.Stderr = &stderr
					if err = cmd.Run(); err != nil {
						t.Fatalf("%v: %s", err, stderr.String())
					}
					var got observation
					if err = json.Unmarshal(stdout.Bytes(), &got); err != nil {
						t.Fatal(err)
					}
					if got.Dir != o.CD {
						t.Errorf("cwd: %q != %q", got.Dir, o.CD)
					}
					if headless {
						if got.Input != o.Prompt {
							t.Error("stdin changed")
						}
					} else if got.Prompt != o.Prompt {
						t.Error("prompt changed")
					}
					if strings.Contains(strings.Join(got.Args, "|"), approvalArgument(agent)) != approve {
						t.Error("approval flag changed")
					}
					if !strings.Contains(strings.Join(got.Args, "|"), "--model|test-model") {
						t.Error("model missing")
					}
					if stderr.String() != "helper stderr\n" {
						t.Error("stderr changed")
					}
					cleanup()
					if got.File != "" {
						if _, err = os.Stat(filepath.Dir(got.File)); !os.IsNotExist(err) {
							t.Errorf("temporary directory retained: %v", err)
						}
					}
				}
			}
		}
	}
}
func TestExitCodeAndCleanup(t *testing.T) {
	o := helperOptions(t)
	o.Headless = true
	t.Setenv("AGENT_AT_TEST_FAIL", "1")
	if got := runAgent(o, nil); got != 23 {
		t.Fatalf("exit code %d", got)
	}
	o.Headless = false
	o.Prompt = strings.Repeat("long", 10000)
	o.Executable = filepath.Join(t.TempDir(), "missing.exe")
	cmd, cleanup, err := prepare(o)
	if err != nil {
		t.Fatal(err)
	}
	var dir string
	for i, a := range cmd.Args {
		if a == "--add-dir" {
			dir = cmd.Args[i+1]
		}
	}
	if err = cmd.Start(); err == nil {
		t.Fatal("started missing executable")
	}
	cleanup()
	if dir == "" {
		t.Fatal("no temporary dir")
	}
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("not cleaned: %v", err)
	}
}
func TestArguments(t *testing.T) {
	o := options{CD: "work", AddDirs: stringsFlag{"a", "b"}}
	want := []string{"--no-alt-screen", "--approve-for-me", "--cd", "work", "--add-dir", "a", "--add-dir", "b", "--", "-initial"}
	if got := agentArgs(o, "-initial"); !reflect.DeepEqual(got, want) {
		t.Fatalf("%q", got)
	}
	o.Headless = true
	o.NoApprove = true
	want = []string{"exec", "--cd", "work", "--add-dir", "a", "--add-dir", "b", "--", "-"}
	if got := agentArgs(o, "-"); !reflect.DeepEqual(got, want) {
		t.Fatalf("%q", got)
	}
}
func TestParseOptions(t *testing.T) {
	o := helperOptions(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	base := []string{"--at", "00:01", "--agent-path", o.Executable, "--cd", o.CD}
	file := filepath.Join(t.TempDir(), "依頼 & %.txt")
	if err := os.WriteFile(file, []byte("\ufeff"+o.Prompt), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := parseOptions(append(base, "--prompt-file", file, "--add-dir", o.CD, "--headless", "--close-on-exit"), now, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if got.Prompt != o.Prompt || !got.Headless || !got.Close || !filepath.IsAbs(got.CD) || !filepath.IsAbs(got.Executable) {
		t.Fatalf("incorrect options: %+v", got)
	}
	cases := [][]string{{}, {""}, {"one", "two"}, {"--prompt-file", file, "text"}, {"--prompt-file", file + "missing"}, {"--cd", file, "text"}, {"--add-dir", file, "text"}, {"--unknown", "x"}, {"\x00"}}
	for _, tail := range cases {
		if _, err := parseOptions(append(append([]string{}, base...), tail...), now, io.Discard); err == nil {
			t.Errorf("accepted %q", tail)
		}
	}
	for _, b := range [][]byte{{0xff}, {0xef, 0xbb, 0xbf}, []byte(" \n\t")} {
		os.WriteFile(file, b, 0600)
		if _, err := parseOptions(append(base, "--prompt-file", file), now, io.Discard); err == nil {
			t.Errorf("accepted bad file %q", b)
		}
	}
	if runtime.GOOS != "windows" {
		if _, err := parseOptions(append(base, "prompt"), now, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResume(t *testing.T) {
	for _, agent := range []string{"codex", "claude"} {
		o := helperOptions(t)
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		base := []string{"--agent", agent, "--agent-path", o.Executable, "--cd", o.CD}
		id := "01912345-6789-7abc-8def-0123456789ab"
		for _, headless := range []bool{false, true} {
			args := append(append([]string{}, base...), "--resume", id)
			if headless {
				args = append(args, "--headless")
			}
			got, err := parseOptions(args, now, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			if got.Prompt != "resume" || got.Resume != id || !got.At.Equal(now) {
				t.Fatalf("resume options: %+v", got)
			}
			cmd, cleanup, err := prepare(got)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = io.Discard
			if err := cmd.Run(); err != nil {
				t.Fatal(err)
			}
			var seen observation
			if err := json.Unmarshal(stdout.Bytes(), &seen); err != nil {
				t.Fatal(err)
			}
			prompt := "resume"
			if headless {
				prompt = "-"
				if seen.Input != "resume" {
					t.Fatalf("input: %q", seen.Input)
				}
			}
			wantTail := []string{"resume", "--", id, prompt}
			if agent == "claude" {
				wantTail = []string{"--resume=" + id, "--"}
				if !headless {
					wantTail = append(wantTail, "resume")
				}
			}
			if len(seen.Args) < len(wantTail) || !reflect.DeepEqual(seen.Args[len(seen.Args)-len(wantTail):], wantTail) {
				t.Fatalf("args: %q", seen.Args)
			}
			if seen.File != "" {
				t.Fatal("resume must not use a file instruction")
			}
		}
		got, err := parseOptions(append(append([]string{}, base...), "--at", "00:01", "--resume", id, "--new-console"), now, io.Discard)
		if err != nil || !got.At.Equal(now.Add(time.Minute)) || !got.NewConsole {
			t.Fatalf("scheduled resume: %+v, %v", got, err)
		}
		for _, tail := range [][]string{{"--resume", ""}, {"--resume", " "}, {"--resume", "-"}, {"--resume", "bad\x00id"}, {"--resume", id, "other prompt"}, {"--resume", id, "--prompt-file", "missing"}} {
			if _, err := parseOptions(append(append([]string{}, base...), tail...), now, io.Discard); err == nil {
				t.Fatalf("accepted %q", tail)
			}
		}
	}
}
func TestDefaultResumeWaitsForAgent(t *testing.T) {
	o := helperOptions(t)
	t.Setenv("AGENT_AT_TEST_FAIL", "1")
	// The default must run in the current terminal and return the child's exit,
	// not acknowledge creation of a detached dedicated window with exit zero.
	if code := run([]string{"--agent-path", o.Executable, "--cd", o.CD, "--resume", "01912345-6789-7abc-8def-0123456789ab"}); code != 23 {
		t.Fatalf("default resume exit: %d", code)
	}
}

func approvalArgument(agent string) string {
	if agent == "claude" {
		return "--permission-mode|auto"
	}
	return "--approve-for-me"
}

func TestClaudeArguments(t *testing.T) {
	o := options{Agent: "claude", CD: "work", AddDirs: stringsFlag{"a", "b"}, Model: "test-model"}
	want := []string{"--permission-mode", "auto", "--add-dir", "a", "b", "--model", "test-model", "--", "-initial"}
	if got := agentArgs(o, "-initial"); !reflect.DeepEqual(got, want) {
		t.Fatalf("interactive: %q", got)
	}
	o.Headless = true
	o.NoApprove = true
	o.Resume = "--option-like-session"
	want = []string{"--print", "--add-dir", "a", "b", "--model", "test-model", "--resume=--option-like-session", "--"}
	if got := agentArgs(o, "-"); !reflect.DeepEqual(got, want) {
		t.Fatalf("headless resume: %q", got)
	}
}

func TestAgentSelection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("PATHEXT", ".EXE")
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, agent := range []string{"codex", "claude"} {
		name := agent
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("fake"), 0700); err != nil {
			t.Fatal(err)
		}
		args := []string{"--agent", agent, "--at", "00:01", "--cd", dir, "prompt"}
		got, err := parseOptions(args, now, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		if got.Agent != agent || got.Executable != path {
			t.Fatalf("selected wrong executable: %+v", got)
		}
	}
	got, err := parseOptions([]string{"--at", "00:01", "--cd", dir, "prompt"}, now, io.Discard)
	if err != nil || got.Agent != "codex" {
		t.Fatalf("default: %+v %v", got, err)
	}
	for _, agent := range []string{"", "other", "Claude"} {
		if _, err := parseOptions([]string{"--agent", agent, "--at", "00:01", "prompt"}, now, io.Discard); err == nil {
			t.Fatalf("accepted agent %q", agent)
		}
	}
}
