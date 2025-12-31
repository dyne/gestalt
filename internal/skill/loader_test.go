package skill

import (
	"os"
	"path/filepath"
	"testing"

	"gestalt/internal/logging"
)

func TestLoaderMissingDir(t *testing.T) {
	loader := Loader{}
	skills, err := loader.Load(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected empty map, got %d", len(skills))
	}
}

func TestLoaderValidSkill(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "git-workflows")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `---
name: git-workflows
description: Helpful git workflows
---

# Git Workflows
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	loader := Loader{}
	skills, err := loader.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	skill, ok := skills["git-workflows"]
	if !ok {
		t.Fatalf("missing git-workflows skill")
	}
	if skill.Name != "git-workflows" {
		t.Fatalf("name mismatch: %q", skill.Name)
	}
}

func TestLoaderMissingSkillFileLogsWarning(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	loader := Loader{Logger: logger}
	skills, err := loader.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected no skills, got %d", len(skills))
	}

	entries := buffer.List()
	if len(entries) == 0 {
		t.Fatalf("expected warning log entry")
	}
	found := false
	for _, entry := range entries {
		if entry.Level == logging.LevelWarning && entry.Message == "skill load failed" {
			found = true
			if entry.Context["skill_id"] != "empty" {
				t.Fatalf("skill_id mismatch: %q", entry.Context["skill_id"])
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected warning log for missing SKILL.md")
	}
}

func TestLoaderInvalidSkillLogsWarning(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "bad-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `---
name: Bad-Skill
description: invalid name format
---
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	loader := Loader{Logger: logger}
	skills, err := loader.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected no skills, got %d", len(skills))
	}

	entries := buffer.List()
	if len(entries) == 0 {
		t.Fatalf("expected warning log entry")
	}
	found := false
	for _, entry := range entries {
		if entry.Level == logging.LevelWarning && entry.Message == "skill load failed" {
			found = true
			if entry.Context["skill_id"] != "bad-skill" {
				t.Fatalf("skill_id mismatch: %q", entry.Context["skill_id"])
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected warning log for invalid SKILL.md")
	}
}
