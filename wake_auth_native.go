package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Native status commands return authentication metadata, never a model request.
// Keep their output private: even failure diagnostics may identify an account.
func authorizeWakeCommand(ctx context.Context, o options, cmd *exec.Cmd) error {
	if runtime.GOOS != "darwin" {
		return checkWakeAuth(o.Agent, cmd.Environ())
	}
	return authorizeDarwinWake(o, cmd, func(args []string) ([]byte, error) {
		probe := exec.Command(cmd.Path, args...)
		probe.Dir, probe.Env = cmd.Dir, cmd.Environ()
		var out, diagnostic authStatusBuffer
		probe.Stdout, probe.Stderr = &out, &diagnostic
		code := runWake(ctx, probe, o.WakeTimeout, io.Discard)
		if code != 0 || out.overflow || diagnostic.overflow {
			return nil, errors.New("native authentication status failed; check existing CLI login and Keychain access")
		}
		// Codex versions emit login status to stderr. Do not expose either stream.
		if o.Agent == "codex" {
			primary, secondary := bytes.TrimSpace(out.Bytes()), bytes.TrimSpace(diagnostic.Bytes())
			if len(primary) == 0 {
				primary, secondary = secondary, primary
			}
			if len(secondary) != 0 {
				return nil, errors.New("ambiguous native authentication status")
			}
			return primary, nil
		}
		return out.Bytes(), nil
	})
}

func authorizeDarwinWake(o options, cmd *exec.Cmd, status func([]string) ([]byte, error)) error {
	values := map[string]string{}
	for _, entry := range cmd.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		values[key] = value
	}
	home := values["HOME"]
	if home == "" {
		return errors.New("authentication home is unavailable")
	}
	for _, key := range []string{"HOME", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "ANTHROPIC_CONFIG_DIR", "XDG_CONFIG_HOME"} {
		if value := values[key]; value != "" && !filepath.IsAbs(value) {
			return errors.New("wake requires absolute authentication/configuration home paths")
		}
	}
	if o.Agent == "codex" {
		root := values["CODEX_HOME"]
		if root == "" {
			root = filepath.Join(home, ".codex")
		}
		if _, err := os.Lstat(filepath.Join(root, "auth.json")); err == nil {
			return checkWakeAuth(o.Agent, cmd.Environ())
		} else if !os.IsNotExist(err) {
			return errors.New("cannot verify existing Codex file authentication")
		}
		out, err := status([]string{"login", "status", "-c", "cli_auth_credentials_store='keyring'"})
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(out)) != "Logged in using ChatGPT" {
			return errors.New("wake requires existing Codex ChatGPT Keychain authentication")
		}
		for i, arg := range cmd.Args {
			if arg == "cli_auth_credentials_store='file'" {
				cmd.Args[i] = "cli_auth_credentials_store='keyring'"
				return nil
			}
		}
		return errors.New("wake credential store policy missing")
	}
	if values["CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST"] != "" || values["ANTHROPIC_UNIX_SOCKET"] != "" {
		return errors.New("host-managed Claude authentication cannot be verified as subscription authentication")
	}
	if err := checkClaudeProviderProfile(values, home); err != nil {
		return err
	}
	out, err := status([]string{"--safe-mode", "--setting-sources", "", "auth", "status", "--json"})
	if err != nil {
		return err
	}
	var state struct {
		LoggedIn     bool   `json:"loggedIn"`
		Method       string `json:"authMethod"`
		Provider     string `json:"apiProvider"`
		Subscription string `json:"subscriptionType"`
	}
	if json.Unmarshal(out, &state) != nil || !state.LoggedIn || state.Method != "claude.ai" || state.Provider != "firstParty" || strings.TrimSpace(state.Subscription) == "" {
		return errors.New("wake requires existing first-party Claude.ai subscription authentication")
	}
	return nil
}

// Status responses contain metadata only. A finite buffer prevents a broken CLI
// from exhausting the scheduler process while its wake timeout is still running.
// Overflow is an error, never a truncated response that can authenticate.
const authStatusMaxBytes = 64 * 1024

type authStatusBuffer struct {
	buffer   bytes.Buffer
	overflow bool
}

func (b *authStatusBuffer) Write(p []byte) (int, error) {
	if len(p) > authStatusMaxBytes-b.Len() {
		b.overflow = true
		return 0, errors.New("authentication status output exceeded metadata limit")
	}
	return b.buffer.Write(p)
}

func (b *authStatusBuffer) Len() int      { return b.buffer.Len() }
func (b *authStatusBuffer) Bytes() []byte { return b.buffer.Bytes() }
