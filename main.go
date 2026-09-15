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
	At          time.Time
	Prompt      string
	CD          string
	AddDirs     stringsFlag
	Model       string
	Executable  string
	Agent       string
	NoApprove   bool
	Headless    bool
	Close       bool
	NewConsole  bool
	Resume      string
	Wake        bool
	WakeText    string
	WakeTimeout time.Duration
	Persist     bool
	List        bool
	Remove      string
}

func parseOptions(args []string, now time.Time, out io.Writer) (options, error) {
	var o options
	var at, promptFile string
	f := flag.NewFlagSet("agent-at", flag.ContinueOnError)
	f.SetOutput(out)
	f.BoolVar(&o.Persist, "persist", false, "Windows/Linux/macOS: save a one-time headless OS task, then exit (Linux requires systemd user manager)")
	f.BoolVar(&o.List, "list", false, "Windows/Linux/macOS: list this user's persistent jobs and saved results")
	f.StringVar(&o.Remove, "remove", "", "Windows/Linux/macOS: cancel an unstarted job or delete its saved results; refuses running jobs")
	f.StringVar(&at, "at", "", "Local HH:mm[:ss] or YYYY-MM-DDTHH:mm[:ss]")
	f.BoolVar(&o.Wake, "wake", false, "Send one minimal headless request; skips user settings (model: clean CLI default)")
	f.StringVar(&o.WakeText, "wake-text", "ok", "Literal one-line reply requested by --wake")
	f.DurationVar(&o.WakeTimeout, "wake-timeout", 2*time.Minute, "Positive execution timeout for --wake; excludes waiting")
	f.StringVar(&promptFile, "prompt-file", "", "Read the prompt from a UTF-8 file (optional BOM)")
	f.StringVar(&o.CD, "cd", "", "Working directory (default: current directory)")
	f.Var(&o.AddDirs, "add-dir", "Additional directory (repeatable)")
	f.StringVar(&o.Model, "model", "", "Agent model (normal: inherit configuration; wake: clean CLI default)")
	f.StringVar(&o.Agent, "agent", "codex", "Agent to run: codex or claude")
	f.StringVar(&o.Executable, "agent-path", "", "Agent executable (.exe or .cmd on Windows; default: selected agent on PATH)")
	f.BoolVar(&o.NoApprove, "no-auto-approve", false, "Inherit agent approval configuration instead of requesting automatic review")
	f.BoolVar(&o.Headless, "headless", false, "Run codex exec or claude --print with the prompt on stdin")
	f.StringVar(&o.Resume, "resume", "", "Resume a selected-agent session ID and send exactly resume (default: start now)")
	f.BoolVar(&o.NewConsole, "new-console", false, "Open a dedicated Windows console instead of inheriting the current terminal")
	f.BoolVar(&o.Close, "close-on-exit", false, "Close the dedicated console after the agent exits")
	f.Usage = func() {
		fmt.Fprintln(out, "Usage: agent-at --at TIME [options] -- \"prompt\"\n       agent-at --at TIME [options] --prompt-file FILE\n       agent-at --resume SESSION_ID [--at TIME] [options]\n       agent-at --wake --at TIME [--agent codex|claude] [--model MODEL]\n\nPlace options before the single prompt argument. Without --persist, Ctrl+C cancels while waiting.\nWake skips user settings; omitted model uses clean CLI default. Even a minimal\nrequest consumes usage; quota timer start/reset is not guaranteed. Without --persist keep the timer\nopen; it does not wake a sleeping PC. Wake policy: Codex 0.154.0 / Claude 2.1.268.\nExisting subscription file authentication is required; keychain-only/unknown\nproviders fail. Mandatory auth/policy and internal discovery may remain.\nUnsupported CLI options fail without retry.")
		f.PrintDefaults()
		fmt.Fprintln(out, "\nWith --persist, --at is required even for resume. Registration is not execution\nsuccess. After registration the terminal may close; the PC must be awake and the\nsame user signed in (screen lock is OK). No catch-up after sleep, power-off or\nsign-out. Always headless; no console restoration. --list / --remove JOB_ID\nmanage private payload/results under LOCALAPPDATA/agent-at/jobs. Keep agent-at,\nthe selected CLI and working paths in place. Terminal-only secrets are not saved.")
	}
	if err := f.Parse(args); err != nil {
		return o, err
	}
	set := map[string]bool{}
	f.Visit(func(v *flag.Flag) { set[v.Name] = true })
	management, err := validatePersist(o, set, f.NArg(), runtime.GOOS)
	if err != nil {
		return o, err
	}
	if management {
		return o, nil
	}
	if err := validateWake(o, set, f.NArg()); err != nil {
		return o, err
	}
	if o.Wake {
		o.Headless = true
		o.Prompt = wakePrompt(o.WakeText)
	}
	if o.Persist {
		o.Headless = true
	}
	if o.Agent != "codex" && o.Agent != "claude" {
		return o, errors.New("--agent must be codex or claude")
	}
	resumeSet := false
	f.Visit(func(v *flag.Flag) {
		if v.Name == "resume" {
			resumeSet = true
		}
	})
	if resumeSet && (strings.TrimSpace(o.Resume) == "" || o.Resume == "-" || !utf8.ValidString(o.Resume) || strings.ContainsAny(o.Resume, "\x00\r\n")) {
		return o, errors.New("--resume requires a nonempty agent session ID")
	}
	if at == "" {
		if o.Persist {
			return o, errors.New("--persist requires a nonempty --at")
		}
		if !resumeSet {
			return o, errors.New("--at is required for a new task")
		}
		o.At = now
	} else {
		o.At, err = parseAt(at, now)
		if err != nil {
			return o, err
		}
	}
	if resumeSet {
		if promptFile != "" || f.NArg() != 0 {
			return o, errors.New("--resume cannot be combined with a prompt or --prompt-file")
		}
		o.Prompt = "resume"
	} else if !o.Wake {
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
	if o.Executable == "" {
		o.Executable = o.Agent
	}
	o.Executable, err = exec.LookPath(o.Executable)
	if err != nil {
		return o, fmt.Errorf("find %s: %w", o.Agent, err)
	}
	o.Executable, err = filepath.Abs(o.Executable)
	if err != nil {
		return o, err
	}
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(o.Executable))
		if ext != ".exe" && ext != ".cmd" {
			return o, errors.New("--agent-path must resolve to an .exe or .cmd file")
		}
	}
	for _, s := range append([]string{o.Executable, o.CD, o.Model}, o.AddDirs...) {
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
	if len(args) > 0 && args[0] == "--internal-persist" {
		return persistentChild(args[1:])
	}
	if len(args) > 0 && args[0] == "--internal-console" {
		return consoleChild(args[1:])
	}
	o, err := parseOptions(args, time.Now(), os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent-at:", err)
		return 2
	}
	if o.Persist || o.List || o.Remove != "" {
		return runPersistent(o)
	}
	if o.Wake {
		return scheduleWake(o)
	}
	// Validate platform command limits and temporary storage before waiting.
	_, cleanup, err := prepare(o)
	cleanup()
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent-at:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Fprintf(os.Stderr, "Scheduled for %s (%s). Keep this timer open; Ctrl+C cancels.\n", o.At.Format(time.RFC3339), o.At.Location())
	code, err := schedule(ctx, wallClock{}, o.At, func() int {
		stop()
		if o.Headless || !o.NewConsole {
			return runAgent(o, nil)
		}
		return launchConsole(o)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent-at: cancelled")
	}
	return code
}
