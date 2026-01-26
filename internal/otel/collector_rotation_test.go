package otel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRotateCollectorFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "otel.json")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	rotated, err := rotateCollectorFile(path)
	if err != nil {
		t.Fatalf("rotateCollectorFile error: %v", err)
	}
	if rotated == "" {
		t.Fatalf("expected rotated file path")
	}
	if _, err := os.Stat(rotated); err != nil {
		t.Fatalf("expected rotated file to exist: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected original file to be renamed")
	}
	if !strings.HasPrefix(filepath.Base(rotated), "otel-") || !strings.HasSuffix(rotated, ".json") {
		t.Fatalf("unexpected rotated file name: %s", rotated)
	}
}

func TestPruneRotatedFilesMaxFiles(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		filepath.Join(dir, "otel-20240101-000000.json"),
		filepath.Join(dir, "otel-20240102-000000.json"),
		filepath.Join(dir, "otel-20240103-000000.json"),
	}
	for index, path := range paths {
		if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		modTime := time.Now().Add(time.Duration(-index) * time.Hour)
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}

	pruneRotatedFiles(dir, rotationConfig{MaxFiles: 2})

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	remaining := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "otel-") {
			remaining++
		}
	}
	if remaining != 2 {
		t.Fatalf("expected 2 rotated files, got %d", remaining)
	}
}

func TestPruneRotatedFilesMaxAge(t *testing.T) {
	dir := t.TempDir()
	oldFile := filepath.Join(dir, "otel-20240101-000000.json")
	if err := os.WriteFile(oldFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	pruneRotatedFiles(dir, rotationConfig{MaxAge: 24 * time.Hour, MaxFiles: 5})

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("expected old file to be pruned")
	}
}
