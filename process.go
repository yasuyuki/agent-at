package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

func codexArgs(o options, prompt string) []string {
	var args []string
	if o.Headless {
		args = append(args, "exec")
	} else {
		args = append(args, "--no-alt-screen")
	}
	if !o.NoApprove {
		args = append(args, "--approve-for-me")
	}
	args = append(args, "--cd", o.CD)
	for _, d := range o.AddDirs {
		args = append(args, "--add-dir", d)
	}
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	if o.Resume != "" {
		args = append(args, "resume", "--", o.Resume)
		return append(args, prompt)
	}
	return append(args, "--", prompt)
}

func prepare(o options) (*exec.Cmd, func(), error) {
	cleanup := func() {}
	prompt := o.Prompt
	if o.Headless {
		prompt = "-"
	} else if o.Resume == "" {
		// Stay below cmd.exe's 8191-character limit for shims, and the Win32
		// 32767 UTF-16 command-line limit. Keep all interactive prompts in a file
		// for .cmd; use a file for .exe when its complete argument line is long.
		n := len(utf16.Encode([]rune(o.Codex))) + 3
		for _, a := range codexArgs(o, prompt) {
			n += 2*len(utf16.Encode([]rune(a))) + 3
		}
		if strings.EqualFold(filepath.Ext(o.Codex), ".cmd") || n >= 32767 {
			dir, err := promptTempDir()
			if err != nil {
				return nil, cleanup, err
			}
			cleanup = func() {
				if err := os.RemoveAll(dir); err != nil {
					fmt.Fprintln(os.Stderr, "codex-at: remove temporary prompt:", err)
				}
			}
			path := filepath.Join(dir, "prompt.txt")
			if err := os.WriteFile(path, []byte(o.Prompt), 0600); err != nil {
				cleanup()
				return nil, func() {}, err
			}
			o.AddDirs = append(o.AddDirs, dir)
			prompt = "Read the UTF-8 file at " + path + " and carry out its contents as my request."
		}
	}
	cmd, err := platformCommand(o.Codex, codexArgs(o, prompt))
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	cmd.Dir = o.CD
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if o.Headless {
		cmd.Stdin = strings.NewReader(o.Prompt)
	}
	return cmd, cleanup, nil
}

// started reports process creation, not completion of the prompt or authentication.
func runCodex(o options, started func(error)) int {
	restore, consoleErr := prepareConsole()
	defer restore()
	if consoleErr != nil {
		if started != nil {
			started(consoleErr)
		}
		fmt.Fprintln(os.Stderr, "codex-at:", consoleErr)
		return 1
	}
	if !o.Headless {
		// Catch Ctrl+C in the supervisor while Codex handles the same console /
		// terminal event. Keep the supervisor alive for cleanup and exit hold.
		// Ignore is unsuitable: Windows can inherit it into the child.
		interrupts := make(chan os.Signal, 1)
		signal.Notify(interrupts, os.Interrupt)
		defer signal.Stop(interrupts)
	}
	cmd, cleanup, err := prepare(o)
	defer cleanup()
	if err == nil {
		err = cmd.Start()
	}
	if started != nil {
		started(err)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "codex-at:", err)
		return 1
	}
	err = cmd.Wait()
	if err == nil {
		return 0
	}
	var e *exec.ExitError
	if errors.As(err, &e) {
		fmt.Fprintf(os.Stderr, "codex-at: Codex exited with code %d\n", e.ExitCode())
		return e.ExitCode()
	}
	fmt.Fprintln(os.Stderr, "codex-at:", err)
	return 1
}
