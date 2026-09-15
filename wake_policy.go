package main

import "strings"

func wakeArgs(o options) []string {
	var args []string
	if o.Agent == "claude" {
		args = []string{"--print", "--safe-mode", "--setting-sources", "", "--tools", "", "--max-turns", "1", "--no-session-persistence", "--output-format", "text", "--system-prompt", wakeInstructions, "--effort", "low"}
	} else {
		args = []string{"exec", "--ignore-user-config", "--ephemeral", "--skip-git-repo-check", "--sandbox", "read-only"}
		// TOML literal strings also preserve the existing .cmd shim's strict
		// prohibition on double quotes in arguments. The instruction path is
		// relative to the isolated cwd, not to the user's configuration home.
		for _, setting := range []string{
			"approval_policy='never'",
			"cli_auth_credentials_store='file'",
			"project_doc_max_bytes=0", "project_root_markers=[]",
			"skills.include_instructions=false", "skills.bundled.enabled=false",
			"features.skill_mcp_dependency_install=false", "features.skill_search=false",
			"features.hooks=false", "features.memories=false",
			"features.shell_tool=false", "features.unified_exec=false", "features.shell_snapshot=false",
			"features.multi_agent=false", "features.goals=false",
			"features.plugins=false", "features.apps=false", "features.remote_plugin=false",
			"web_search='disabled'", "model_instructions_file='instructions.txt'",
			"model_reasoning_effort='low'", "history.persistence='none'",
		} {
			args = append(args, "-c", setting)
		}
	}
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	args = append(args, "--")
	if o.Agent == "codex" {
		args = append(args, "-")
	}
	return args
}

// Preserve authentication homes, OS credential stores, proxy/CA and managed
// policy. Suppress environment overrides that select paid API providers,
// ordinary models or nonessential integration/debug behaviour.
func wakeEnvironment(env []string) []string {
	remove := map[string]bool{
		"OPENAI_API_KEY": true, "CODEX_API_KEY": true, "CODEX_ACCESS_TOKEN": true, "OPENAI_BASE_URL": true,
		"ANTHROPIC_API_KEY": true, "ANTHROPIC_AUTH_TOKEN": true, "ANTHROPIC_BASE_URL": true,
		"ANTHROPIC_MODEL": true, "ANTHROPIC_SMALL_FAST_MODEL": true,
		"ANTHROPIC_DEFAULT_HAIKU_MODEL": true, "ANTHROPIC_DEFAULT_SONNET_MODEL": true, "ANTHROPIC_DEFAULT_OPUS_MODEL": true,
		"CLAUDE_CODE_USE_BEDROCK": true, "CLAUDE_CODE_USE_VERTEX": true, "CLAUDE_CODE_USE_FOUNDRY": true,
		"CLAUDE_CODE_USE_GATEWAY": true, "CLAUDE_CODE_USE_MANTLE": true,
		"CLAUDE_CODE_USE_ANTHROPIC_AWS": true, "CLAUDE_CODE_USE_ANTHROPIC_GOOGLE_CLOUD": true,
		"ANTHROPIC_PROFILE": true, "ANTHROPIC_FEDERATION_RULE_ID": true, "ANTHROPIC_ORGANIZATION_ID": true,
		"CLAUDE_CODE_SIMPLE": true, "CLAUDE_CODE_EFFORT_LEVEL": true,
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": true,
	}
	result := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if !remove[strings.ToUpper(name)] {
			result = append(result, entry)
		}
	}
	// Official essential-traffic mode also suppresses Claude's otherwise
	// automatic session-title inference in headless mode.
	return append(result, "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1")
}
