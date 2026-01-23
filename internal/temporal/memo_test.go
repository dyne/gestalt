package temporal

import (
	"strings"
	"testing"

	"gestalt/internal/agent"
)

func TestSerializeAgentConfigRoundTrip(t *testing.T) {
	profile := &agent.Agent{
		Name:    "Codex",
		CLIType: "codex",
		CLIConfig: map[string]interface{}{
			"model": "o3",
		},
		Skills: []string{"mcp-builder"},
	}
	payload, err := SerializeAgentConfig(profile)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if strings.TrimSpace(payload) == "" {
		t.Fatalf("expected payload")
	}
	restored, err := DeserializeAgentConfig(payload)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if restored.Name != "Codex" {
		t.Fatalf("expected name Codex, got %q", restored.Name)
	}
	if restored.CLIType != "codex" {
		t.Fatalf("expected cli_type codex, got %q", restored.CLIType)
	}
	if restored.CLIConfig["model"] != "o3" {
		t.Fatalf("expected model o3, got %v", restored.CLIConfig["model"])
	}
}

func TestSerializeAgentConfigTruncates(t *testing.T) {
	profile := &agent.Agent{
		Name:    "Large",
		CLIType: "codex",
		CLIConfig: map[string]interface{}{
			"user_instructions": strings.Repeat("a", memoLimitBytes+100),
		},
	}
	payload, err := SerializeAgentConfig(profile)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if len(payload) > memoLimitBytes {
		t.Fatalf("expected payload length <= %d, got %d", memoLimitBytes, len(payload))
	}
}

func TestDeserializeAgentConfigRejectsJSON(t *testing.T) {
	payload := `{"name":"JSON","cli_type":"codex"}`
	_, err := DeserializeAgentConfig(payload)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "legacy JSON memo detected") {
		t.Fatalf("expected legacy JSON memo error, got %v", err)
	}
}
