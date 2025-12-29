package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"gestalt/internal/logging"
)

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("GESTALT_PORT", "9090")
	t.Setenv("GESTALT_SHELL", "/bin/zsh")
	t.Setenv("GESTALT_TOKEN", "secret")

	cfg := loadConfig()
	if cfg.Port != 9090 {
		t.Fatalf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.Shell != "/bin/zsh" {
		t.Fatalf("expected shell /bin/zsh, got %q", cfg.Shell)
	}
	if cfg.AuthToken != "secret" {
		t.Fatalf("expected token secret, got %q", cfg.AuthToken)
	}
}

func TestLoadConfigDefaultsOnInvalidPort(t *testing.T) {
	t.Setenv("GESTALT_PORT", "not-a-number")
	cfg := loadConfig()
	if cfg.Port != 8080 {
		t.Fatalf("expected default port 8080, got %d", cfg.Port)
	}
}

func TestFindStaticDir(t *testing.T) {
	root := t.TempDir()
	frontendDist := filepath.Join(root, "frontend", "dist")
	if err := os.MkdirAll(frontendDist, 0755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	if got := findStaticDir(); got != filepath.Join("frontend", "dist") {
		t.Fatalf("expected %q, got %q", filepath.Join("frontend", "dist"), got)
	}
}

func TestLoadAgentsIntegration(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "config", "agents")
	promptsDir := filepath.Join(root, "config", "prompts")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "hello.txt"), []byte("echo hi\n"), 0644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	agentJSON := `{"name":"Codex","shell":"/bin/bash","prompt":"hello","llm_type":"codex","llm_model":"default"}`
	if err := os.WriteFile(filepath.Join(agentsDir, "codex.json"), []byte(agentJSON), 0644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(10), logging.LevelInfo, io.Discard)
	agents, err := loadAgents(logger)
	if err != nil {
		t.Fatalf("load agents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents["codex"].Name != "Codex" {
		t.Fatalf("expected Codex, got %q", agents["codex"].Name)
	}
}

func TestLoadAgentsReportsInvalidJSON(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "config", "agents")
	promptsDir := filepath.Join(root, "config", "prompts")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "bad.json"), []byte("{"), 0644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(10), logging.LevelInfo, io.Discard)
	if _, err := loadAgents(logger); err == nil {
		t.Fatalf("expected error for invalid agent json")
	}
}
