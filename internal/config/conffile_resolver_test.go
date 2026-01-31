package config

import (
	"bytes"
	"strings"
	"testing"
)

func TestConffileResolverInvalidInputReprompts(t *testing.T) {
	input := strings.NewReader("x\n\n")
	output := &bytes.Buffer{}
	resolver := ConffileResolver{
		Interactive: true,
		In:          input,
		Out:         output,
	}

	choice, err := resolver.ResolveConflict(ConffilePrompt{
		RelPath:  "agents/example.toml",
		DestPath: "/tmp/example.toml",
		NewBytes: []byte("new"),
	})
	if err != nil {
		t.Fatalf("resolve conflict: %v", err)
	}
	if choice.Action != ConffileKeep {
		t.Fatalf("expected keep action, got %v", choice.Action)
	}
	if !strings.Contains(output.String(), "Please enter Y, N, D, or A.") {
		t.Fatalf("expected invalid choice message")
	}
	if strings.Count(output.String(), "Configuration file 'config/agents/example.toml'") < 2 {
		t.Fatalf("expected prompt to repeat after invalid input")
	}
}

func TestConffileResolverDiffThenKeep(t *testing.T) {
	input := strings.NewReader("d\nn\n")
	output := &bytes.Buffer{}
	resolver := ConffileResolver{
		Interactive: true,
		In:          input,
		Out:         output,
		DiffRunner: func(oldPath, newPath string) (string, error) {
			return "--- old\n+++ new\n", nil
		},
	}

	choice, err := resolver.ResolveConflict(ConffilePrompt{
		RelPath:  "agents/example.toml",
		DestPath: "/tmp/example.toml",
		NewBytes: []byte("new"),
	})
	if err != nil {
		t.Fatalf("resolve conflict: %v", err)
	}
	if choice.Action != ConffileKeep {
		t.Fatalf("expected keep action, got %v", choice.Action)
	}
	if !strings.Contains(output.String(), "--- old") {
		t.Fatalf("expected diff output in prompt")
	}
	if strings.Count(output.String(), "What do you want to do? [N] ") != 2 {
		t.Fatalf("expected prompt to repeat after diff")
	}
}

func TestConffileResolverInstallChoice(t *testing.T) {
	input := strings.NewReader("y\n")
	output := &bytes.Buffer{}
	resolver := ConffileResolver{
		Interactive: true,
		In:          input,
		Out:         output,
	}

	choice, err := resolver.ResolveConflict(ConffilePrompt{
		RelPath:  "agents/example.toml",
		DestPath: "/tmp/example.toml",
		NewBytes: []byte("new"),
	})
	if err != nil {
		t.Fatalf("resolve conflict: %v", err)
	}
	if choice.Action != ConffileInstall {
		t.Fatalf("expected install action, got %v", choice.Action)
	}
	if !strings.Contains(output.String(), "Configuration file 'config/agents/example.toml'") {
		t.Fatalf("expected prompt to include config path")
	}
	if !strings.Contains(output.String(), "What do you want to do? [N] ") {
		t.Fatalf("expected prompt to include default action")
	}
}

func TestConffileResolverApplyAllInstalls(t *testing.T) {
	input := strings.NewReader("a\n")
	output := &bytes.Buffer{}
	resolver := ConffileResolver{
		Interactive: true,
		In:          input,
		Out:         output,
	}

	choice, err := resolver.ResolveConflict(ConffilePrompt{
		RelPath:  "agents/example.toml",
		DestPath: "/tmp/example.toml",
		NewBytes: []byte("new"),
	})
	if err != nil {
		t.Fatalf("resolve conflict: %v", err)
	}
	if choice.Action != ConffileInstall {
		t.Fatalf("expected install action, got %v", choice.Action)
	}

	choice, err = resolver.ResolveConflict(ConffilePrompt{
		RelPath:  "agents/second.toml",
		DestPath: "/tmp/second.toml",
		NewBytes: []byte("newer"),
	})
	if err != nil {
		t.Fatalf("resolve conflict after apply-all: %v", err)
	}
	if choice.Action != ConffileInstall {
		t.Fatalf("expected install action after apply-all, got %v", choice.Action)
	}
}

func TestConffileResolverEmptyInputKeeps(t *testing.T) {
	input := strings.NewReader("\n")
	output := &bytes.Buffer{}
	resolver := ConffileResolver{
		Interactive: true,
		In:          input,
		Out:         output,
	}

	choice, err := resolver.ResolveConflict(ConffilePrompt{
		RelPath:  "agents/example.toml",
		DestPath: "/tmp/example.toml",
		NewBytes: []byte("new"),
	})
	if err != nil {
		t.Fatalf("resolve conflict: %v", err)
	}
	if choice.Action != ConffileKeep {
		t.Fatalf("expected keep action, got %v", choice.Action)
	}
	if !strings.Contains(output.String(), "The default action is to keep your current version.") {
		t.Fatalf("expected default action message")
	}
}
