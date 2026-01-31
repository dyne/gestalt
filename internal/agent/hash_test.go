package agent

import "testing"

func TestComputeConfigHashStable(t *testing.T) {
	agentA := &Agent{
		Name:      "Coder",
		Shell:     "/bin/bash",
		CLIType:   "codex",
		CodexMode: CodexModeMCPServer,
		CLIConfig: map[string]interface{}{
			"model":           "o3",
			"approval_policy": "never",
		},
	}
	agentB := &Agent{
		Name:      "Coder",
		Shell:     "/bin/bash",
		CLIType:   "codex",
		CodexMode: CodexModeMCPServer,
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

	agentB.CodexMode = CodexModeTUI
	hashD := ComputeConfigHash(agentB)
	if hashD == hashC {
		t.Fatalf("expected hash to change when codex_mode changes")
	}
}
