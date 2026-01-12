package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/scip"
	"gestalt/internal/watcher"
)

func TestSCIPAutoReindexTriggersForStaleIndex(t *testing.T) {
	projectDir := t.TempDir()
	sourcePath := filepath.Join(projectDir, "main.go")
	if err := os.WriteFile(sourcePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	indexPath := filepath.Join(projectDir, "index.db")
	if err := buildTestSCIPDB(indexPath); err != nil {
		t.Fatalf("build test db: %v", err)
	}

	meta, err := scip.BuildMetadata(projectDir, []string{"go"})
	if err != nil {
		t.Fatalf("BuildMetadata failed: %v", err)
	}
	meta.CreatedAt = time.Now().Add(-48 * time.Hour)
	if err := scip.SaveMetadata(indexPath, meta); err != nil {
		t.Fatalf("SaveMetadata failed: %v", err)
	}

	handler, err := NewSCIPHandler(indexPath, nil, SCIPHandlerOptions{ProjectRoot: projectDir})
	if err != nil {
		t.Fatalf("NewSCIPHandler failed: %v", err)
	}
	handler.autoReindex = true
	handler.autoReindexMaxAge = 24 * time.Hour

	called := make(chan string, 1)
	handler.enqueueReindex = func(path string) {
		called <- path
	}

	handler.maybeAutoReindex()

	select {
	case path := <-called:
		if path != projectDir {
			t.Fatalf("expected reindex path %q, got %q", projectDir, path)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected auto-reindex to be triggered")
	}
}

func TestSCIPFileWatcherQueuesReindex(t *testing.T) {
	projectDir := t.TempDir()
	sourcePath := filepath.Join(projectDir, "main.go")
	if err := os.WriteFile(sourcePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	indexPath := filepath.Join(projectDir, "index.db")
	if err := buildTestSCIPDB(indexPath); err != nil {
		t.Fatalf("build test db: %v", err)
	}

	bus := event.NewBus[watcher.Event](context.Background(), event.BusOptions{Name: "scip_test"})
	t.Cleanup(bus.Close)

	handler, err := NewSCIPHandler(indexPath, nil, SCIPHandlerOptions{
		ProjectRoot:        projectDir,
		AutoReindex:        true,
		AutoReindexOnStart: false,
		EventBus:           bus,
		WatchDebounce:      10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewSCIPHandler failed: %v", err)
	}

	called := make(chan string, 1)
	handler.enqueueReindex = func(path string) {
		called <- path
	}

	bus.Publish(watcher.Event{
		Type:      watcher.EventTypeFileChanged,
		Path:      sourcePath,
		Timestamp: time.Now().UTC(),
	})

	select {
	case path := <-called:
		if path != projectDir {
			t.Fatalf("expected reindex path %q, got %q", projectDir, path)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected watcher to queue reindex")
	}
}
