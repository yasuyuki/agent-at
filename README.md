# codex-at

Run an installed, authenticated Codex once at a local time. The timer needs no
external runtime. Windows 10/11 x64 is the primary target; Linux and macOS are
also supported by the source. [日本語](README.ja.md)

**Status: preparing an initial preview; no release has been published.**
The user has confirmed five basic Windows verification steps. Their detailed
record is in Windows commit `4c5c662`, pending integration into this checkout.
Actual sleep/resume, Ctrl+C, automatic console closing and other checks outside
those five steps remain unverified. See [verification](docs/VERIFICATION.md);
full Windows 10/11 acceptance and auto-review coverage are not established.

## Use

Run `dist\codex-at.exe` in PowerShell from the extracted project directory:

```powershell
.\dist\codex-at.exe --at 23:30 -- "Review the current changes"
```

For a multiline or long request, save a UTF-8 file and use:

```powershell
$timerArgs = @(
  '--at', '23:30:15'
  '--cd', 'C:\Projects\my project'
  '--prompt-file', '.\request.txt'
)
.\dist\codex-at.exe @timerArgs
```

For a particular date, use `--at 2027-01-15T23:30`. For headless execution, add
`--headless` before the prompt. On Linux/macOS use `./codex-at` instead of the
Windows executable. Interactive Codex uses the current terminal on those OSes.

| Option | Meaning |
| --- | --- |
| `--at TIME` | Required `HH:mm[:ss]` or `YYYY-MM-DDTHH:mm[:ss]`, local time |
| `"prompt"` / `--prompt-file FILE` | Exactly one; put the single prompt argument after all options |
| `--cd DIR` | Working directory; default is the directory where the timer was started |
| `--add-dir DIR` | Additional directory; repeatable |
| `--model MODEL` | Only supplied to Codex when specified; otherwise Codex selects the model |
| `--codex PATH` | Executable to run; default resolves `codex` from PATH at reservation time |
| `--no-approve-for-me` | Omit the default `--approve-for-me` option |
| `--headless` | Run `codex exec`; send the prompt on stdin |
| `--close-on-exit` | Close the Windows dedicated console after Codex exits |
| `--help` | Show English help |

Time-only input selects the next occurrence (an equal/past clock time means
tomorrow). A full date must be in the future. Invalid dates and local times
that do not exist or are ambiguous at clock transitions are rejected.
The selected instant, paths, executable and prompt contents are fixed before
waiting. Files accept UTF-8 with or without a BOM; whitespace-only prompts,
invalid UTF-8, NUL and missing files/directories are rejected.
For Windows `.cmd` shims, paths and model names cannot contain double quotes or line breaks.

Leave the timer running. Ctrl+C cancels while waiting (exit 130). Nothing is
saved as a scheduled task. The timer neither wakes the computer nor survives
termination/restart. After resume it checks the wall clock and launches once if
overdue, normally within one second. System clock changes affect when the fixed
instant is reached; later time-zone changes do not reinterpret the reservation.
Each timer is independent. There are no retries or recurring schedules.

## Codex prerequisites and process lifetime

Install and authenticate Codex for the account that runs the timer. Its runtime
and network requirements still apply: a npm `.cmd` shim may need Node.js even
though this timer does not. Check the [official Codex CLI reference](https://developers.openai.com/codex/cli/reference).
The locally inspected Codex 0.153.4 help accepts `--approve-for-me` in both
interactive and `exec` modes. Actual auto-review behavior must be verified for
your Codex version/account/model; CLI acceptance alone does not prove it.
Unsupported options fail visibly. The timer never switches to another approval
or sandbox policy to recover. Configured model/authentication/settings are
inherited; the default approval flag requests Codex's automatic review behavior.
Use `--no-approve-for-me` to leave that option unset.

On Windows, interactive mode starts this same executable in a new console, then
runs Codex there with `--no-alt-screen` to preserve scrollback. You can continue
the conversation after the initial task. After Codex itself exits, the window
waits for any key unless `--close-on-exit` was specified. The timer exits after
receiving the child's process-start result, not after the task finishes. A timer
exit code of zero means that process creation succeeded, not that Codex accepted
the flags, authenticated, or completed the task. Later errors remain in the
Codex console. For `.cmd`, process creation refers to the command interpreter.

Headless mode inherits stdout/stderr and returns Codex's exit code (also through
`.cmd`). `--close-on-exit` is accepted and has no effect in headless mode.
On Linux/macOS interactive mode runs in the caller's terminal and returns
Codex's exit code; it does not open a window or wait for a key. `--close-on-exit`
has no effect there either. Input errors use exit 2; launch errors use exit 1.

Native Windows `.exe` files run directly. Trusted `.cmd` shims run through the
Windows system `cmd.exe`, with delayed expansion disabled and arguments passed
as quoted environment values. A shim must forward arguments without `CALL` or
re-evaluating them as shell code (the standard npm `%*` forwarding pattern).
Other Windows script types are not accepted. No generated launch script or
private environment manager is needed.

Interactive `.cmd` prompts and interactive prompts too large for the Windows
command-line budget use a private, unique system-temporary directory containing
`prompt.txt`. The original UTF-8 contents are preserved. Codex receives an
instruction to read that file, and the directory is added using `--add-dir`.
This is a file-reading instruction, not a native prompt-file attachment; actual
model compliance is a separate acceptance check. That directory is writable
under Codex's additional-directory semantics. It is deleted when Codex exits,
before the console waits for a key. Headless prompts always use stdin.
Normal startup failures clean up too. Forced termination, shutdown, or a file
held open by another process can leave `codex-at-*` directories in the system
temporary location; they contain the request text. Do not remove one while its
Codex session is still running.

## Build and test

Use Go from the [official distribution](https://go.dev/dl/).
The official stable download API returned **Go 1.27.1 on 2026-09-08**;
the build toolchain archive was SHA-256 checked against that API.
No C compiler or third-party Go dependency is needed. Time-zone data is embedded.
The unsigned Windows x64 executable is at `dist/codex-at.exe`, with
`dist/SHA256SUMS` for integrity checking.

From the repository root in PowerShell on Windows:

```powershell
go test ./...
go vet ./...
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go build -trimpath -buildvcs=false -o dist/codex-at.exe .
```

From the repository root in a POSIX shell:

```sh
go test -race ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o codex-at .
```

Cross-build Windows from a POSIX shell:

```sh
export CGO_ENABLED=0 GOOS=windows GOARCH=amd64
go vet ./...
go test -c -o codex-at-windows.test.exe .
go build -trimpath -buildvcs=false -o dist/codex-at.exe .
```

Compiling the Windows tests is not running them. Follow the Windows matrix in
[verification](docs/VERIFICATION.md) before claiming acceptance. The tests use a
fake Codex process; they do not authenticate or call an AI service.

MIT licensed. This project is not an official OpenAI product.
