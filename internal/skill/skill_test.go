package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFileValid(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "git-workflows")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	path := filepath.Join(skillDir, "SKILL.md")
	content := `---
name: git-workflows
description: Helpful git workflows
license: MIT
compatibility: ">=1.0"
metadata:
  owner: dyne
allowed_tools:
  - bash
  - ""
---

# Git Workflows
Run these steps.
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	skill, err := ParseFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if skill.Name != "git-workflows" {
		t.Fatalf("name mismatch: %q", skill.Name)
	}
	if skill.Description != "Helpful git workflows" {
		t.Fatalf("description mismatch: %q", skill.Description)
	}
	if skill.License != "MIT" {
		t.Fatalf("license mismatch: %q", skill.License)
	}
	if skill.Compatibility != ">=1.0" {
		t.Fatalf("compatibility mismatch: %q", skill.Compatibility)
	}
	if len(skill.AllowedTools) != 1 || skill.AllowedTools[0] != "bash" {
		t.Fatalf("allowed tools mismatch: %v", skill.AllowedTools)
	}
	if !strings.Contains(skill.Content, "# Git Workflows") {
		t.Fatalf("content missing body: %q", skill.Content)
	}
}

func TestParseMissingFrontmatter(t *testing.T) {
	if _, err := Parse([]byte("no frontmatter")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseFileNameMismatch(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "git-workflows")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	content := `---
name: other
description: Helpful git workflows
---

# Git Workflows
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := ParseFile(path)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "does not match directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateNameRules(t *testing.T) {
	tests := []struct {
		name    string
		skill   Skill
		wantErr string
	}{
		{
			name: "uppercase",
			skill: Skill{
				Name:        "Git-Workflows",
				Description: "desc",
			},
			wantErr: "invalid",
		},
		{
			name: "leading hyphen",
			skill: Skill{
				Name:        "-git",
				Description: "desc",
			},
			wantErr: "invalid",
		},
		{
			name: "trailing hyphen",
			skill: Skill{
				Name:        "git-",
				Description: "desc",
			},
			wantErr: "invalid",
		},
		{
			name: "too long",
			skill: Skill{
				Name:        strings.Repeat("a", 65),
				Description: "desc",
			},
			wantErr: "1-64",
		},
		{
			name: "missing description",
			skill: Skill{
				Name:        "git-workflows",
				Description: " ",
			},
			wantErr: "description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.skill.Validate()
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateOptionalDirs(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "git-workflows")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(skillDir, "scripts")
	if err := os.WriteFile(path, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	skill := Skill{
		Name:        "git-workflows",
		Description: "desc",
		Path:        skillDir,
	}
	err := skill.Validate()
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "scripts") {
		t.Fatalf("unexpected error: %v", err)
	}
}
