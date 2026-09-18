# Unresolved defects

- `TestDarwinSubscriptionStatus/claude_subscription` fails on Windows hosts —
  `wake_auth_native_test.go:45`. The subtest expects `ok=true` but gets `cannot
  locate Windows Anthropic authentication profiles`, because the case does not
  provide an `APPDATA` profile root for the Windows branch of `checkWakeAuth`.
  Reproduced with `go test -run TestDarwinSubscriptionStatus .` on Go 1.27.1
  windows/amd64, both on this branch and on unmodified `main` at `ef9a36a`.
