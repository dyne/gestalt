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
	agent.Interface = AgentInterfaceMCP
	hashMCP := ComputeConfigHash(agent)
	if hashMCP == hashCLI {
		t.Fatalf("expected hash to change when interface changes")
	}
}

func TestComputeConfigHashLegacyAlias(t *testing.T) {
	agentLegacy := &Agent{
		Name:      "Coder",
		Shell:     "/bin/bash",
		CLIType:   "codex",
		CodexMode: CodexModeMCPServer,
	}
	agentInterface := &Agent{
		Name:      "Coder",
		Shell:     "/bin/bash",
		CLIType:   "codex",
		Interface: AgentInterfaceMCP,
	}
	hashLegacy := ComputeConfigHash(agentLegacy)
	hashInterface := ComputeConfigHash(agentInterface)
	if hashLegacy == "" || hashInterface == "" {
		t.Fatalf("expected non-empty hashes")
	}
	if hashLegacy != hashInterface {
		t.Fatalf("expected legacy alias hash to match interface hash")
	}
}
