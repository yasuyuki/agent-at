package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

type stringsFlag []string

func (s *stringsFlag) String() string     { return strings.Join(*s, ", ") }
func (s *stringsFlag) Set(v string) error { *s = append(*s, v); return nil }

type options struct {
	At        time.Time
	Prompt    string
	CD        string
	AddDirs   stringsFlag
	Model     string
	Codex     string
	NoApprove bool
	Headless  bool
	Close     bool
}

func parseOptions(args []string, now time.Time, out io.Writer) (options, error) {
	var o options
	var at, promptFile string
	f := flag.NewFlagSet("codex-at", flag.ContinueOnError)
	f.SetOutput(out)
	f.StringVar(&at, "at", "", "Required local HH:mm[:ss] or YYYY-MM-DDTHH:mm[:ss]")
	f.StringVar(&promptFile, "prompt-file", "", "Read the prompt from a UTF-8 file (optional BOM)")
	f.StringVar(&o.CD, "cd", "", "Working directory (default: current directory)")
	f.Var(&o.AddDirs, "add-dir", "Additional directory (repeatable)")
	f.StringVar(&o.Model, "model", "", "Codex model (default: inherit Codex configuration)")
	f.StringVar(&o.Codex, "codex", "", "Codex executable (.exe or .cmd on Windows; default: PATH lookup)")
	f.BoolVar(&o.NoApprove, "no-approve-for-me", false, "Omit the default --approve-for-me option")
	f.BoolVar(&o.Headless, "headless", false, "Run codex exec with the prompt on stdin")
	f.BoolVar(&o.Close, "close-on-exit", false, "Close the dedicated console after Codex exits")
	f.Usage = func() {
		fmt.Fprintln(out, "Usage: codex-at --at TIME [options] -- \"prompt\"\n       codex-at --at TIME [options] --prompt-file FILE\n\nPlace options before the single prompt argument. Ctrl+C cancels while waiting.")
		f.PrintDefaults()
	}
	if err := f.Parse(args); err != nil {
		return o, err
	}
	if at == "" {
		return o, errors.New("--at is required")
	}
	var err error
	o.At, err = parseAt(at, now)
	if err != nil {
		return o, err
	}
	if (promptFile != "" && f.NArg() != 0) || (promptFile == "" && f.NArg() != 1) {
		return o, errors.New("provide exactly one prompt argument or --prompt-file")
	}
	if promptFile != "" {
		p, err := filepath.Abs(promptFile)
		if err != nil {
			return o, err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return o, fmt.Errorf("read prompt: %w", err)
		}
		o.Prompt = strings.TrimPrefix(string(b), "\ufeff")
	} else {
		o.Prompt = f.Arg(0)
	}
	if !utf8.ValidString(o.Prompt) || strings.ContainsRune(o.Prompt, 0) {
		return o, errors.New("prompt must be valid UTF-8 without NUL")
	}
	if strings.TrimSpace(o.Prompt) == "" {
		return o, errors.New("prompt must not be empty")
	}
	if o.CD == "" {
		o.CD = "."
	}
	o.CD, err = absoluteDir(o.CD)
	if err != nil {
		return o, err
	}
	for i, d := range o.AddDirs {
		o.AddDirs[i], err = absoluteDir(d)
		if err != nil {
			return o, err
		}
	}
	if o.Codex == "" {
		o.Codex = "codex"
	}
	o.Codex, err = exec.LookPath(o.Codex)
	if err != nil {
		return o, fmt.Errorf("find Codex: %w", err)
	}
	o.Codex, err = filepath.Abs(o.Codex)
	if err != nil {
		return o, err
	}
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(o.Codex))
		if ext != ".exe" && ext != ".cmd" {
			return o, errors.New("--codex must resolve to an .exe or .cmd file")
		}
	}
	for _, s := range append([]string{o.Codex, o.CD, o.Model}, o.AddDirs...) {
		if !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
			return o, errors.New("paths and model must be valid UTF-8 without NUL")
		}
	}
	return o, nil
}
func absoluteDir(p string) (string, error) {
	a, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	s, err := os.Stat(a)
	if err != nil {
		return "", err
	}
	if !s.IsDir() {
		return "", fmt.Errorf("not a directory: %s", a)
	}
	return a, nil
}
func main() { os.Exit(run(os.Args[1:])) }
func run(args []string) int {
	if len(args) > 0 && args[0] == "--internal-console" {
		return consoleChild(args[1:])
	}
	o, err := parseOptions(args, time.Now(), os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "codex-at:", err)
		return 2
	}
	// Validate platform command limits and temporary storage before waiting.
	_, cleanup, err := prepare(o)
	cleanup()
	if err != nil {
		fmt.Fprintln(os.Stderr, "codex-at:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Fprintf(os.Stderr, "Scheduled for %s (%s). Keep this timer open; Ctrl+C cancels.\n", o.At.Format(time.RFC3339), o.At.Location())
	code, err := schedule(ctx, wallClock{}, o.At, func() int {
		stop()
		if o.Headless {
			return runCodex(o, nil)
		}
		return launchConsole(o)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "codex-at: cancelled")
	}
	return code
}
