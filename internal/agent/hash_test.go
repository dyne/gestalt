package agent

import "testing"

func TestComputeConfigHashStable(t *testing.T) {
	agentA := &Agent{
		Name:      "Coder",
		Shell:     "/bin/bash",
		CLIType:   "codex",
		Interface: AgentInterfaceCLI,
		CLIConfig: map[string]interface{}{
			"model":           "o3",
			"approval_policy": "never",
		},
	}
	agentB := &Agent{
		Name:      "Coder",
		Shell:     "/bin/bash",
		CLIType:   "codex",
		Interface: AgentInterfaceCLI,
		CLIConfig: map[string]interface{}{
			"approval_policy": "never",
			"model":           "o3",
		},
	}

	hashA := ComputeConfigHash(agentA)
	hashB := ComputeConfigHash(agentB)
	if hashA == "" || hashB == "" {
		t.Fatalf("expected non-empty hashes")
	}
	if hashA != hashB {
		t.Fatalf("expected hashes to match: %q vs %q", hashA, hashB)
	}

	agentB.CLIConfig["model"] = "o4"
	hashC := ComputeConfigHash(agentB)
	if hashC == hashA {
		t.Fatalf("expected hash to change when config changes")
	}
}

func TestComputeConfigHashInterfaceChanges(t *testing.T) {
	agent := &Agent{
		Name:      "Coder",
		Shell:     "/bin/bash",
		CLIType:   "codex",
		Interface: AgentInterfaceCLI,
	}
	hashCLI := ComputeConfigHash(agent)
	if hashCLI == "" {
		t.Fatalf("expected non-empty hash")
	}
	agent.Interface = "invalid"
	hashMCP := ComputeConfigHash(agent)
	if hashMCP == hashCLI {
		t.Fatalf("expected hash to change when interface changes")
	}
}
