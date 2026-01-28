package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingConfigReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flow", "automations.json")
	repo := NewFileRepository(path, nil)

	cfg, err := repo.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Version != ConfigVersion {
		t.Fatalf("expected version %d, got %d", ConfigVersion, cfg.Version)
	}
	if len(cfg.Triggers) != 0 {
		t.Fatalf("expected empty triggers, got %d", len(cfg.Triggers))
	}
	if cfg.BindingsByTriggerID == nil || len(cfg.BindingsByTriggerID) != 0 {
		t.Fatalf("expected empty bindings map")
	}
}

func TestLoadCorruptConfigBacksUpFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flow", "automations.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	repo := NewFileRepository(path, nil)
	cfg, err := repo.Load()
	if err == nil || err != ErrInvalidConfig {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
	if cfg.Version != ConfigVersion {
		t.Fatalf("expected version %d, got %d", ConfigVersion, cfg.Version)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("expected original file to be moved, stat err: %v", statErr)
	}
	matches, _ := filepath.Glob(path + ".*.bck")
	if len(matches) == 0 {
		t.Fatalf("expected backup file to exist")
	}
}

func TestSaveConfigIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flow", "automations.json")
	repo := NewFileRepository(path, nil)

	cfg := Config{
		Version: ConfigVersion,
		Triggers: []EventTrigger{
			{
				ID:        "t1",
				Label:     "Trigger",
				EventType: "workflow_paused",
				Where:     map[string]string{"terminal_id": "t1"},
			},
		},
		BindingsByTriggerID: map[string][]ActivityBinding{
			"t1": {
				{ActivityID: "toast_notification", Config: map[string]any{"message": "hello"}},
			},
		},
	}

	if err := repo.Save(cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !strings.Contains(string(data), "\"version\"") {
		t.Fatalf("expected saved config to include version")
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("readdir failed: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "automations-") {
			t.Fatalf("unexpected temp file %s", entry.Name())
		}
	}
}
