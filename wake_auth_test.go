package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWakeAuthFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		agent, data string
		ok          bool
	}{
		{"codex", `{"auth_mode":"chatgpt","tokens":{"access_token":"fixture"}}`, true},
		{"codex", `{"tokens":{"access_token":"fixture"}}`, true},
		{"codex", `{"auth_mode":"apikey","OPENAI_API_KEY":"fixture"}`, false},
		{"codex", `{"tokens":{"access_token":"fixture"},"personal_access_token":"fixture"}`, false},
		{"codex", `{"tokens":{"access_token":"fixture"},"bedrock_api_key":"fixture"}`, false},
		{"codex", `{"tokens":{"access_token":"fixture"},"bedrock_access_keys":{}}`, false},
		{"codex", `{"auth_mode":"unknown","tokens":{"access_token":"fixture"}}`, false},
		{"codex", `{}`, false},
		{"codex", `{invalid`, false},
		{"claude", `{"claudeAiOauth":{"accessToken":"fixture"}}`, runtime.GOOS != "darwin"},
		{"claude", `{"claudeAiOauth":{"accessToken":"fixture"},"enterpriseGateway":{}}`, false},
		{"claude", `{}`, false},
	} {
		t.Run(tc.agent+tc.data, func(t *testing.T) {
			home := t.TempDir()
			root, name := ".codex", "auth.json"
			if tc.agent == "claude" {
				root, name = ".claude", ".credentials.json"
			}
			if err := os.Mkdir(filepath.Join(home, root), 0700); err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(home, root, name)
			if err := os.WriteFile(p, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			err := checkWakeAuth(tc.agent, []string{"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + home})
			if (err == nil) != tc.ok {
				t.Fatalf("want success=%v: %v", tc.ok, err)
			}
			b, readErr := os.ReadFile(p)
			if readErr != nil || string(b) != tc.data {
				t.Fatal("credential store changed")
			}
		})
	}
}

func TestWakeRefusesFederation(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".claude")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"fixture"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	profiles := filepath.Join(home, ".config", "anthropic", "configs")
	if runtime.GOOS == "windows" {
		profiles = filepath.Join(home, "Anthropic", "configs")
	}
	if err := os.MkdirAll(profiles, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profiles, "default.json"), []byte(`{"authentication":{"type":"oidc_federation"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkWakeAuth("claude", []string{"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + home}); err == nil {
		t.Fatal("accepted federation")
	}
}

func TestWakeRejectsRelativeAuthenticationRoots(t *testing.T) {
	home := t.TempDir()
	for _, key := range []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR", "ANTHROPIC_CONFIG_DIR", "XDG_CONFIG_HOME", "APPDATA"} {
		if err := checkWakeAuth("claude", []string{"HOME=" + home, "USERPROFILE=" + home, key + "=../elsewhere"}); err == nil {
			t.Fatalf("accepted relative %s", key)
		}
	}
}
