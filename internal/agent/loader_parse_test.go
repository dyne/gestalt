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

func TestInterfaceAndCodexModeNotCapturedInCLIConfig(t *testing.T) {
	data := []byte(`
name = "Codex"
shell = "/bin/bash"
cli_type = "codex"
interface = "mcp"
codex_mode = "mcp-server"
model = "o3"
gui_modules = ["plan-progress"]
`)
	agent, err := loadAgentFromBytes("agent.toml", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.Interface != AgentInterfaceMCP {
		t.Fatalf("expected interface %q, got %q", AgentInterfaceMCP, agent.Interface)
	}
	if agent.CLIConfig == nil {
		t.Fatalf("expected CLI config")
	}
	if _, ok := agent.CLIConfig["interface"]; ok {
		t.Fatalf("did not expect interface in CLI config")
	}
	if _, ok := agent.CLIConfig["codex_mode"]; ok {
		t.Fatalf("did not expect codex_mode in CLI config")
	}
	if _, ok := agent.CLIConfig["gui_modules"]; ok {
		t.Fatalf("did not expect gui_modules in CLI config")
	}
	if value, ok := agent.CLIConfig["model"]; !ok || value != "o3" {
		t.Fatalf("expected model in cli_config, got %#v", agent.CLIConfig)
	}
}

func TestLLMModelAliasSetsModel(t *testing.T) {
	data := []byte(`
name = "Codex"
shell = "/bin/bash"
llm_model = "default"
`)
	agent, err := loadAgentFromBytes("agent.toml", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.Model != "default" {
		t.Fatalf("expected model to be set from llm_model, got %q", agent.Model)
	}
}

func TestModelOverridesLLMModel(t *testing.T) {
	data := []byte(`
name = "Codex"
shell = "/bin/bash"
model = "primary"
llm_model = "legacy"
`)
	agent, err := loadAgentFromBytes("agent.toml", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.Model != "primary" {
		t.Fatalf("expected model to override llm_model, got %q", agent.Model)
	}
}
