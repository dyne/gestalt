package watcher

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/git"
)

// GitWatcher monitors HEAD changes and emits git branch change events.
type GitWatcher struct {
	bus           *event.Bus[Event]
	headPath      string
	currentBranch string
	watchHandle   Handle
	mutex         sync.Mutex
}

// StartGitWatcher creates a GitWatcher if the working directory is a git repo.
func StartGitWatcher(bus *event.Bus[Event], watch Watch, workDir string) (*GitWatcher, error) {
	if bus == nil {
		return nil, errors.New("event bus is nil")
	}
	if watch == nil {
		return nil, errors.New("watcher is nil")
	}
	if strings.TrimSpace(workDir) == "" {
		workDir = "."
	}

	gitDir := git.ResolveGitDir(workDir)
	if gitDir == "" {
		return nil, nil
	}

	headPath := filepath.Join(gitDir, "HEAD")
	watcher := &GitWatcher{
		bus:           bus,
		headPath:      headPath,
		currentBranch: git.ReadGitBranch(headPath),
	}

	handle, err := watch.Watch(headPath, func(event Event) {
		watcher.handleFileChanged(event)
	})
	if err != nil {
		return nil, err
	}
	watcher.watchHandle = handle

	return watcher, nil
}

// Close stops watching git branch changes.
func (watcher *GitWatcher) Close() {
	if watcher == nil {
		return
	}
	if watcher.watchHandle != nil {
		_ = watcher.watchHandle.Close()
		watcher.watchHandle = nil
	}
}

func (watcher *GitWatcher) handleFileChanged(event Event) {
	if event.Path != watcher.headPath {
		return
	}
	branch := git.ReadGitBranch(watcher.headPath)
	if branch == "" {
		return
	}

	watcher.mutex.Lock()
	if branch == watcher.currentBranch {
		watcher.mutex.Unlock()
		return
	}
	watcher.currentBranch = branch
	watcher.mutex.Unlock()

	if watcher.bus == nil {
		return
	}
	watcher.bus.Publish(Event{
		Type:      EventTypeGitBranchChanged,
		Path:      branch,
		Timestamp: time.Now().UTC(),
	})
}
