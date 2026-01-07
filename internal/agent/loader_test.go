package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"gestalt/internal/logging"
)

func TestLoaderMissingDir(t *testing.T) {
	loader := Loader{}
	agents, err := loader.Load(nil, filepath.Join(t.TempDir(), "missing"), "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected empty map, got %d", len(agents))
	}
}

func TestLoaderValidAgents(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex.json"), []byte(`{
		"name": "Codex",
		"shell": "/bin/bash",
		"llm_type": "codex",
		"llm_model": "default"
	}`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte(`ignore`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	loader := Loader{}
	agents, err := loader.Load(nil, dir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	agent, ok := agents["codex"]
	if !ok {
		t.Fatalf("missing codex agent")
	}
	if agent.Name != "Codex" {
		t.Fatalf("name mismatch: %q", agent.Name)
	}
}

func TestLoaderInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	loader := Loader{}
	if _, err := loader.Load(nil, dir, "", nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoaderDuplicateAgentName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.json"), []byte(`{
		"name": "Coder",
		"shell": "/bin/bash"
	}`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.json"), []byte(`{
		"name": "Coder",
		"shell": "/bin/zsh"
	}`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	loader := Loader{}
	if _, err := loader.Load(nil, dir, "", nil); err == nil {
		t.Fatalf("expected error")
	} else if !strings.Contains(err.Error(), "duplicate agent name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoaderInvalidAgent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{
		"name": "Bad",
		"shell": " "
	}`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	loader := Loader{}
	if _, err := loader.Load(nil, dir, "", nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoaderExampleAgents(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Dir(filepath.Dir(wd))
	dir := filepath.Join(root, "config", "agents")
	promptsDir := filepath.Join(root, "config", "prompts")

	loader := Loader{}
	agents, err := loader.Load(nil, dir, promptsDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) == 0 {
		t.Fatalf("expected agents in %s", dir)
	}

	expected := []string{"copilot", "codex", "promptline"}
	for _, id := range expected {
		if _, ok := agents[id]; !ok {
			t.Fatalf("missing agent %q", id)
		}
	}
}

func TestLoaderMissingPromptLogsWarning(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex.json"), []byte(`{
		"name": "Codex",
		"shell": "/bin/bash",
		"prompt": ["missing"],
		"llm_type": "codex"
	}`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	promptsDir := t.TempDir()

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	loader := Loader{Logger: logger}
	if _, err := loader.Load(nil, dir, promptsDir, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := buffer.List()
	if len(entries) == 0 {
		t.Fatalf("expected warning log entry")
	}
	found := false
	for _, entry := range entries {
		if entry.Level == logging.LevelWarning && entry.Message == "agent prompt file missing" {
			found = true
			if entry.Context["prompt"] != "missing" {
				t.Fatalf("prompt context mismatch: %q", entry.Context["prompt"])
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected warning log for missing prompt")
	}
}

func TestLoaderMissingSkillLogsWarning(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex.json"), []byte(`{
		"name": "Codex",
		"shell": "/bin/bash",
		"skills": ["git-workflows", "missing-skill"]
	}`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	loader := Loader{Logger: logger}
	skillIndex := map[string]struct{}{
		"git-workflows": {},
	}
	agents, err := loader.Load(nil, dir, "", skillIndex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	agent, ok := agents["codex"]
	if !ok {
		t.Fatalf("missing codex agent")
	}
	if len(agent.Skills) != 1 || agent.Skills[0] != "git-workflows" {
		t.Fatalf("skills mismatch: %v", agent.Skills)
	}

	entries := buffer.List()
	if len(entries) == 0 {
		t.Fatalf("expected warning log entry")
	}
	found := false
	for _, entry := range entries {
		if entry.Level == logging.LevelWarning && entry.Message == "agent skill missing" {
			found = true
			if entry.Context["skill"] != "missing-skill" {
				t.Fatalf("skill context mismatch: %q", entry.Context["skill"])
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected warning log for missing skill")
	}
}

func TestLoaderWithFS(t *testing.T) {
	fsys := fstest.MapFS{
		"config/agents/codex.json": &fstest.MapFile{
			Data: []byte(`{
				"name": "Codex",
				"shell": "/bin/bash",
				"llm_type": "codex"
			}`),
		},
		"config/prompts/init.txt": &fstest.MapFile{
			Data: []byte("echo ok"),
		},
	}

	loader := Loader{}
	agents, err := loader.Load(fsys, "config/agents", "config/prompts", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if _, ok := agents["codex"]; !ok {
		t.Fatalf("missing codex agent")
	}
}
