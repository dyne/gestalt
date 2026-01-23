package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateConfigValidDir(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := []byte("name = \"Codex\"\nshell = \"/bin/bash\"\n")
	if err := os.WriteFile(filepath.Join(agentsDir, "codex.toml"), data, 0o644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runValidateConfigWithOutput([]string{"--agents-dir", agentsDir}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Summary: 1 valid, 0 invalid") {
		t.Fatalf("unexpected summary: %q", out.String())
	}
}

func TestValidateConfigInvalidDir(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := []byte("name = \"Bad\"\nshell = \" \"\n")
	if err := os.WriteFile(filepath.Join(agentsDir, "bad.toml"), data, 0o644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runValidateConfigWithOutput([]string{"--agents-dir", agentsDir}, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if !strings.Contains(out.String(), "Summary: 0 valid, 1 invalid") {
		t.Fatalf("unexpected summary: %q", out.String())
	}
}

func TestValidateConfigEmptyDir(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runValidateConfigWithOutput([]string{"--agents-dir", agentsDir}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out.String(), "Summary: 0 valid, 0 invalid") {
		t.Fatalf("unexpected summary: %q", out.String())
	}
}

func TestValidateConfigWarnsOnLegacyMemo(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, ".gestalt", "config", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	data := []byte("name = \"Codex\"\nshell = \"/bin/bash\"\n")
	if err := os.WriteFile(filepath.Join(agentsDir, "codex.toml"), data, 0o644); err != nil {
		t.Fatalf("write agent: %v", err)
	}
	temporalDir := filepath.Join(root, ".gestalt", "temporal")
	if err := os.MkdirAll(temporalDir, 0o755); err != nil {
		t.Fatalf("mkdir temporal: %v", err)
	}
	memoPath := filepath.Join(temporalDir, "memo.json")
	legacyMemo := []byte(`{"agent_config":"{\"name\":\"Legacy\"}"}`)
	if err := os.WriteFile(memoPath, legacyMemo, 0o644); err != nil {
		t.Fatalf("write memo: %v", err)
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

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runValidateConfigWithOutput([]string{"--agents-dir", agentsDir}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "legacy JSON agent_config memos detected") {
		t.Fatalf("expected legacy memo warning, got %q", errOut.String())
	}
}
