package agent

import (
	"errors"
	"os"
	"testing"
)

func TestRegistryLoadOrReload(t *testing.T) {
	dir := t.TempDir()
	writeAgentFile(t, dir, "codex", "Codex", "/bin/bash")

	registry := NewRegistry(RegistryOptions{
		Agents:    map[string]Agent{},
		AgentsDir: dir,
	})
	first, reloaded, err := registry.LoadOrReload("codex")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if reloaded {
		t.Fatalf("expected reloaded=false")
	}
	if first == nil || first.Shell != "/bin/bash" {
		t.Fatalf("unexpected shell: %#v", first)
	}

	writeAgentFile(t, dir, "codex", "Codex", "/bin/zsh")
	second, reloaded, err := registry.LoadOrReload("codex")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded {
		t.Fatalf("expected reloaded=true")
	}
	if second == nil || second.Shell != "/bin/zsh" {
		t.Fatalf("unexpected shell after reload: %#v", second)
	}
}

func TestRegistrySnapshot(t *testing.T) {
	registry := NewRegistry(RegistryOptions{
		Agents: map[string]Agent{
			"codex": {Name: "Codex", Shell: "/bin/bash"},
		},
	})

	snapshot := registry.Snapshot()
	if _, ok := snapshot["codex"]; !ok {
		t.Fatalf("expected codex in snapshot")
	}
	snapshot["new"] = Agent{Name: "New"}
	if _, ok := registry.Get("new"); ok {
		t.Fatalf("expected snapshot to be a copy")
	}
}

func TestRegistryMissingAgent(t *testing.T) {
	registry := NewRegistry(RegistryOptions{Agents: map[string]Agent{}})
	_, _, err := registry.LoadOrReload("missing")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}
