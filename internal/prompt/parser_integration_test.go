package prompt

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//go:embed testdata/embedded/config/prompts/*
var embeddedPromptsFS embed.FS

type overlayFS struct {
	primary  fs.FS
	fallback fs.FS
}

func (o overlayFS) Open(name string) (fs.File, error) {
	file, err := o.primary.Open(name)
	if err == nil {
		return file, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return o.fallback.Open(name)
	}
	return nil, err
}

func embeddedConfigFS(t *testing.T) fs.FS {
	t.Helper()
	sub, err := fs.Sub(embeddedPromptsFS, "testdata/embedded")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	return sub
}

func TestRenderFromFilesystem(t *testing.T) {
	root := t.TempDir()
	promptsDir := filepath.Join(root, "config", "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "local.tmpl"), []byte("Local start\n{{include fragment.txt}}\nLocal end\n"), 0644); err != nil {
		t.Fatalf("write local template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "fragment.txt"), []byte("local fragment\n"), 0644); err != nil {
		t.Fatalf("write fragment: %v", err)
	}

	parser := NewParser(os.DirFS(root), "config/prompts", root)
	result, err := parser.Render("local")
	if err != nil {
		t.Fatalf("render local: %v", err)
	}
	expectedContent := "Local start\nlocal fragment\nLocal end\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"local.tmpl", "fragment.txt"}
	if strings.Join(result.Files, ",") != strings.Join(expectedFiles, ",") {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderIncludeFromWorkdirRoot(t *testing.T) {
	root := t.TempDir()
	promptsDir := filepath.Join(root, "config", "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("root notes\n"), 0644); err != nil {
		t.Fatalf("write root include: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "root-include.tmpl"), []byte("Start\n{{include notes.md}}\nEnd\n"), 0644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	parser := NewParser(os.DirFS(root), "config/prompts", root)
	result, err := parser.Render("root-include")
	if err != nil {
		t.Fatalf("render root-include: %v", err)
	}
	expectedContent := "Start\nroot notes\nEnd\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"root-include.tmpl", "notes.md"}
	if strings.Join(result.Files, ",") != strings.Join(expectedFiles, ",") {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderSkipsBinaryInclude(t *testing.T) {
	root := t.TempDir()
	promptsDir := filepath.Join(root, "config", "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	binaryPath := filepath.Join(root, "binary.dat")
	if err := os.WriteFile(binaryPath, []byte{0x00, 0x01, 0x02}, 0644); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "binary-include.tmpl"), []byte("Before\n{{include binary.dat}}\nAfter\n"), 0644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	parser := NewParser(os.DirFS(root), "config/prompts", root)
	result, err := parser.Render("binary-include")
	if err != nil {
		t.Fatalf("render binary-include: %v", err)
	}
	expectedContent := "Before\nAfter\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"binary-include.tmpl"}
	if strings.Join(result.Files, ",") != strings.Join(expectedFiles, ",") {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderFromEmbeddedFS(t *testing.T) {
	parser := NewParser(embeddedConfigFS(t), "config/prompts", ".")
	result, err := parser.Render("embedded")
	if err != nil {
		t.Fatalf("render embedded: %v", err)
	}
	expectedContent := "Embedded start\nembedded fragment\nEmbedded end\n"
	if string(result.Content) != expectedContent {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	expectedFiles := []string{"embedded.tmpl", "embedded-fragment.txt"}
	if strings.Join(result.Files, ",") != strings.Join(expectedFiles, ",") {
		t.Fatalf("unexpected files: %#v", result.Files)
	}
}

func TestRenderOverlayFSUsesExternal(t *testing.T) {
	externalRoot := t.TempDir()
	promptsDir := filepath.Join(externalRoot, "config", "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "override.tmpl"), []byte("external version\n"), 0644); err != nil {
		t.Fatalf("write override: %v", err)
	}

	parser := NewParser(overlayFS{
		primary:  os.DirFS(externalRoot),
		fallback: embeddedConfigFS(t),
	}, "config/prompts", externalRoot)

	result, err := parser.Render("override")
	if err != nil {
		t.Fatalf("render override: %v", err)
	}
	if string(result.Content) != "external version\n" {
		t.Fatalf("unexpected content: %q", string(result.Content))
	}
	if len(result.Files) != 1 || result.Files[0] != "override.tmpl" {
		t.Fatalf("unexpected files: %#v", result.Files)
	}

	result, err = parser.Render("embedded")
	if err != nil {
		t.Fatalf("render embedded: %v", err)
	}
	if !strings.Contains(string(result.Content), "Embedded start") {
		t.Fatalf("expected embedded content, got %q", string(result.Content))
	}
}
