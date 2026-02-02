package main

import (
	"testing"

	"gestalt/internal/agent"
)

func TestSelectAgentMissing(t *testing.T) {
	_, err := selectAgent(map[string]agent.Agent{}, "missing")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestSelectAgentRequiresCodex(t *testing.T) {
	agents := map[string]agent.Agent{
		"test": {CLIType: "copilot"},
	}
	_, err := selectAgent(agents, "test")
	if err == nil {
		t.Fatalf("expected error")
	}
}
