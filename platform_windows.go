package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// cmd.exe and CRT quoting are different. Expand quoted data from environment
// variables with delayed expansion disabled. Never put prompt text in cmd code.
// Trusted .cmd shims must forward %* without CALL or a second evaluation.
func platformCommand(path string, args []string) (*exec.Cmd, error) {
	if !strings.EqualFold(filepath.Ext(path), ".cmd") {
		cmd := exec.Command(path, args...)
		line := syscall.EscapeArg(path)
		for _, a := range args {
			line += " " + syscall.EscapeArg(a)
		}
		if len(utf16.Encode([]rune(line))) >= 32767 {
			return nil, fmt.Errorf("Codex command line exceeds the Windows limit; shorten paths or reduce additional directories")
		}
		return cmd, nil
	}
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if !filepath.IsAbs(shell) {
		return nil, fmt.Errorf("SystemRoot must identify the Windows installation")
	}
	cmd := exec.Command(shell)
	env := os.Environ()
	// Replace, rather than duplicate, inherited names (Windows is case-insensitive).
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(strings.ToUpper(env[i]), "CODEX_AT_ARG_") {
			env = append(env[:i], env[i+1:]...)
		}
	}
	values := append([]string{path}, args...)
	var refs []string
	expanded := 0
	for i, value := range values {
		if strings.ContainsAny(value, "\x00\r\n\"") {
			return nil, fmt.Errorf(".cmd arguments must not contain NUL, newline or double quote")
		}
		// Force quotes even for values without spaces. Double trailing backslashes
		// for the eventual native argv decoder, except the batch filename itself.
		quoted := value
		if i > 0 {
			quoted += strings.Repeat("\\", len(value)-len(strings.TrimRight(value, "\\")))
		}
		name := fmt.Sprintf("CODEX_AT_ARG_%d", i)
		env = append(env, name+"="+quoted)
		refs = append(refs, "\"%"+name+"%\"")
		expanded += len(utf16.Encode([]rune(quoted))) + 3
	}
	line := strings.Join(refs, " ")
	if expanded+len(shell)+32 >= 8191 || len(line)+len(shell)+32 >= 8191 {
		return nil, fmt.Errorf("Codex .cmd command line exceeds cmd.exe's limit; shorten paths or use a native .exe")
	}
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: syscall.EscapeArg(shell) + " /d /v:off /s /c \"" + line + "\""}
	return cmd, nil
}

type launchResult struct{ Error string }

// prepareConsole selects UTF-8 for an interactive Windows console while Codex
// runs. Console code pages belong to the console rather than this process, so
// callers must invoke the returned function when the session is complete.
func prepareConsole() (func(), error) {
	noop := func() {}
	in, out := os.Stdin, os.Stdout
	if in == nil || out == nil {
		return noop, nil
	}
	kernel := syscall.NewLazyDLL("kernel32.dll")
	getMode := kernel.NewProc("GetConsoleMode")
	for _, f := range []*os.File{in, out} {
		var mode uint32
		ok, _, _ := getMode.Call(f.Fd(), uintptr(unsafe.Pointer(&mode)))
		if ok == 0 {
			// Standard streams can be files, pipes, or absent. There is no
			// console state to change in that case.
			return noop, nil
		}
	}
	getInput := kernel.NewProc("GetConsoleCP")
	getOutput := kernel.NewProc("GetConsoleOutputCP")
	setInput := kernel.NewProc("SetConsoleCP")
	setOutput := kernel.NewProc("SetConsoleOutputCP")
	inputCP, _, inputErr := getInput.Call()
	if inputCP == 0 {
		return noop, inputErr
	}
	outputCP, _, outputErr := getOutput.Call()
	if outputCP == 0 {
		return noop, outputErr
	}
	const utf8CodePage = 65001
	if ok, _, err := setInput.Call(utf8CodePage); ok == 0 {
		return noop, err
	}
	if ok, _, err := setOutput.Call(utf8CodePage); ok == 0 {
		// Avoid leaving a half-configured console when output setup fails.
		_, _, _ = setInput.Call(inputCP)
		return noop, err
	}
	return func() {
		// Best effort: process exit also releases a dedicated console, while
		// an attached console must regain its caller's prior settings.
		_, _, _ = setInput.Call(inputCP)
		_, _, _ = setOutput.Call(outputCP)
	}, nil
}

func launchConsole(o options) int {
	requestR, requestW, err := os.Pipe()
	if err != nil {
		return launchError(err)
	}
	defer requestR.Close()
	defer requestW.Close()
	resultR, resultW, err := os.Pipe()
	if err != nil {
		return launchError(err)
	}
	defer resultR.Close()
	defer resultW.Close()
	// AdditionalInheritedHandles must explicitly be inheritable on Windows.
	for _, f := range []*os.File{requestR, resultW} {
		if err = syscall.SetHandleInformation(syscall.Handle(f.Fd()), syscall.HANDLE_FLAG_INHERIT, syscall.HANDLE_FLAG_INHERIT); err != nil {
			return launchError(err)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return launchError(err)
	}
	cmd := exec.Command(exe, "--internal-console", strconv.FormatUint(uint64(requestR.Fd()), 10), strconv.FormatUint(uint64(resultW.Fd()), 10))
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags:              0x00000010, // CREATE_NEW_CONSOLE
		AdditionalInheritedHandles: []syscall.Handle{syscall.Handle(requestR.Fd()), syscall.Handle(resultW.Fd())},
	}
	if err = cmd.Start(); err != nil {
		return launchError(err)
	}
	requestR.Close()
	resultW.Close()
	if err = json.NewEncoder(requestW).Encode(o); err != nil {
		requestW.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return launchError(err)
	}
	requestW.Close()
	var result launchResult
	err = json.NewDecoder(resultR).Decode(&result)
	// The console owns the running session after the launch acknowledgement.
	// Releasing this process handle does not terminate the child.
	_ = cmd.Process.Release()
	if err != nil {
		return launchError(fmt.Errorf("dedicated console did not acknowledge launch: %w", err))
	}
	if result.Error != "" {
		return launchError(fmt.Errorf("%s", result.Error))
	}
	return 0
}
func launchError(err error) int { fmt.Fprintln(os.Stderr, "codex-at:", err); return 1 }

func consoleChild(args []string) int {
	if len(args) != 2 {
		return launchError(fmt.Errorf("invalid internal console handles"))
	}
	handles := make([]uintptr, 2)
	for i, a := range args {
		h, err := strconv.ParseUint(a, 10, 64)
		if err != nil || h == 0 {
			return launchError(fmt.Errorf("invalid internal console handle"))
		}
		handles[i] = uintptr(h)
	}
	input := os.NewFile(handles[0], "request")
	result := os.NewFile(handles[1], "result")
	defer input.Close()
	defer result.Close()
	// Do not leak protocol handles to Codex; EOF must also report child failures.
	for _, h := range handles {
		if err := syscall.SetHandleInformation(syscall.Handle(h), syscall.HANDLE_FLAG_INHERIT, 0); err != nil {
			return launchError(err)
		}
	}
	report := func(err error) {
		r := launchResult{}
		if err != nil {
			r.Error = err.Error()
		}
		_ = json.NewEncoder(result).Encode(r)
		result.Close()
	}
	var o options
	if err := json.NewDecoder(input).Decode(&o); err != nil {
		report(err)
		return 1
	}
	input.Close()
	in, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		report(err)
		return 1
	}
	defer in.Close()
	out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		report(err)
		return 1
	}
	defer out.Close()
	os.Stdin = in
	os.Stdout = out
	os.Stderr = out
	code := runCodex(o, report)
	if !o.Close {
		fmt.Fprintf(out, "\nCodex exited (code %d). Press any key to close this window.\n", code)
		if err := waitKey(in); err != nil {
			fmt.Fprintln(out, "codex-at: wait for key:", err)
		}
	}
	return code
}
func waitKey(in *os.File) error {
	kernel := syscall.NewLazyDLL("kernel32.dll")
	var mode uint32
	get := kernel.NewProc("GetConsoleMode")
	set := kernel.NewProc("SetConsoleMode")
	ok, _, err := get.Call(in.Fd(), uintptr(unsafe.Pointer(&mode)))
	if ok == 0 {
		return err
	}
	ok, _, err = set.Call(in.Fd(), uintptr(mode&^(1|2|4)))
	if ok == 0 {
		return err
	}
	defer set.Call(in.Fd(), uintptr(mode))
	// Discard keys left over from the just-ended Codex session.
	ok, _, err = kernel.NewProc("FlushConsoleInputBuffer").Call(in.Fd())
	if ok == 0 {
		return err
	}
	type inputRecord struct {
		EventType uint16
		Padding   uint16
		Data      [4]uint32
	}
	for {
		var event inputRecord
		var count uint32
		ok, _, err = kernel.NewProc("ReadConsoleInputW").Call(in.Fd(), uintptr(unsafe.Pointer(&event)), 1, uintptr(unsafe.Pointer(&count)))
		if ok == 0 {
			return err
		}
		if count == 1 && event.EventType == 1 && event.Data[0] != 0 {
			return nil
		}
	}
}
