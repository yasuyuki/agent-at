package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// This is an allowlist for existing subscription file credentials, not a login
// implementation. Never print, copy, rewrite or relocate credential bytes.
// Unknown/keychain-only/provider-managed modes fail before starting the CLI.
func checkWakeAuth(agent string, env []string) error {
	values := map[string]string{}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		values[strings.ToUpper(key)] = value
	}
	// The child changes cwd. Do not validate one store and let a relative
	// environment override select a different store from the temporary cwd.
	for _, key := range []string{"HOME", "USERPROFILE", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "ANTHROPIC_CONFIG_DIR", "XDG_CONFIG_HOME", "APPDATA"} {
		if value := values[key]; value != "" && !filepath.IsAbs(value) {
			return errors.New("wake requires absolute authentication/configuration home paths")
		}
	}
	home := values["HOME"]
	if runtime.GOOS == "windows" {
		home = values["USERPROFILE"]
	}
	if home == "" {
		return errors.New("authentication home is unavailable")
	}
	if agent == "codex" {
		root := values["CODEX_HOME"]
		if root == "" {
			root = filepath.Join(home, ".codex")
		}
		var auth struct {
			Mode    string          `json:"auth_mode"`
			API     json.RawMessage `json:"OPENAI_API_KEY"`
			PAT     json.RawMessage `json:"personal_access_token"`
			Bedrock json.RawMessage `json:"bedrock_api_key"`
			AWS     json.RawMessage `json:"bedrock_access_keys"`
			Tokens  *struct {
				Access string `json:"access_token"`
			} `json:"tokens"`
		}
		if err := readWakeAuthJSON(filepath.Join(root, "auth.json"), &auth); err != nil {
			return err
		}
		for _, field := range []json.RawMessage{auth.API, auth.PAT, auth.Bedrock, auth.AWS} {
			if nonnull(field) {
				return errors.New("Codex API/provider credentials are not supported by wake")
			}
		}
		if (auth.Mode != "" && auth.Mode != "chatgpt" && auth.Mode != "chatgptAuthTokens") || auth.Tokens == nil || auth.Tokens.Access == "" {
			return errors.New("wake requires existing Codex ChatGPT file authentication; keychain-only or other modes are unsupported")
		}
		return nil
	}
	if values["CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST"] != "" || values["ANTHROPIC_UNIX_SOCKET"] != "" {
		return errors.New("host-managed Claude authentication cannot be verified as subscription authentication")
	}
	if runtime.GOOS == "darwin" {
		return errors.New("Claude wake cannot verify macOS keychain authentication; normal scheduling remains available")
	}
	root := values["CLAUDE_CONFIG_DIR"]
	if root == "" {
		root = filepath.Join(home, ".claude")
	}
	var auth struct {
		Gateway json.RawMessage `json:"enterpriseGateway"`
		OAuth   *struct {
			Access string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if err := readWakeAuthJSON(filepath.Join(root, ".credentials.json"), &auth); err != nil {
		return err
	}
	if nonnull(auth.Gateway) || auth.OAuth == nil || auth.OAuth.Access == "" {
		return errors.New("wake requires existing Claude subscription OAuth file authentication without a saved gateway")
	}
	// The Anthropic federation profile resolver is independent of setting-sources
	// and can take precedence over subscription OAuth even in safe mode.
	profileRoot := values["ANTHROPIC_CONFIG_DIR"]
	if profileRoot == "" {
		if runtime.GOOS == "windows" {
			if values["APPDATA"] == "" {
				return errors.New("cannot locate Windows Anthropic authentication profiles")
			}
			profileRoot = filepath.Join(values["APPDATA"], "Anthropic")
		} else {
			configHome := values["XDG_CONFIG_HOME"]
			if configHome == "" {
				configHome = filepath.Join(home, ".config")
			}
			profileRoot = filepath.Join(configHome, "anthropic")
		}
	}
	name := "default"
	b, err := os.ReadFile(filepath.Join(profileRoot, "active_config"))
	if err == nil {
		name = strings.TrimSpace(string(b))
	} else if !os.IsNotExist(err) {
		return errors.New("cannot verify active Anthropic authentication profile")
	}
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return errors.New("unsupported Anthropic authentication profile name")
	}
	b, err = os.ReadFile(filepath.Join(profileRoot, "configs", name+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.New("cannot verify Anthropic authentication profile")
	}
	var profile struct {
		Authentication struct {
			Type string `json:"type"`
		} `json:"authentication"`
	}
	if json.Unmarshal(b, &profile) != nil || profile.Authentication.Type != "" {
		return errors.New("Anthropic provider profiles are not supported by wake")
	}
	return nil
}

func nonnull(raw json.RawMessage) bool {
	return len(raw) != 0 && strings.TrimSpace(string(raw)) != "null"
}

func readWakeAuthJSON(path string, target any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return errors.New("subscription file credentials are unavailable (keychain-only authentication is unsupported)")
	}
	if json.Unmarshal(b, target) != nil {
		return errors.New("subscription credential format is unsupported")
	}
	return nil
}
