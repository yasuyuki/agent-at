package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWakeOptions(t *testing.T) {
	exe, _ := os.Executable()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	base := []string{"--wake", "--at", "01:00", "--agent-path", exe}
	for _, args := range [][]string{
		{}, {"--headless", "--no-auto-approve=false"},
		{"--agent", "claude", "--model", "some-model", "--wake-text", "日本語 \" & | %PATH% ! ^ \\"},
	} {
		o, err := parseOptions(append(append([]string{}, base...), args...), now, io.Discard)
		if err != nil || !o.Headless || o.WakeTimeout != 2*time.Minute || !strings.HasPrefix(o.Prompt, "Return exactly this text: ") {
			t.Fatalf("%v: %+v %v", args, o, err)
		}
		var literal string
		if err := json.Unmarshal([]byte(strings.TrimPrefix(o.Prompt, "Return exactly this text: ")), &literal); err != nil || literal != o.WakeText {
			t.Fatalf("literal lost: %q %v", literal, err)
		}
	}
	for _, args := range [][]string{
		{"--resume", ""}, {"--prompt-file", ""}, {"--cd", ""}, {"--add-dir", ""},
		{"--headless=false"}, {"--new-console"}, {"--close-on-exit"}, {"--", "prompt"},
		{"--wake-text", " "}, {"--wake-text", "a\nb"}, {"--wake-text", "\r"}, {"--wake-text", "a\x00b"}, {"--wake-text", "\xff"},
		{"--wake-timeout", "0s"}, {"--wake-timeout", "-1s"}, {"--wake-timeout", "nonsense"},
	} {
		if _, err := parseOptions(append(append([]string{}, base...), args...), now, io.Discard); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	for _, args := range [][]string{
		{"--wake", "--agent-path", exe},
		{"--at", "01:00", "--wake-text", "ok", "prompt"},
		{"--at", "01:00", "--wake-timeout", "2m", "prompt"},
	} {
		if _, err := parseOptions(args, now, io.Discard); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestWakePreparedOnce(t *testing.T) {
	for _, agent := range []string{"codex", "claude"} {
		t.Run(agent, func(t *testing.T) {
			o := helperOptions(t)
			o.Agent, o.Wake, o.Headless = agent, true, true
			o.Model, o.Prompt = "test-model", wakePrompt(`日本語 "quoted" & | < > %PATH% ! ^ \`)
			cmd, cleanup, err := prepareWake(o)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			if cmd.Dir == o.CD {
				t.Fatal("inherited project cwd")
			}
			entries, err := os.ReadDir(cmd.Dir)
			if err != nil {
				t.Fatal(err)
			}
			wantFiles := 0
			if agent == "codex" {
				wantFiles = 1
				b, err := os.ReadFile(filepath.Join(cmd.Dir, "instructions.txt"))
				if err != nil || string(b) != wakeInstructions {
					t.Fatal("missing fixed instructions", err)
				}
			}
			if len(entries) != wantFiles {
				t.Fatalf("unexpected cwd files %v", entries)
			}
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			c := &fakeClock{now: time.Now(), steps: []time.Duration{-time.Hour, 2 * time.Hour}}
			launches := 0
			code, err := schedule(context.Background(), c, c.now.Add(time.Second), func() int {
				launches++
				return runWake(context.Background(), cmd, time.Minute, io.Discard)
			})
			if err != nil || code != 0 || launches != 1 || c.waits != 2 {
				t.Fatalf("%d %v %d %s", code, err, launches, stderr.String())
			}
			var got observation
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Input != o.Prompt || got.Dir != cmd.Dir || !reflect.DeepEqual(got.Args, wakeArgs(o)) {
				t.Fatalf("unexpected child: %+v", got)
			}
			cleanup()
			if _, err := os.Stat(cmd.Dir); !os.IsNotExist(err) {
				t.Fatalf("cwd survives: %v", err)
			}
		})
	}
}

func TestWakeEnvironment(t *testing.T) {
	input := []string{"HOME=/auth", "USERPROFILE=C:\\auth", "CODEX_HOME=/codex", "CLAUDE_CONFIG_DIR=/claude", "HTTPS_PROXY=https://proxy", "SSL_CERT_FILE=/ca", "OPENAI_API_KEY=secret", "ANTHROPIC_API_KEY=secret", "CLAUDE_CODE_USE_BEDROCK=1", "CLAUDE_CODE_SIMPLE=1"}
	copyInput := append([]string{}, input...)
	want := append(append([]string{}, input[:6]...), "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1")
	if got := wakeEnvironment(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected environment %v", got)
	}
	if !reflect.DeepEqual(input, copyInput) {
		t.Fatal("mutated parent environment")
	}
}

func TestWakeExitAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		o := helperOptions(t)
		o.Prompt = wakePrompt("ok")
		if fail {
			t.Setenv("AGENT_AT_TEST_FAIL", "1")
		}
		cmd, cleanup, err := prepareWake(o)
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
		want := 0
		if fail {
			want = 23
		}
		if got := runWake(context.Background(), cmd, time.Minute, io.Discard); got != want {
			t.Fatalf("code %d want %d", got, want)
		}
		cleanup()
	}
	o := helperOptions(t)
	o.Executable = filepath.Join(t.TempDir(), "missing.exe")
	cmd, cleanup, err := prepareWake(o)
	if err != nil {
		t.Fatal(err)
	}
	if got := runWake(context.Background(), cmd, time.Minute, io.Discard); got != 1 {
		t.Fatalf("launch failure %d", got)
	}
	cleanup()
	if _, err := os.Stat(cmd.Dir); !os.IsNotExist(err) {
		t.Fatal("failed launch cwd survives")
	}
}

type wakeCancelWriter struct{ cancel context.CancelFunc }

func (w wakeCancelWriter) Write(b []byte) (int, error) {
	w.cancel()
	return len(b), nil
}

func TestWakeTimeoutAndCancellation(t *testing.T) {
	for _, mode := range []string{"timeout", "running-cancel", "before-launch"} {
		t.Run(mode, func(t *testing.T) {
			o := helperOptions(t)
			t.Setenv("AGENT_AT_TEST_WAIT", "1")
			cmd, cleanup, err := prepareWake(o)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
			limit, want := time.Minute, 130
			switch mode {
			case "timeout":
				limit, want = 50*time.Millisecond, 124
			case "running-cancel":
				cmd.Stdout = wakeCancelWriter{cancel}
			case "before-launch":
				cancel()
			}
			if got := runWake(ctx, cmd, limit, io.Discard); got != want {
				t.Fatalf("exit %d want %d", got, want)
			}
			if mode == "before-launch" && cmd.Process != nil {
				t.Fatal("started cancelled request")
			}
			if mode != "before-launch" && cmd.ProcessState == nil {
				t.Fatal("child not reaped")
			}
			cleanup()
			if _, err := os.Stat(cmd.Dir); !os.IsNotExist(err) {
				t.Fatal("temporary cwd survived", err)
			}
		})
	}
}
