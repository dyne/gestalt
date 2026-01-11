package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractConfigIsNoOp(t *testing.T) {
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
		filepath.Join(root, "gestalt"),
		filepath.Join(root, ".gestalt"),
	} {
		_, err := os.Stat(dir)
		if err == nil {
			t.Fatalf("expected %s to not exist", dir)
		}
		if !os.IsNotExist(err) {
			t.Fatalf("unexpected stat error for %s: %v", dir, err)
		}
	}
}
