# agent-at v0.3.0

Keep one-time Codex or Claude Code reservations after the terminal closes, or
send a minimal wake request at a chosen time.

## Changes

- `--persist` saves a headless reservation and exits. Supports normal requests,
  `--resume`, and `--wake` through Windows Task Scheduler, Linux systemd user
  timers, or macOS LaunchAgents.
- `--list` shows reservations and saved results. `--remove JOB_ID` cancels an
  unstarted reservation or removes completed logs/results, refusing active work.
- `--wake` sends one minimal request with optional `--wake-text`, `--model`, and
  `--wake-timeout`. It suppresses development context/tools and uses existing
  subscription authentication; it does not bypass usage limits or prove a quota reset.
- macOS wake checks Claude.ai subscription metadata or Codex ChatGPT file/native
  keyring authentication through the installed CLI. No Keychain secrets are exported.
- Fixed requests, private stdout/stderr/results, duplicate-start protection and
  wake timeout/descendant cleanup are shared across platforms.
- Existing foreground timers, interactive requests and session resume remain available.

## Downloads

| Asset | Contents |
| --- | --- |
| `agent-at-windows-x64.zip` | Windows x64, `dist/agent-at.exe` |
| `agent-at-linux-x64.tar.gz` | Linux x64, `dist/agent-at` |
| `agent-at-macos-x64.tar.gz` | macOS Intel, `dist/agent-at` |
| `agent-at-macos-arm64.tar.gz` | macOS Apple Silicon, `dist/agent-at` |
| `agent-at-source.zip` | Source at the tagged release commit |
| `SHA256SUMS` | SHA-256 checksums for archives and source-commit record |
| `SOURCE-COMMIT.txt` | Exact source commit |

Extract the archive before running. Keep the executable, installed agent CLI,
work directories and authentication paths stable while reservations exist.
The agent CLIs require their own installation and login. Windows binaries are
unsigned; macOS binaries are not Developer ID signed or notarized.

## Scheduling conditions and verification limits

Windows requires the same signed-in user and an awake PC at the scheduled time;
there is no catch-up after power-off, sleep or sign-out. Linux requires a running
systemd user manager; it does not enable linger and may deliver calendar timers
late after suspend. macOS requires a GUI login domain; its minute-based calendar
trigger checks the saved year/seconds, may catch up at login/resume within that
year, and requires an unchanged system timezone. No custom daemon or automatic
retry is added. Registration success is distinct from model-request success.

Linux tests, race and vet, Windows/Darwin cross-checks and all four builds pass.
Windows native registration/execution/cleanup and a minimal Claude wake passed
on the `f9d96de` ancestor. **Native Linux persistent-timer execution and native
macOS scheduling/Keychain behavior remain unverified.** Whole-terminal closure,
reboot/login/lock/suspend and other listed cases remain open. Cross-compilation
is not native execution; this release preserves those limitations explicitly.

See the [English README](https://github.com/yasuyuki/agent-at/blob/v0.3.0/README.md),
[Japanese README](https://github.com/yasuyuki/agent-at/blob/v0.3.0/README.ja.md), and
[verification record](https://github.com/yasuyuki/agent-at/blob/v0.3.0/docs/VERIFICATION.md).
