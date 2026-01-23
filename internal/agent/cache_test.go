package agent

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAgentCacheMiss(t *testing.T) {
	dir := t.TempDir()
	writeAgentFile(t, dir, "codex", "Codex", "/bin/bash")

	cache := NewAgentCache(nil)
	agent, reloaded, err := cache.LoadOrReload("codex", dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if reloaded {
		t.Fatalf("expected reloaded=false for cache miss")
	}
	if agent == nil || agent.Name != "Codex" {
		t.Fatalf("unexpected agent: %#v", agent)
	}
}

func TestAgentCacheHit(t *testing.T) {
	dir := t.TempDir()
	writeAgentFile(t, dir, "codex", "Codex", "/bin/bash")

	cache := NewAgentCache(nil)
	first, reloaded, err := cache.LoadOrReload("codex", dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if reloaded {
		t.Fatalf("expected reloaded=false for initial load")
	}
	second, reloaded, err := cache.LoadOrReload("codex", dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded {
		t.Fatalf("expected reloaded=false for cache hit")
	}
	if first != second {
		t.Fatalf("expected cached agent instance")
	}
}

func TestAgentCacheReload(t *testing.T) {
	dir := t.TempDir()
	writeAgentFile(t, dir, "codex", "Codex", "/bin/bash")

	cache := NewAgentCache(nil)
	first, _, err := cache.LoadOrReload("codex", dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	writeAgentFile(t, dir, "codex", "Codex", "/bin/zsh")
	second, reloaded, err := cache.LoadOrReload("codex", dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded {
		t.Fatalf("expected reloaded=true")
	}
	if second.Shell == first.Shell {
		t.Fatalf("expected shell to change")
	}
}

func TestAgentCacheClear(t *testing.T) {
	dir := t.TempDir()
	writeAgentFile(t, dir, "codex", "Codex", "/bin/bash")

	cache := NewAgentCache(nil)
	first, _, err := cache.LoadOrReload("codex", dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cache.Clear()
	second, reloaded, err := cache.LoadOrReload("codex", dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded {
		t.Fatalf("expected reloaded=false after clear")
	}
	if first == second {
		t.Fatalf("expected new cached agent after clear")
	}
}

func TestAgentCacheConcurrentLoad(t *testing.T) {
	dir := t.TempDir()
	writeAgentFile(t, dir, "codex", "Codex", "/bin/bash")

	cache := NewAgentCache(nil)
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := cache.LoadOrReload("codex", dir)
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func writeAgentFile(t *testing.T, dir, agentID, name, shell string) {
	t.Helper()
	path := filepath.Join(dir, agentID+".toml")
	data := []byte("name = \"" + name + "\"\n" + "shell = \"" + shell + "\"\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write agent: %v", err)
	}
}
