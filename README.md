# codex-at

Run an installed, authenticated Codex once at a local time. The timer needs no
external runtime. Windows 10/11 x64 is the primary target; Linux and macOS are
also supported by the source. [日本語](README.ja.md)

**v0.1.0-preview.1 — initial preview.**
Download the [Windows x64 ZIP](https://github.com/yasuyuki/codex-at/releases/download/v0.1.0-preview.1/codex-at-windows-x64.zip)
and [checksums](https://github.com/yasuyuki/codex-at/releases/download/v0.1.0-preview.1/SHA256SUMS)
from the [release page](https://github.com/yasuyuki/codex-at/releases/tag/v0.1.0-preview.1).
Extract the ZIP before use. The changes below (`--resume`, inherited terminal,
`--new-console`, UTF-8 console setup) are in the current source/build and are
not included in that initial release ZIP.

Windows 11 native automated tests and five basic interactive steps have passed.
One harmless shell escalation was automatically reviewed and allowed in headless
mode. The user confirmed Windows 10 task execution and saved-file results on
2026-09-10, but reported unreadable Japanese console output. Actual sleep/resume,
Ctrl+C, automatic console closing and
other scenarios remain unverified. See [verification](docs/VERIFICATION.md) for
the exact tested scope and remaining checks.

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
| `--at TIME` | Required for new tasks; optional with `--resume`. `HH:mm[:ss]` or `YYYY-MM-DDTHH:mm[:ss]`, local time |
| `"prompt"` / `--prompt-file FILE` | Exactly one for new tasks; unavailable with `--resume`; put the single prompt argument after all options |
| `--cd DIR` | Working directory; default is the directory where the timer was started |
| `--add-dir DIR` | Additional directory; repeatable |
| `--model MODEL` | Only supplied to Codex when specified; otherwise Codex selects the model |
| `--codex PATH` | Executable to run; default resolves `codex` from PATH at reservation time |
| `--no-approve-for-me` | Omit the default `--approve-for-me` option |
| `--headless` | Run `codex exec`; send the prompt on stdin |
| `--resume ID` | Resume a saved Codex session and send exactly `resume`; immediately unless `--at` is specified |
| `--new-console` | Open a separate Windows console; default uses the caller's terminal |
| `--close-on-exit` | Close the Windows dedicated console after Codex exits |
| `--help` | Show English help |

To resume a task stopped by a usage limit, use its full Codex session ID (the
hash shown by Codex), under the same account and Codex home that saved it:

```powershell
$sessionId = Read-Host 'Codex session ID'
.\dist\codex-at.exe --resume $sessionId
```

Add `--at 23:30` to wait for a limit reset, or `--headless` for `codex exec
resume`. No new prompt is needed or accepted: the message is exactly `resume`,
including with Windows `.cmd` shims. The ID is passed to Codex directly; this
timer has no separate job database and does not recover missing session history
or bypass a usage limit. `--cd` retains its usual current-directory default;
run from the original project or supply that project's directory explicitly.

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

Interactive mode uses the caller's terminal on every OS, with
`--no-alt-screen` to retain scrollback. On Windows this preserves the console
or Windows Terminal tab opened from PowerShell/cmd; it does not start a new
shell or guess its executable from parent processes. Codex's own tool shell
continues to follow its Codex configuration. You can keep chatting after the
initial task. The timer waits for Codex and returns its exit code.

For an attached Windows console, the timer temporarily selects UTF-8 input and
output code pages before starting Codex and restores the previous values when
Codex exits. Redirected streams are left unchanged. This corrects the legacy
code-page mismatch; visible Japanese glyphs still require a suitable console font.

Use `--new-console` to select the previous dedicated-window behavior on Windows.
That window stays open after Codex exits until a key is pressed, unless
`--close-on-exit` is set. The parent timer exits after process creation is
acknowledged; zero then means only that launch succeeded, not authentication or
task completion. With a `.cmd` shim this acknowledges the command interpreter.
Later errors remain in the dedicated console.

Headless mode inherits stdout/stderr and returns Codex's exit code.
`--new-console` has no effect in headless mode or on Linux/macOS.
`--close-on-exit` only affects a dedicated Windows console; the caller's terminal
is never closed. Input errors use exit 2; launch errors use exit 1.

Native Windows `.exe` files run directly. Trusted `.cmd` shims run through the
Windows system `cmd.exe`, with delayed expansion disabled and arguments passed
as quoted environment values. A shim must forward arguments without `CALL` or
re-evaluating them as shell code (the standard npm `%*` forwarding pattern).
Other Windows script types are not accepted. No generated launch script or
private environment manager is needed.

New-task interactive `.cmd` prompts and interactive prompts too large for the Windows
command-line budget use a private, unique system-temporary directory containing
`prompt.txt`. Windows creates it with a protected current-user DACL and does not
inherit additional permissions from TEMP. The original UTF-8 contents are preserved. Codex receives an
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

MIT licensed. The executable includes Go runtime/standard-library code; its
[license notice](docs/GO-LICENSE.txt) is included in the distribution.
This project is not an official OpenAI product.
