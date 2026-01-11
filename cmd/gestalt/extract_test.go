package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"gestalt/internal/config"
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

func TestExtractConfigEmitsEvents(t *testing.T) {
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

	bus := config.Bus()
	events, cancel := bus.Subscribe()
	defer cancel()

	var mu sync.Mutex
	extracted := 0
	done := make(chan struct{})
	go func() {
		for event := range events {
			if event.Type() == "config_extracted" {
				mu.Lock()
				extracted++
				mu.Unlock()
			}
		}
		close(done)
	}()

	if code := runExtractConfig(); code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if extracted == 0 {
		t.Fatalf("expected config_extracted events")
	}
}
