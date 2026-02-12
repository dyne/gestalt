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
output_filter = "ansi-strip"
output_filters = ["utf8-guard", "scrollback-vt"]
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
	if _, ok := agent.CLIConfig["output_filter"]; ok {
		t.Fatalf("did not expect output_filter in CLI config")
	}
	if _, ok := agent.CLIConfig["output_filters"]; ok {
		t.Fatalf("did not expect output_filters in CLI config")
	}
	if value, ok := agent.CLIConfig["model"]; !ok || value != "o3" {
		t.Fatalf("expected model in cli_config, got %#v", agent.CLIConfig)
	}
	if agent.OutputFilter != "ansi-strip" {
		t.Fatalf("expected output_filter, got %q", agent.OutputFilter)
	}
	if len(agent.OutputFilters) != 2 || agent.OutputFilters[0] != "utf8-guard" || agent.OutputFilters[1] != "scrollback-vt" {
		t.Fatalf("expected output_filters, got %#v", agent.OutputFilters)
	}
}
