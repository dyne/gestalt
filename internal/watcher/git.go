package watcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// GitWatcher monitors HEAD changes and emits git branch change events.
type GitWatcher struct {
	hub           *EventHub
	headPath      string
	currentBranch string
	subscription  string
	mutex         sync.Mutex
}

// StartGitWatcher creates a GitWatcher if the working directory is a git repo.
func StartGitWatcher(hub *EventHub, workDir string) (*GitWatcher, error) {
	if hub == nil {
		return nil, errors.New("event hub is nil")
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
		hub:           hub,
		headPath:      headPath,
		currentBranch: readGitBranch(headPath),
	}

	if err := hub.WatchFile(headPath); err != nil {
		return nil, err
	}

	watcher.subscription = hub.Subscribe(EventTypeFileChanged, watcher.handleFileChanged)
	return watcher, nil
}

// Close stops watching git branch changes.
func (watcher *GitWatcher) Close() {
	if watcher == nil || watcher.hub == nil {
		return
	}
	if watcher.subscription != "" {
		watcher.hub.Unsubscribe(watcher.subscription)
		watcher.subscription = ""
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

	watcher.hub.Publish(Event{
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
