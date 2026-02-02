package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"gestalt/internal/ports"
	"gestalt/internal/prompt"
)

func TestEnsureExtractedConfig(t *testing.T) {
	workdir := t.TempDir()
	withWorkdir(t, workdir, func() {
		if err := ensureExtractedConfig(defaultConfigDir, bytes.NewReader(nil), io.Discard); err != nil {
			t.Fatalf("ensure extracted config: %v", err)
		}
		path := filepath.Join(defaultConfigDir, "agents", "coder.toml")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected extracted agent at %s: %v", path, err)
		}
	})
}

func TestConfigOverlayPrefersLocalPrompt(t *testing.T) {
	workdir := t.TempDir()
	withWorkdir(t, workdir, func() {
		fallbackPath := filepath.Join(".gestalt", "config", "prompts")
		localPath := filepath.Join("config", "prompts")
		if err := os.MkdirAll(fallbackPath, 0o755); err != nil {
			t.Fatalf("mkdir fallback: %v", err)
		}
		if err := os.MkdirAll(localPath, 0o755); err != nil {
			t.Fatalf("mkdir local: %v", err)
		}
		if err := os.WriteFile(filepath.Join(fallbackPath, "coder.tmpl"), []byte("fallback\n"), 0o644); err != nil {
			t.Fatalf("write fallback prompt: %v", err)
		}
		if err := os.WriteFile(filepath.Join(localPath, "coder.tmpl"), []byte("local\n"), 0o644); err != nil {
			t.Fatalf("write local prompt: %v", err)
		}

		fsys, root := buildConfigOverlay(defaultConfigDir)
		parser := prompt.NewParser(fsys, filepath.ToSlash(filepath.Join(root, "prompts")), ".", ports.NewPortRegistry())
		result, err := parser.Render("coder")
		if err != nil {
			t.Fatalf("render prompt: %v", err)
		}
		if string(result.Content) != "local\n" {
			t.Fatalf("expected local prompt, got %q", string(result.Content))
		}
	})
}

func TestConfigOverlayPrefersLocalAgent(t *testing.T) {
	workdir := t.TempDir()
	withWorkdir(t, workdir, func() {
		fallbackPath := filepath.Join(".gestalt", "config", "agents")
		localPath := filepath.Join("config", "agents")
		if err := os.MkdirAll(fallbackPath, 0o755); err != nil {
			t.Fatalf("mkdir fallback: %v", err)
		}
		if err := os.MkdirAll(localPath, 0o755); err != nil {
			t.Fatalf("mkdir local: %v", err)
		}
		if err := os.WriteFile(filepath.Join(fallbackPath, "coder.toml"), []byte("name=\"Fallback\"\nshell=\"bash\"\n"), 0o644); err != nil {
			t.Fatalf("write fallback agent: %v", err)
		}
		if err := os.WriteFile(filepath.Join(localPath, "coder.toml"), []byte("name=\"Local\"\nshell=\"bash\"\n"), 0o644); err != nil {
			t.Fatalf("write local agent: %v", err)
		}

		fsys, root := buildConfigOverlay(defaultConfigDir)
		agents, err := loadAgents(fsys, root)
		if err != nil {
			t.Fatalf("load agents: %v", err)
		}
		agent, ok := agents["coder"]
		if !ok {
			t.Fatalf("expected coder agent")
		}
		if agent.Name != "Local" {
			t.Fatalf("expected local agent, got %q", agent.Name)
		}
	})
}

func withWorkdir(t *testing.T, dir string, fn func()) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore chdir: %v", err)
		}
	}()
	fn()
}
