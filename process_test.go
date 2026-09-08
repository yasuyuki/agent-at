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
	Dir, Input, File, Prompt string
}

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--internal-console" {
		os.Exit(consoleChild(os.Args[2:]))
	}
	if os.Getenv("CODEX_AT_TEST_HELPER") == "1" {
		o := observation{Args: os.Args[1:]}
		o.Dir, _ = os.Getwd()
		if len(o.Args) > 0 && o.Args[0] == "exec" {
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
		if report := os.Getenv("CODEX_AT_TEST_REPORT"); report != "" {
			data, _ := json.Marshal(o)
			if err := os.WriteFile(report, data, 0600); err != nil {
				os.Exit(97)
			}
		}
		_ = json.NewEncoder(os.Stdout).Encode(o)
		fmtBytes := []byte("helper stderr\n")
		_, _ = os.Stderr.Write(fmtBytes)
		if os.Getenv("CODEX_AT_TEST_FAIL") == "1" {
			os.Exit(23)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func helperOptions(t *testing.T) options {
	t.Helper()
	t.Setenv("CODEX_AT_TEST_HELPER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "日本語 space & (x) %PATH% ! ^")
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return options{CD: dir, Codex: exe, Prompt: "日本語\n\"quoted\" & | < > %PATH% ! ^ \\ end\n", AddDirs: stringsFlag{dir}}
}
func TestPrepareRoundTrip(t *testing.T) {
	for _, headless := range []bool{false, true} {
		for _, long := range []bool{false, true} {
			for _, approve := range []bool{false, true} {
				o := helperOptions(t)
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
				if strings.Contains(strings.Join(got.Args, "|"), "--approve-for-me") != approve {
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
func TestExitCodeAndCleanup(t *testing.T) {
	o := helperOptions(t)
	o.Headless = true
	t.Setenv("CODEX_AT_TEST_FAIL", "1")
	if got := runCodex(o, nil); got != 23 {
		t.Fatalf("exit code %d", got)
	}
	o.Headless = false
	o.Prompt = strings.Repeat("long", 10000)
	o.Codex = filepath.Join(t.TempDir(), "missing.exe")
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
	if got := codexArgs(o, "-initial"); !reflect.DeepEqual(got, want) {
		t.Fatalf("%q", got)
	}
	o.Headless = true
	o.NoApprove = true
	want = []string{"exec", "--cd", "work", "--add-dir", "a", "--add-dir", "b", "--", "-"}
	if got := codexArgs(o, "-"); !reflect.DeepEqual(got, want) {
		t.Fatalf("%q", got)
	}
}
func TestParseOptions(t *testing.T) {
	o := helperOptions(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	base := []string{"--at", "00:01", "--codex", o.Codex, "--cd", o.CD}
	file := filepath.Join(t.TempDir(), "依頼 & %.txt")
	if err := os.WriteFile(file, []byte("\ufeff"+o.Prompt), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := parseOptions(append(base, "--prompt-file", file, "--add-dir", o.CD, "--headless", "--close-on-exit"), now, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if got.Prompt != o.Prompt || !got.Headless || !got.Close || !filepath.IsAbs(got.CD) || !filepath.IsAbs(got.Codex) {
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
