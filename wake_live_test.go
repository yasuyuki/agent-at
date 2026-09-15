package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Opt-in verification only: one billable request to an existing authenticated
// vendor CLI. Normal tests never call an AI service. Evidence stays outside Git.
func TestWakeLive(t *testing.T) {
	exe := os.Getenv("AGENT_AT_LIVE_PATH")
	if exe == "" {
		t.Skip("set AGENT_AT_LIVE_PATH explicitly for one real request")
	}
	agent, evidence := os.Getenv("AGENT_AT_LIVE_AGENT"), os.Getenv("AGENT_AT_LIVE_EVIDENCE")
	if !filepath.IsAbs(evidence) {
		t.Fatal("AGENT_AT_LIVE_EVIDENCE must be an existing absolute directory outside the repository")
	}
	now := time.Now()
	o, err := parseOptions([]string{"--wake", "--agent", agent, "--agent-path", exe, "--at", now.Add(2 * time.Second).Format("2006-01-02T15:04:05")}, now, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	cmd, cleanup, err := prepareWake(o)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if err := checkWakeAuth(agent, cmd.Environ()); err != nil {
		t.Fatal(err)
	}
	// CLI-owned diagnostics added only for this explicitly requested smoke.
	if agent == "codex" {
		cmd.Args = append(cmd.Args[:len(cmd.Args)-2], "--json", "--", "-")
		cmd.Env = append(cmd.Env, "RUST_LOG=codex_core=debug,codex_app_server=debug")
	} else {
		for i := range cmd.Args {
			if cmd.Args[i] == "--output-format" {
				cmd.Args[i+1] = "json"
			}
		}
		cmd.Args = append(cmd.Args[:len(cmd.Args)-1], "--debug-file", filepath.Join(evidence, agent+"-debug.log"), "--")
	}
	out, err := os.Create(filepath.Join(evidence, agent+"-stdout.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	diagnostic, err := os.Create(filepath.Join(evidence, agent+"-stderr.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer diagnostic.Close()
	cmd.Stdout, cmd.Stderr = out, diagnostic
	code, err := schedule(context.Background(), wallClock{}, o.At, func() int { return runWake(context.Background(), cmd, o.WakeTimeout, diagnostic) })
	if err != nil || code != 0 {
		t.Fatalf("wake exit=%d error=%v; inspect private diagnostics", code, err)
	}
	cleanup()
	if _, err := os.Stat(cmd.Dir); !os.IsNotExist(err) {
		t.Fatal("wake cwd survived", err)
	}
}
