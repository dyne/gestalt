package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoaderMissingDir(t *testing.T) {
	loader := Loader{}
	agents, err := loader.Load(filepath.Join(t.TempDir(), "missing"))
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
	agents, err := loader.Load(dir)
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
	if _, err := loader.Load(dir); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoaderInvalidAgent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{
		"name": "Bad",
		"shell": "/bin/bash",
		"llm_type": "other"
	}`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	loader := Loader{}
	if _, err := loader.Load(dir); err == nil {
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

	loader := Loader{}
	agents, err := loader.Load(dir)
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
