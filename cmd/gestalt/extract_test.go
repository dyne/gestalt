package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractConfigCreatesLayout(t *testing.T) {
	root := t.TempDir()
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

	if code := runExtractConfig(); code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	for _, dir := range []string{
		filepath.Join("gestalt", "config", "agents"),
		filepath.Join("gestalt", "config", "prompts"),
		filepath.Join("gestalt", "config", "skills"),
		filepath.Join("gestalt", "dist"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", dir)
		}
	}
}
