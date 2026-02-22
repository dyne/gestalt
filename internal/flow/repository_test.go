package flow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryRepositoryLoadMissingConfigReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	repo := NewDirectoryRepository(filepath.Join(dir, "flows"), nil)

	cfg, err := repo.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Version != ConfigVersion {
		t.Fatalf("expected version %d, got %d", ConfigVersion, cfg.Version)
	}
	if len(cfg.Triggers) != 0 {
		t.Fatalf("expected no triggers, got %d", len(cfg.Triggers))
	}
	if len(cfg.BindingsByTriggerID) != 0 {
		t.Fatalf("expected no bindings, got %d", len(cfg.BindingsByTriggerID))
	}
}

func TestDirectoryRepositoryLoadAggregatesManagedYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	flowsDir := filepath.Join(dir, "flows")
	if err := os.MkdirAll(flowsDir, 0o755); err != nil {
		t.Fatalf("mkdir flows dir: %v", err)
	}

	one := []byte(`
id: trigger-one
label: Trigger One
event_type: file_changed
where:
  path: README.md
bindings:
  - activity_id: toast_notification
    config:
      level: info
      message_template: one
`)
	two := []byte(`
id: trigger-two
label: Trigger Two
event_type: git_branch_changed
where: {}
bindings: []
`)

	if err := os.WriteFile(filepath.Join(flowsDir, "one.flow.yaml"), one, 0o644); err != nil {
		t.Fatalf("write one.flow.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(flowsDir, "two.flow.yaml"), two, 0o644); err != nil {
		t.Fatalf("write two.flow.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(flowsDir, "README.md"), []byte("notes"), 0o644); err != nil {
		t.Fatalf("write unmanaged file: %v", err)
	}

	repo := NewDirectoryRepository(flowsDir, nil)
	cfg, err := repo.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(cfg.Triggers) != 2 {
		t.Fatalf("expected two triggers, got %d", len(cfg.Triggers))
	}
	if len(cfg.BindingsByTriggerID["trigger-one"]) != 1 {
		t.Fatalf("expected trigger-one binding to load")
	}
}

func TestDirectoryRepositorySaveWritesManagedFilesAndDeletesStaleManaged(t *testing.T) {
	dir := t.TempDir()
	flowsDir := filepath.Join(dir, "flows")
	if err := os.MkdirAll(flowsDir, 0o755); err != nil {
		t.Fatalf("mkdir flows dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(flowsDir, "stale.flow.yaml"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale flow file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(flowsDir, "notes.yaml"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write unmanaged file: %v", err)
	}

	cfg := Config{
		Version: ConfigVersion,
		Triggers: []EventTrigger{
			{
				ID:        "Trigger One",
				Label:     "Trigger One",
				EventType: "file_changed",
				Where:     map[string]string{"path": "README.md"},
			},
		},
		BindingsByTriggerID: map[string][]ActivityBinding{
			"Trigger One": {
				{
					ActivityID: "toast_notification",
					Config: map[string]any{
						"level":            "info",
						"message_template": "hello",
					},
				},
			},
		},
	}

	repo := NewDirectoryRepository(flowsDir, nil)
	if err := repo.Save(cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(flowsDir, "trigger-one.flow.yaml")); err != nil {
		t.Fatalf("expected normalized managed flow file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(flowsDir, "stale.flow.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected stale managed file removed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(flowsDir, "notes.yaml")); err != nil {
		t.Fatalf("expected unmanaged file retained: %v", err)
	}
}

func TestDirectoryRepositorySaveRejectsFilenameCollisions(t *testing.T) {
	dir := t.TempDir()
	repo := NewDirectoryRepository(filepath.Join(dir, "flows"), nil)

	cfg := Config{
		Version: ConfigVersion,
		Triggers: []EventTrigger{
			{ID: "Flow A", EventType: "file_changed"},
			{ID: "flow-a", EventType: "file_changed"},
		},
		BindingsByTriggerID: map[string][]ActivityBinding{},
	}
	err := repo.Save(cfg)
	if err == nil {
		t.Fatal("expected collision validation error")
	}
	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if validationErr.Kind != ValidationConflict {
		t.Fatalf("expected conflict error kind, got %q", validationErr.Kind)
	}
}

func TestDirectoryRepositoryLoadInvalidManagedYAMLReturnsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	flowsDir := filepath.Join(dir, "flows")
	if err := os.MkdirAll(flowsDir, 0o755); err != nil {
		t.Fatalf("mkdir flows dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(flowsDir, "bad.flow.yaml"), []byte("id: 123"), 0o644); err != nil {
		t.Fatalf("write invalid flow yaml: %v", err)
	}

	repo := NewDirectoryRepository(flowsDir, nil)
	_, err := repo.Load()
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}
