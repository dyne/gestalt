package watcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gestalt/internal/event"
)

// GitWatcher monitors HEAD changes and emits git branch change events.
type GitWatcher struct {
	bus           *event.Bus[Event]
	headPath      string
	currentBranch string
	watchHandle   Handle
	cancel        func()
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

	gitDir := resolveGitDir(workDir)
	if gitDir == "" {
		return nil, nil
	}

	headPath := filepath.Join(gitDir, "HEAD")
	watcher := &GitWatcher{
		bus:           bus,
		headPath:      headPath,
		currentBranch: readGitBranch(headPath),
	}

	handle, err := WatchFile(bus, watch, headPath)
	if err != nil {
		return nil, err
	}
	watcher.watchHandle = handle

	events, cancel := bus.SubscribeFiltered(func(event Event) bool {
		return event.Type == EventTypeFileChanged && event.Path == headPath
	})
	watcher.cancel = cancel
	go watcher.consume(events)
	return watcher, nil
}

// Close stops watching git branch changes.
func (watcher *GitWatcher) Close() {
	if watcher == nil {
		return
	}
	if watcher.cancel != nil {
		watcher.cancel()
		watcher.cancel = nil
	}
	if watcher.watchHandle != nil {
		_ = watcher.watchHandle.Close()
		watcher.watchHandle = nil
	}
}

func (watcher *GitWatcher) consume(events <-chan Event) {
	for event := range events {
		watcher.handleFileChanged(event)
	}
}

func (watcher *GitWatcher) handleFileChanged(event Event) {
	if event.Path != watcher.headPath {
		return
	}
	branch := readGitBranch(watcher.headPath)
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

func resolveGitDir(workDir string) string {
	gitPath := filepath.Join(workDir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return gitPath
	}
	if !info.Mode().IsRegular() {
		return ""
	}
	contents, err := os.ReadFile(gitPath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(contents))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return ""
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if gitDir == "" {
		return ""
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(workDir, gitDir)
	}
	return gitDir
}

func readGitBranch(headPath string) string {
	contents, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(contents))
	if line == "" {
		return ""
	}
	const prefix = "ref: "
	if strings.HasPrefix(line, prefix) {
		ref := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	short := line
	if len(short) > 12 {
		short = short[:12]
	}
	return fmt.Sprintf("detached@%s", short)
}
