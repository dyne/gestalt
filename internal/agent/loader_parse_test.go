package agent

import (
	"strings"
	"testing"
)

func TestParseErrorIncludesPosition(t *testing.T) {
	data := []byte("name = \"Test\"\ncli_type =\n")
	_, err := loadAgentFromBytes("bad.toml", data)
	if err == nil {
		t.Fatalf("expected parse error")
	}
	message := err.Error()
	if !strings.Contains(message, "parse agent file bad.toml") {
		t.Fatalf("expected parse error prefix, got %q", message)
	}
	if !strings.Contains(message, "line 2") {
		t.Fatalf("expected line info in error, got %q", message)
	}
}

func TestCodexModeNotInCLIConfig(t *testing.T) {
	data := []byte(`
name = "Codex"
shell = "/bin/bash"
cli_type = "codex"
codex_mode = "tui"
model = "o3"
`)
	agent, err := loadAgentFromBytes("codex.toml", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := agent.CLIConfig["codex_mode"]; ok {
		t.Fatalf("expected codex_mode to be excluded from cli_config")
	}
	if agent.CLIConfig["model"] != "o3" {
		t.Fatalf("expected model in cli_config, got %#v", agent.CLIConfig)
	}
}
