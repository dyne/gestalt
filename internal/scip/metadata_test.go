//go:build !noscip

package scip

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMetadataRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "index.db")
	meta := IndexMetadata{
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		ProjectRoot: tempDir,
		Languages:   []string{"go"},
		FilesHashed: "abc123",
	}

	if err := SaveMetadata(indexPath, meta); err != nil {
		t.Fatalf("SaveMetadata failed: %v", err)
	}

	loaded, err := LoadMetadata(indexPath)
	if err != nil {
		t.Fatalf("LoadMetadata failed: %v", err)
	}
	if !loaded.CreatedAt.Equal(meta.CreatedAt) {
		t.Fatalf("expected created_at %s, got %s", meta.CreatedAt, loaded.CreatedAt)
	}
	if loaded.ProjectRoot != meta.ProjectRoot {
		t.Fatalf("expected project root %q, got %q", meta.ProjectRoot, loaded.ProjectRoot)
	}
	if loaded.FilesHashed != meta.FilesHashed {
		t.Fatalf("expected files hash %q, got %q", meta.FilesHashed, loaded.FilesHashed)
	}
}

func TestIsFreshDetectsChanges(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	meta, err := BuildMetadata(tempDir, []string{"go"})
	if err != nil {
		t.Fatalf("BuildMetadata failed: %v", err)
	}

	fresh, err := IsFresh(meta)
	if err != nil {
		t.Fatalf("IsFresh failed: %v", err)
	}
	if !fresh {
		t.Fatalf("expected metadata to be fresh")
	}

	if err := os.WriteFile(filePath, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("rewrite file: %v", err)
	}

	fresh, err = IsFresh(meta)
	if err != nil {
		t.Fatalf("IsFresh failed after change: %v", err)
	}
	if fresh {
		t.Fatalf("expected metadata to be stale after change")
	}
}
