package watcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGitWatcherPublishesBranchChange(t *testing.T) {
	workDir := t.TempDir()
	gitDir := filepath.Join(workDir, ".git")
	refsDir := filepath.Join(gitDir, "refs", "heads")
	if err := os.MkdirAll(refsDir, 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}

	headPath := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write head: %v", err)
	}

	fsWatcher, err := New()
	if err != nil {
		t.Skipf("skipping watcher test (fsnotify unavailable): %v", err)
	}
	hub := NewEventHub(context.Background(), fsWatcher)
	defer hub.Close()

	gitWatcher, err := StartGitWatcher(hub, workDir)
	if err != nil {
		t.Fatalf("start git watcher: %v", err)
	}
	if gitWatcher == nil {
		t.Fatalf("expected git watcher")
	}

	events := make(chan Event, 1)
	id := hub.Subscribe(EventTypeGitBranchChanged, func(event Event) {
		select {
		case events <- event:
		default:
		}
	})
	defer hub.Unsubscribe(id)

	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/feature\n"), 0o644); err != nil {
		t.Fatalf("update head: %v", err)
	}

	select {
	case event := <-events:
		if event.Path != "feature" {
			t.Fatalf("expected branch feature, got %q", event.Path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for branch change")
	}
}

func TestGitWatcherPublishesDetachedHead(t *testing.T) {
	workDir := t.TempDir()
	gitDir := filepath.Join(workDir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}

	headPath := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write head: %v", err)
	}

	fsWatcher, err := New()
	if err != nil {
		t.Skipf("skipping watcher test (fsnotify unavailable): %v", err)
	}
	hub := NewEventHub(context.Background(), fsWatcher)
	defer hub.Close()

	gitWatcher, err := StartGitWatcher(hub, workDir)
	if err != nil {
		t.Fatalf("start git watcher: %v", err)
	}
	if gitWatcher == nil {
		t.Fatalf("expected git watcher")
	}

	events := make(chan Event, 1)
	id := hub.Subscribe(EventTypeGitBranchChanged, func(event Event) {
		select {
		case events <- event:
		default:
		}
	})
	defer hub.Unsubscribe(id)

	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(headPath, []byte("9fceb02b1c9a1b2d3e4f5a6b7c8d9e0f1a2b3c4d\n"), 0o644); err != nil {
		t.Fatalf("write detached head: %v", err)
	}

	select {
	case event := <-events:
		if !strings.HasPrefix(event.Path, "detached@") {
			t.Fatalf("expected detached head, got %q", event.Path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for detached head")
	}
}
