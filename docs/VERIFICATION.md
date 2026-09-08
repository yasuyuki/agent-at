# Verification and remaining acceptance

Status as of 2026-09-08: **not accepted for Windows release**. Windows 10/11
full acceptance remains incomplete. Publication preparation is authorized;
external publication and push remain on hold.

The user reports that all five basic Windows steps were confirmed and recorded
in Windows commit `4c5c662`. Do not request those confirmations again. The commit
has not reached this checkout; its exact step descriptions must be read from
that record rather than inferred. Actual sleep/resume, Ctrl+C, automatic console
closing and other checks outside those five steps remain unverified. Integrate
the Windows-tested source and record before selecting publication artifacts.

## Contract and implementation

The standalone Windows x64 timer must validate and snapshot a one-time local
reservation before waiting, launch once after its deadline (including resume),
and preserve the Codex invocation, approval choice, working directory and
process/console lifetime. Linux/macOS use the caller's terminal and must not be
rejected solely because of their OS. Windows `.cmd` is supported without
embedding the request in shell code; long interactive requests use a temporary
UTF-8 file. No scheduler service, persistence, retries, private launcher or
external timer runtime is part of the product.

The deliverables are source, tests, the unsigned `dist/codex-at.exe`, checksum,
English CLI, English/Japanese README and MIT license. The initial branch is
`main`. On 2026-09-08 the user explicitly requested a commit for handoff to
Windows Codex before native acceptance. This snapshot is for that verification;
absence of a Windows runner is not treated as a passing test.

## Executed

Host: isolated Linux x86-64; source: this repository. Toolchain: Go 1.27.1,
returned as stable by <https://go.dev/dl/?mode=json> on 2026-09-08. The Linux
amd64 toolchain archive matched the official SHA-256:
`63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445`.

| Check | Result / scope |
| --- | --- |
| `go test -race ./...` | Passed on Linux; civil-time validation, next-day/year rollover, New York and Lord Howe DST folds/gaps, Apia skipped date, controllable clock resume/backwards jump/cancel/exactly-once |
| Process tests | Passed on Linux; fake Codex receives Japanese, newlines, quotes, shell metacharacters, long prompts, working directory, approval/model flags and stdin unchanged; stderr and exit 23 preserved; temporary prompt cleanup on exit/start failure |
| Input tests | Passed on Linux; UTF-8 BOM, invalid/empty text, unreadable/missing file, mutually exclusive prompt inputs, directory errors |
| `go vet ./...` | Passed for Linux and Windows targets |
| Windows x64 test build | Passed (`go test -c`); **not executed** |
| Windows x64 application build | Passed with `CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`; PE32+ x86-64 console executable targeting Windows 10.00 |
| macOS arm64 application build | Passed with `CGO_ENABLED=0`; **not executed** |
| Linux CLI subprocess checks | Two independent timers each launched one fake Codex at the same deadline, preserving BOM-file content/stdin/cwd/default approval flag; real SIGINT while waiting returned 130 with no Codex launch |
| Real Codex headless on Linux | Passed: timer invoked Codex 0.153.4, inherited model `gpt-6-astra`, returned exact `CODEX_AT_SMOKE_OK` and exit 0 |
| Independent source review | Windows cmd quoting, IPC and cleanup reviewed; non-character key handling fixed to use `ReadConsoleInputW`; Windows runtime behavior remains unverified |

The real-Codex smoke requested only the fixed response and prohibited tools and
file changes. `--approve-for-me` was supplied and accepted; stderr reported
`approval: on-request` and `sandbox: workspace-write`. No approval request was
triggered. This proves initial prompt/stdin and option acceptance for this Linux
invocation only. It does **not** prove automatic review of shell, filesystem,
network, MCP or other tool requests, or any Windows behavior. Do not infer that
`on-request` means automatic review was exercised or that the flag was ignored.

## Windows acceptance matrix (Windows record pending integration)

Use both a Windows 10 x64 and Windows 11 x64 desktop session with a usable
console and installed/authenticated Codex. Open PowerShell in this repository's
Windows checkout. Follow the build/test commands in the README. `go test ./...`
executes actual `cmd.exe` and opens a short-lived dedicated console; no AI
service is used by those tests. The Windows tests cover `.exe`/`.cmd`, special
characters and trailing directory separators, model/approval arguments,
headless stdin/stderr/exit code, child launch acknowledgement and temporary file
cleanup. Run this on both OSes; a cross-build or Wine run is not a replacement.

For live Codex, use the native executable path with `--codex`, then repeat with
the installed `.cmd` shim. Get paths from that installation; do not copy Linux
paths or invent a wrapper. The working directory must meet Codex's existing
requirements (for example a Git repository for `exec`). Keep settings and
approval policies unchanged except for the product's documented switches.
Record OS build, resolved Codex version/path type, exact timer inputs, result,
exit code and any limitation without recording credentials or private prompts.

A short interactive smoke, in PowerShell at the repository root:

```powershell
$at = (Get-Date).AddSeconds(10).ToString('yyyy-MM-ddTHH:mm:ss')
$timerArgs = @(
  '--at', $at
  '--cd', (Get-Location).Path
  '--', 'Reply with exactly CODEX_AT_SMOKE_OK. Do not use tools.'
)
.\dist\codex-at.exe @timerArgs
```

After integrating the Windows record, retain its confirmed results and use this
matrix only for remaining checks, with harmless synthetic requests:

| Operation | Required observation |
| --- | --- |
| Default interactive request | Parent exits after process-start acknowledgement; child displays the exact requested response |
| Continue with a second message | Conversation continues in the same Codex session |
| End Codex normally | Prior output remains; the window stays open |
| Press an arrow/F-key in the held window | Window closes, not just for character keys |
| Repeat with `--close-on-exit` | Window closes only after Codex exits |
| Ctrl+C during a running interactive task | Codex handles the event; supervisor survives to clean up and hold the window after Codex exits |
| `--headless` with and without `--close-on-exit` | No dedicated window; original prompt on stdin, stdout/stderr and exit status reach caller |
| Native executable and `.cmd` with a long UTF-8 request file | File contents are intact; Codex actually reads and executes the file request; no prompt metacharacters become shell code |
| Codex normal exit/start failure | Its temporary `codex-at-*` prompt directory is deleted; failed launch is visible |
| Wait, cancel with Ctrl+C before deadline | Exit 130; no Codex process |
| Suspend until after the deadline and resume | Exactly one launch, without waiting the original remaining duration again |
| Two timers at the same time | Each runs independently once |
| Omit model / specify a supported model | Existing model inherited / explicit model applied |
| Default / `--no-approve-for-me` | Default flag accepted / flag absent; no fallback to weaker policies |
| Real approval-requiring operation in a disposable workspace | Record whether automatic review is requested, allowed/denied, and whether interaction is required, separately for interactive and headless |

The last row requires an operation that actually reaches Codex's approval
boundary for that environment. A tool-free answer or an operation already
allowed by the sandbox is insufficient. Use harmless disposable inputs and
record only the observed scope; do not generalize one approval to all tools.

The current development environment exposes no Windows execution tool or
Windows mount. Its documented isolation blocks host/LAN/Tailscale/VSOCK access.
No boundary was changed to obtain a runner. These remaining tests must be
performed from a Windows-capable session before acceptance. The initial commit
is explicitly authorized as a handoff snapshot while those checks remain open.
After testing and any fixes, update this record and commit only this project's
changes; run the workspace's common push preflight with explicit **hold**. Do
not publish or push.
