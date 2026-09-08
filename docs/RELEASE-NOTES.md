# codex-at — Initial preview

Draft release notes. No tag, release or public repository has been created.
The Windows-tested source and executable from `4c5c662` are integrated.
The version and publication destination remain to be selected.

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

Windows 11 native tests and five user-observed interactive steps passed: initial
response, continued conversation, normal exit with retained history, and arrow-key
closure. Headless automatic review allowed one harmless shell escalation without
human interaction. This does not establish other review boundaries or denial
handling. Temporary request directories use a protected current-user Windows DACL.

Windows 10, actual sleep/resume, Ctrl+C, automatic console closing, long interactive
file access and integration with an installed `.cmd` shim remain unverified.
The native automated `.cmd` tests use a synthetic forwarding shim. See the
verification record for the complete scope; this is a preview, not full acceptance.

Reservations exist only while the timer is running. There is no persistence,
wake-up service, retry or recurring schedule. Forced termination can leave
request text in temporary files. The Windows executable is unsigned.
