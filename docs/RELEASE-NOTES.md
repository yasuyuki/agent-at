# codex-at — Initial preview

Draft release notes. No tag, release or public repository has been created.
Integrate Windows commit `4c5c662` and check the matching executable before
selecting a release version and publishing these notes.

Schedule an installed, authenticated Codex to run once at a local time.
The Windows x64 timer is a standalone executable and needs no external runtime.
Codex itself retains its normal installation and authentication requirements.

- Interactive Windows sessions open in a dedicated console with scrollback.
- Headless sessions use `codex exec`, with the request on stdin and the exit
  status returned to the caller.
- Requests can come from an argument or a UTF-8 file. Working directory,
  additional directories and model selection are configurable.
- `--approve-for-me` is passed by default and can be omitted explicitly.
- Linux and macOS can build from source and use the current terminal.
- English and Japanese documentation and an MIT license are included.

## Verification limits

Five basic Windows steps have been confirmed by the user. Actual sleep/resume,
Ctrl+C and automatic console closing remain unverified on Windows, along with
other checks outside those five steps. Full Windows 10/11 acceptance and the
scope of real Codex automatic review are not established.

Reservations exist only while the timer is running. There is no persistence,
wake-up service, retry or recurring schedule. Forced termination can leave
request text in temporary files. The Windows executable is unsigned.
