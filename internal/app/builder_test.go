package app

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"gestalt/internal/logging"
)

func TestBuildWiresManager(t *testing.T) {
	root := t.TempDir()
	configRoot := filepath.Join(root, ".gestalt")
	agentsDir := filepath.Join(configRoot, "config", "agents")
	promptsDir := filepath.Join(configRoot, "config", "prompts")

	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "hello.tmpl"), []byte("echo hi\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	agentTOML := "name = \"Codex\"\nshell = \"/bin/bash\"\nprompt = \"hello\"\ncli_type = \"codex\"\nllm_model = \"default\"\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "codex.toml"), []byte(agentTOML), 0o644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(10), logging.LevelInfo, io.Discard)
	configFS := os.DirFS(configRoot)

	result, err := Build(BuildOptions{
		Logger:               logger,
		Shell:                "/bin/bash",
		ConfigFS:             configFS,
		ConfigOverlay:        configFS,
		ConfigRoot:           "config",
		AgentsDir:            agentsDir,
		SessionLogDir:        filepath.Join(root, "sessions"),
		InputHistoryDir:      filepath.Join(root, "input-history"),
		SessionRetentionDays: 7,
		BufferLines:          1000,
	})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	if result.Manager == nil {
		t.Fatalf("expected manager to be set")
	}
	if len(result.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(result.Agents))
	}
	if result.Agents["codex"].Name != "Codex" {
		t.Fatalf("expected Codex agent, got %q", result.Agents["codex"].Name)
	}
	if len(result.Skills) != 0 {
		t.Fatalf("expected no skills, got %d", len(result.Skills))
	}
	agents := result.Manager.ListAgents()
	if len(agents) != 1 {
		t.Fatalf("expected 1 manager agent, got %d", len(agents))
	}
	if agents[0].ID != "codex" {
		t.Fatalf("expected agent id codex, got %q", agents[0].ID)
	}
}
