# Verification and remaining acceptance

Status as of 2026-09-10: the **v0.2.0** command/build is **agent-at**, with Codex
and Claude Code selected by `--agent`. The repository is now [yasuyuki/agent-at](https://github.com/yasuyuki/agent-at).
The **codex-at v0.1.0-preview.1** ZIP is retained as a separate historical release. Historical Windows results below apply to that earlier build,
not to native acceptance of the new Claude adapter or the UTF-8 display fix.

## Agent selection and Claude Code (2026-09-10)

Contract: rename the command/executable to `agent-at` / `dist/agent-at.exe`;
select `codex` (default) or `claude` with `--agent`. Both must support scheduled
new tasks, exact `resume` messages to a selected saved session, stdin in
headless mode, UTF-8 prompt files, model/additional directories, process exit
codes and existing terminal/console behavior. `--agent-path` and
`--no-auto-approve` replace the Codex-specific timer option names. Release
history is retained. The repository was subsequently renamed from `codex-at`
to `agent-at`; the initial release artifact names are unchanged.

Implementation: Codex uses its existing CLI arguments; Claude uses `--print`
for headless stdin, `--resume=ID` for the saved conversation, process cwd for
`--cd`, and `--permission-mode auto` by default. Opt-out leaves the selected
agent's approval configuration unchanged. No permission-bypass flag is added.
Claude's own interactive renderer is retained. CLI semantics were checked
against installed Claude Code 2.1.267 and its
[official reference](https://code.claude.com/docs/en/cli-reference).

Executed on Linux using the existing Go 1.27.1 toolchain:

- `go test -race ./...` and `go vet ./...`: passed. Both agents' fake processes
  receive Japanese, multiline/long prompts, cwd, model/additional-directory
  arguments, approval choice and resume input. Agent selection resolves the
  corresponding executable; unsupported agents are rejected before waiting.
- Windows amd64 vet, test cross-compilation and `dist/agent-at.exe` cross-build:
  passed. `.cmd` round-trip and resume tests cover both agents but have **not
  executed on native Windows** for this update.
- macOS arm64 cross-build: passed; not a native execution test.
- Real Claude Code 2.1.267: a disposable saved conversation received a marker
  without tools; `agent-at --agent claude --headless --resume ID` sent only
  `resume` and returned that remembered marker, exit 0. A separate scheduled
  new request through the timer also returned its exact marker, exit 0.
  Both selected the default automatic-review option; no tool approval was
  exercised, so this does not prove an actual auto-mode approval decision.

Remaining: Claude interactive rendering, actual Claude tool approvals and
Windows native/desktop execution with `.exe` and installed `.cmd`. The prior
Windows 10 job success is user-confirmed for Codex, with Japanese mojibake
reported. The new console fix and continuation after real quota recovery
remain unverified on that desktop. Existing checks below retain their stated
historical scope. Current binaries, temporary directories and build commands
use `agent-at`; historical commands naming `codex-at` refer to their recorded
older build. Use the current README for executable names and options.

## Resume and terminal update (2026-09-10)

Contract: `--resume ID` must reopen that saved Codex conversation and send only
`resume`; `--at` optionally delays it. New requests retain their required time.
Default interactive execution must keep the caller's console and environment;
`--new-console` explicitly requests the old Windows dedicated window. Configure
UTF-8 console input/output during Codex execution and restore prior code pages
on return. Codex's configured tool shell is unchanged.

Verified on Linux with Go 1.27.1 (official archive checksum rechecked):

- `go test -race ./...` and `go vet ./...`: pass. Immediate/scheduled resume,
  prompt conflicts, exact interactive argument/headless stdin payload, session
  ID forwarding and default attached-process exit status are covered.
- Windows amd64 `go vet ./...`, `go test -c` and application cross-build: pass.
  The new native tests check console UTF-8/restoration, dedicated-child code
  pages and exact `.cmd` resume arguments; they have **not run on Windows**.
- Real Codex 0.154.0 headless resume: a disposable conversation was given a
  marker to remember without tools. The rebuilt timer sent only `resume` and
  received the remembered marker; the resumed session ID matched the original,
  exit 0. This proves conversation continuity, not usage-limit reset handling.

Remaining acceptance: execute the native tests and observe Japanese input/output
on Windows 10/11 in the inherited terminal and `--new-console`, using both native
Codex and its installed `.cmd` shim. Confirm a previously limit-stopped session
continues after quota recovers. Font rendering, visible interactive resume and
actual limit recovery are not established by cross-compilation or the Linux test.
The new executable/checksum are in `dist`; the initial GitHub Release ZIP has
not been replaced by this update.

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
English CLI, English/Japanese README and MIT license. The integration branch is
`main`. A preview does not assert full acceptance of every target OS or scenario.

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

## Windows native results (2026-09-08)

Restored the complete `codex-at-handoff.bundle` into a new independent checkout,
on `main` at `e6d9707c2f9ac22b2c030d54059126ec810d80a4`, with a clean initial
working tree. No existing directory was overwritten. Origin is the local bundle,
not a published remote.

Host: native Windows x64, build **26200.9168**, display version **25H2**
(Windows 11; the legacy registry ProductName still says Windows 10 Pro).
Go **1.27.1 windows/amd64**, official Windows ZIP SHA-256
`a3911b5e0e1b1053f25ed0675f4c1c6aad1e2bfcf253df2b9be4caabd2edd95d`,
verified against <https://go.dev/dl/?mode=json> before extraction.
The installed native Codex executable reports **0.153.4**, matching the latest
CLI entry in the [official changelog](https://learn.chatgpt.com/docs/changelog)
checked on this date. Existing authentication/configuration were used unchanged.
No installed `.cmd` shim was found; the actual cmd.exe parser was exercised with
the existing synthetic `%*` forwarding test shim.

| Check | Result / scope |
| --- | --- |
| Native `go test ./...`, `go vet ./...`, Windows x64 build | Passed before the fix; original native build hash matched the bundled executable |
| Native process tests | `.exe` and real `cmd.exe` with synthetic `.cmd`: Japanese, long/multiline requests, shell metacharacters, trailing slash, model/approval flags, stdin/stderr, exit 23, temporary-file read/cleanup and missing-executable cleanup passed |
| Native dedicated console test | Production IPC launched a fake Codex in a new console, read the long prompt, acknowledged launch and removed its directory; visual output, key handling and actual window closure were not observed |
| Native scheduler tests | Simulated clock resume/backwards change, cancellation and exactly-once passed; physical suspend and console Ctrl+C remain untested |
| Real headless initial response | `--at` set to local now + 3 seconds, `--headless --codex <installed native exe>`; request `Reply with exactly CODEX_AT_WINDOWS_OK. Do not use tools or change files.`; inherited `gpt-6-astra`, exact response and timer exit 0 |
| Real explicit model / close flag | `--headless --close-on-exit --model gpt-6-astra --cd <disposable Git repository>`; explicit model applied, output and exit 0 returned |
| Real automatic review | In the preceding invocation, explicitly requested one `exec_command` with `sandbox_permissions=require_escalated`, command `Write-Output CODEX_AT_APPROVAL_OK`; a separate `guardian_review` record returned `risk_level=low`, `user_authorization=high`, `outcome=allow`; the command then output the marker and exited 0, with no human approval interaction |
| Real long headless / opt-out | Rebuilt exe, `--headless --no-approve-for-me --prompt-file <UTF-8 BOM file>`; 2,000 repetitions of Japanese and shell metacharacters plus a tool-free marker request; `CODEX_AT_LONG_OK`, exit 0, inherited model; Codex reported `approval: never`, existing workspace-write sandbox |

The automatic-review row proves headless automatic review for this explicit, harmless shell
escalation on this Windows account/version/model. It does not establish denial
handling, interactive automatic review, network, MCP, sensitive file operations,
or every possible approval boundary. Unlike the earlier Linux smoke, an actual
review request and decision were observed, not merely option acceptance.

Independent review found that Unix mode bits did not protect temporary request
files on Windows. The actual TEMP ACL allowed additional principals to modify
children. The fix creates the directory atomically with a protected current-user
DACL and an inheritable file/directory ACE, before writing any request bytes.
The regression inspects both directory and file ACLs, checks inheritance is
blocked on the directory, and requires exactly the current-user FullControl ACE.
It does not impersonate another account. Normal Codex sandbox access to this
directory must still be verified in the interactive long-request check below.

After the fix, native `go test ./...` (including the ACL regression) and
`go vet ./...` passed. Windows x64 was rebuilt with `CGO_ENABLED=0`, `-trimpath`
and `-buildvcs=false`; `dist/SHA256SUMS` was regenerated from that executable.
Linux amd64 and macOS arm64 cross-builds also passed; they are not native tests.

### User-observed interactive smoke (2026-09-08)

The user reported that all five requested handoff steps worked with the rebuilt
executable: launch from PowerShell at local now + 10 seconds with
`Reply exactly CODEX_AT_SMOKE_OK.`, observe that response in the dedicated
console, send `Reply exactly SECOND_OK.` and continue the same conversation,
enter `/quit` and observe retained history and the key-wait message, then press
an arrow key and observe the window close. These are user-observed results,
not desktop observations by the agent. No repeat of these steps is required.
The report covers those five steps; it does not establish physical resume,
Ctrl+C, `--close-on-exit`, F-keys, long interactive file reading, interactive
automatic review, installed `.cmd` integration or another OS.

## Remaining Windows acceptance

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

Use this matrix for the remaining checks with harmless synthetic requests.
The ordinary native interactive smoke, second message, normal exit/history and
arrow-key close are already user-confirmed above; the launch-acknowledgement
timing and F-key variants retain their narrower automated/unverified scope.

| Operation | Required observation |
| --- | --- |
| Default interactive request | Caller's terminal is retained; timer waits for Codex and returns its exit code |
| `--new-console` interactive request | Parent exits after process-start acknowledgement; dedicated child displays the response |
| Continue with a second message | Conversation continues in the same Codex session |
| End Codex normally with `--new-console` | Prior output remains; the window stays open |
| Press an arrow/F-key in the held window | Window closes, not just for character keys |
| Repeat `--new-console` with `--close-on-exit` | Window closes only after Codex exits |
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

The Windows verification session could run native Windows processes but had no
native desktop control surface. Visible history, continued interaction and arrow-key exit are confirmed
by the user above. Physical sleep/resume, Ctrl+C console delivery and real
interactive long-file reading remain unverified. Windows 10 task execution is
user-confirmed above; native automated tests on that OS and macOS execution remain unverified. The matrix above
remains the manual acceptance procedure; automated rows have the narrower scopes
recorded in Windows native results. Real installed `.cmd` integration remains
unverified. Opt-out was exercised with a tool-free request, not an approval test.
After future testing or fixes, update this record with the exact observed scope.
Do not treat checks outside the confirmed scenarios as completed.

## Initial preview artifact verification

The publication-preparation checkout integrates the Windows-tested source and
executable without changing application code. Linux race tests and vet,
Windows vet/test cross-compilation, and macOS arm64 cross-build pass after
integration. A Go 1.27.1 Windows x64 cross-build using `CGO_ENABLED=0`,
`-trimpath` and `-buildvcs=false` is compared byte-for-byte to the native Windows
executable; the published checksum must refer to that same binary.

The Windows ZIP retains the README directory layout: `dist/codex-at.exe`,
`dist/SHA256SUMS`, both READMEs, `LICENSE`, and the verification, release
notes and Go license notice under `docs/`. It includes no Git bundle, private workspace records or
credentials. Extract it before running the README commands. Checksums establish
integrity, not an Authenticode signature. The executable remains unsigned.


## v0.2.0 release artifacts

The v0.2.0 source keeps the application code from the agent-selection update.
Release preparation runs Linux `go test -race ./...` and `go vet ./...`, Windows
amd64 vet/test cross-compilation, and Windows/macOS cross-builds using Go 1.27.1.
The Windows rebuild must match the tracked `dist/agent-at.exe` byte for byte;
`dist/SHA256SUMS` identifies that executable. This reproducibility check does
not establish native Windows execution of the new features.

`agent-at-windows-x64.zip` includes `dist/agent-at.exe`, `dist/SHA256SUMS`, both
READMEs, `LICENSE`, and the verification, release notes and Go license notice.
`agent-at-source.zip` is a Git archive of the release commit under `agent-at/`.
`SOURCE-COMMIT.txt` identifies the exact commit shared by the release tag and
both ZIPs. Top-level `SHA256SUMS` covers both ZIPs and `SOURCE-COMMIT.txt`.
The ZIP manifests and extracted executable hash are checked before publication;
public downloads are compared with the prepared artifacts after publication.
The executable is unsigned. All platform and approval limits above still apply
when publishing this version as a regular release.
