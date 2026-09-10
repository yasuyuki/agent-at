# agent-at v0.2.0

Schedule Codex or Claude Code to run once at a local time, or resume a saved
conversation immediately or at a scheduled time. This release renames the
command and repository from `codex-at` to `agent-at`.

## Changes

- Select Codex (default) or Claude Code with `--agent codex|claude`.
- Resume a saved conversation with `--resume ID` and send exactly `resume`.
  Omit `--at` to resume immediately, or supply it to wait for the chosen time.
- Inherit the caller's terminal by default. On Windows, `--new-console` selects
  the previous dedicated-window behavior.
- Select UTF-8 console input/output on Windows while the agent runs, then
  restore the original code pages to address Japanese mojibake.
- Use `--headless` for `codex exec` or `claude --print`, with the request on
  stdin. UTF-8 prompt files, additional directories and model selection remain
  supported.

## Migration

The Windows executable is now `dist/agent-at.exe`. Replace the timer's
`--codex` option with `--agent-path`, and `--no-approve-for-me` with
`--no-auto-approve`. The old option names are not aliases.

Codex requests `--approve-for-me` by default; Claude requests
`--permission-mode auto`. Opting out leaves the selected agent's approval
configuration unchanged. No permission-bypass fallback is added. Both agents
still require their own installation and authentication.

## Downloads

- `agent-at-windows-x64.zip`: standalone Windows x64 timer and documentation.
  Extract it before running `dist/agent-at.exe`.
- `agent-at-source.zip`: source at the release commit, including the tracked
  Windows executable.
- `SHA256SUMS`: checksums for both ZIPs and `SOURCE-COMMIT.txt`.
- `SOURCE-COMMIT.txt`: the exact source commit used for this release.

See the [English README](https://github.com/yasuyuki/agent-at/blob/v0.2.0/README.md)
or [Japanese README](https://github.com/yasuyuki/agent-at/blob/v0.2.0/README.ja.md)
for usage and build instructions.

## Verification and limitations

Linux race tests and vet, Windows vet/test cross-compilation, and Windows/macOS
cross-builds pass. Real Codex session resume and Claude scheduled execution and
same-session resume passed on Linux. Earlier Windows 11 native tests and basic
Codex interactions passed; the user also confirmed earlier Windows 10 job
execution and saved-file recovery.

The new UTF-8 display fix and Claude behavior have not been verified on a
Windows desktop. Claude interactive behavior and actual auto-mode tool approvals,
continuation after quota recovery, and other outstanding scenarios retain the
limits in the [verification record](https://github.com/yasuyuki/agent-at/blob/v0.2.0/docs/VERIFICATION.md).
Cross-compilation is not native execution. The Windows executable is unsigned.

Reservations exist only while the timer runs. There is no persistence, wake-up
service, retry or recurring schedule. This release does not bypass usage limits.
