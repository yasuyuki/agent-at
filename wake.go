package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const wakeInstructions = "Return only the requested literal text. Do not use tools or perform other work."

func validateWake(o options, set map[string]bool, positional int) error {
	if !o.Wake {
		if set["wake-text"] || set["wake-timeout"] {
			return errors.New("--wake-text and --wake-timeout require --wake")
		}
		return nil
	}
	for _, name := range []string{"resume", "prompt-file", "cd", "add-dir"} {
		if set[name] {
			return fmt.Errorf("--wake cannot be combined with --%s", name)
		}
	}
	if positional != 0 || (set["headless"] && !o.Headless) || o.NewConsole || o.Close {
		return errors.New("--wake requires headless execution without a prompt, new console or close-on-exit")
	}
	if o.WakeTimeout <= 0 {
		return errors.New("--wake-timeout must be positive")
	}
	if !utf8.ValidString(o.WakeText) || strings.ContainsAny(o.WakeText, "\x00\r\n") || strings.TrimSpace(o.WakeText) == "" {
		return errors.New("--wake-text must be nonblank, one-line UTF-8 without NUL")
	}
	return nil
}

func wakePrompt(text string) string {
	b, _ := json.Marshal(text)
	return "Return exactly this text: " + string(b)
}

// Preparation happens once, before waiting. It never starts the CLI or reads its
// normal configuration, skills, credentials or model list.
func prepareWake(o options) (*exec.Cmd, func(), error) {
	noop := func() {}
	dir, err := promptTempDir()
	if err != nil {
		return nil, noop, err
	}
	cleanup := func() {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "agent-at: remove wake directory:", err)
		}
	}
	if o.Agent == "codex" {
		if err := os.WriteFile(filepath.Join(dir, "instructions.txt"), []byte(wakeInstructions), 0600); err != nil {
			cleanup()
			return nil, noop, err
		}
	}
	cmd, err := platformCommand(o.Executable, wakeArgs(o))
	if err != nil {
		cleanup()
		return nil, noop, err
	}
	cmd.Dir = dir
	cmd.Env = wakeEnvironment(cmd.Environ())
	cmd.Stdin = strings.NewReader(o.Prompt)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd, cleanup, nil
}

func scheduleWake(o options) int {
	cmd, cleanup, err := prepareWake(o)
	defer cleanup()
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent-at:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	model := o.Model
	if model == "" {
		model = "CLI default (user settings skipped)"
	}
	fmt.Fprintf(os.Stderr, "Wake scheduled for %s (%s); agent=%s model=%s. Wake skips user settings; specify --model to target a model. Keep this timer open; Ctrl+C cancels.\n", o.At.Format(time.RFC3339), o.At.Location(), o.Agent, model)
	code, err := schedule(ctx, wallClock{}, o.At, func() int {
		if err := authorizeWakeCommand(ctx, o, cmd); err != nil {
			fmt.Fprintln(os.Stderr, "agent-at: wake authentication unsupported:", err)
			return 1
		}
		return runWake(ctx, cmd, o.WakeTimeout, os.Stderr)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent-at: wake cancelled before launch")
	}
	return code
}

func runWake(ctx context.Context, cmd *exec.Cmd, limit time.Duration, diagnostic io.Writer) int {
	code, _ := runWakeResult(ctx, cmd, limit, diagnostic)
	return code
}

// Share the same runner with persisted jobs, retaining a failure category even
// when a child's own exit code happens to equal the timeout code.
func runWakeResult(ctx context.Context, cmd *exec.Cmd, limit time.Duration, diagnostic io.Writer) (int, string) {
	if ctx.Err() != nil {
		return 130, "cancelled"
	}
	started := time.Now()
	terminate, release, err := startWakeProcess(cmd)
	if err != nil {
		fmt.Fprintln(diagnostic, "agent-at: wake launch failed:", err)
		return 1, "launch_failed"
	}
	defer release()
	fmt.Fprintf(diagnostic, "Wake launched at %s\n", started.Format(time.RFC3339Nano))
	timer := time.NewTimer(limit)
	defer timer.Stop()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	code, status := 0, "completed"
	select {
	case err = <-done:
		if err != nil {
			code, status = 1, "failed"
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				code = exit.ExitCode()
			}
			fmt.Fprintln(diagnostic, "agent-at:", err)
		}
	case <-ctx.Done():
		code, status = 130, "cancelled"
	case <-timer.C:
		code, status = 124, "timeout"
	}
	if status == "cancelled" || status == "timeout" {
		if err := terminate(); err != nil {
			fmt.Fprintln(diagnostic, "agent-at: terminate wake:", err)
		}
		<-done // Reap the child before its temporary directory is removed.
	}
	fmt.Fprintf(diagnostic, "wake request %s (exit %d, elapsed %s)\n", status, code, time.Since(started).Round(time.Millisecond))
	return code, status
}
