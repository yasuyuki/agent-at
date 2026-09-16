package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDarwinSubscriptionStatus(t *testing.T) {
	for _, tc := range []struct {
		name, agent, output string
		ok                  bool
	}{
		{"claude subscription", "claude", `{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty","subscriptionType":"max"}`, true},
		{"claude API", "claude", `{"loggedIn":true,"authMethod":"api_key","apiProvider":"firstParty","subscriptionType":"max"}`, false},
		{"claude provider", "claude", `{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"bedrock","subscriptionType":"max"}`, false},
		{"claude logged out", "claude", `{"loggedIn":false}`, false},
		{"claude malformed", "claude", `{} {}`, false},
		{"claude wrong type", "claude", `{"loggedIn":"true"}`, false},
		{"codex ChatGPT", "codex", "Logged in using ChatGPT\n", true},
		{"codex API", "codex", "Logged in using an API key", false},
		{"codex unknown", "codex", "Logged in", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			o := options{Agent: tc.agent}
			cmd := exec.Command("fake", wakeArgs(o)...)
			cmd.Env = []string{"HOME=" + home}
			err := authorizeDarwinWake(o, cmd, func(args []string) ([]byte, error) {
				want := []string{"--safe-mode", "--setting-sources", "", "auth", "status", "--json"}
				if tc.agent == "codex" {
					want = []string{"login", "status", "-c", "cli_auth_credentials_store='keyring'"}
				}
				if !reflect.DeepEqual(args, want) {
					t.Fatalf("status args %q", args)
				}
				return []byte(tc.output), nil
			})
			if (err == nil) != tc.ok {
				t.Fatalf("ok=%v err=%v", tc.ok, err)
			}
			if tc.agent == "codex" && tc.ok && !strings.Contains(strings.Join(cmd.Args, " "), "cli_auth_credentials_store='keyring'") {
				t.Fatal("request would not use validated Keychain")
			}
		})
	}
}

func TestDarwinAuthNeverFallsBackFromInvalidFile(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"OPENAI_API_KEY":"fixture"}`)
	path := filepath.Join(home, ".codex", "auth.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("fake")
	cmd.Env = []string{"HOME=" + home, "USERPROFILE=" + home}
	err := authorizeDarwinWake(options{Agent: "codex"}, cmd, func([]string) ([]byte, error) {
		t.Fatal("native fallback bypassed explicit file auth")
		return nil, nil
	})
	if err == nil {
		t.Fatal("accepted API auth")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(data) {
		t.Fatal("changed auth")
	}
}

func TestDarwinAuthStatusFailureAndManagedProvider(t *testing.T) {
	for _, agent := range []string{"codex", "claude"} {
		cmd := exec.Command("fake", wakeArgs(options{Agent: agent})...)
		cmd.Env = []string{"HOME=" + t.TempDir()}
		if err := authorizeDarwinWake(options{Agent: agent}, cmd, func([]string) ([]byte, error) { return nil, errors.New("status unavailable") }); err == nil {
			t.Fatal("accepted failed status")
		}
	}
	cmd := exec.Command("fake")
	cmd.Env = []string{"HOME=" + t.TempDir(), "CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST=1"}
	if err := authorizeDarwinWake(options{Agent: "claude"}, cmd, func([]string) ([]byte, error) { t.Fatal("queried managed provider"); return nil, nil }); err == nil {
		t.Fatal("accepted managed provider")
	}
}

func TestAuthStatusBufferRejectsOverflow(t *testing.T) {
	var b authStatusBuffer
	if _, err := b.Write([]byte(strings.Repeat("x", authStatusMaxBytes))); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte("x")); err == nil || !b.overflow {
		t.Fatal("accepted status output beyond bounded metadata buffer")
	}
	if b.Len() != authStatusMaxBytes {
		t.Fatal("buffer grew after overflow")
	}
}

func TestAuthStatusBufferCopyCannotBypassLimit(t *testing.T) {
	var b authStatusBuffer
	if _, err := io.Copy(&b, strings.NewReader(strings.Repeat("x", authStatusMaxBytes+1))); err == nil || !b.overflow || b.Len() > authStatusMaxBytes {
		t.Fatal("copy bypassed metadata bound")
	}
}
