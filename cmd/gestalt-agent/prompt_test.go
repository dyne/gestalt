package main

import (
	"os"
	"path/filepath"
	"testing"

	"gestalt/internal/agent"
	"gestalt/internal/ports"
)

func TestRenderDeveloperPromptWithIncludes(t *testing.T) {
	workdir := t.TempDir()
	withWorkdir(t, workdir, func() {
		promptDir := filepath.Join("config", "prompts")
		if err := os.MkdirAll(promptDir, 0o755); err != nil {
			t.Fatalf("mkdir prompts: %v", err)
		}
		if err := os.WriteFile(filepath.Join(promptDir, "extra.md"), []byte("Extra line\n"), 0o644); err != nil {
			t.Fatalf("write include: %v", err)
		}
		coder := "Coder \"prompt\"\n{{include extra.md}}\n"
		architect := "Architect line\n"
		if err := os.WriteFile(filepath.Join(promptDir, "coder.tmpl"), []byte(coder), 0o644); err != nil {
			t.Fatalf("write coder prompt: %v", err)
		}
		if err := os.WriteFile(filepath.Join(promptDir, "architect.tmpl"), []byte(architect), 0o644); err != nil {
			t.Fatalf("write architect prompt: %v", err)
		}

		agent := agent.Agent{Prompts: agent.PromptList{"coder", "architect"}}
		resolver := ports.NewPortRegistry()
		content, err := renderDeveloperPrompt(agent, os.DirFS("."), configRoot, resolver)
		if err != nil {
			t.Fatalf("render developer prompt: %v", err)
		}
		expected := "Coder \"prompt\"\nExtra line\n\n\nArchitect line\n"
		if content != expected {
			t.Fatalf("unexpected prompt content:\n%q\nexpected:\n%q", content, expected)
		}
	})
}

func TestRenderDeveloperPromptPorts(t *testing.T) {
	workdir := t.TempDir()
	withWorkdir(t, workdir, func() {
		t.Setenv("GESTALT_PORT", "")
		t.Setenv("GESTALT_BACKEND_PORT", "")
		t.Setenv("GESTALT_TEMPORAL_HOST", "")
		t.Setenv("GESTALT_OTEL_HTTP_ENDPOINT", "")
		promptDir := filepath.Join("config", "prompts")
		if err := os.MkdirAll(promptDir, 0o755); err != nil {
			t.Fatalf("mkdir prompts: %v", err)
		}
		content := "{{port frontend}}\n{{port backend}}\n{{port temporal}}\n{{port otel}}\n"
		if err := os.WriteFile(filepath.Join(promptDir, "ports.tmpl"), []byte(content), 0o644); err != nil {
			t.Fatalf("write ports prompt: %v", err)
		}

		agent := agent.Agent{Prompts: agent.PromptList{"ports"}}
		resolver := defaultPortResolver()
		rendered, err := renderDeveloperPrompt(agent, os.DirFS("."), configRoot, resolver)
		if err != nil {
			t.Fatalf("render ports prompt: %v", err)
		}
		expected := "57417\n57417\n7233\n4318\n"
		if rendered != expected {
			t.Fatalf("unexpected ports output: %q", rendered)
		}
	})
}
