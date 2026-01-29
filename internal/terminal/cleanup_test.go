package terminal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupSessionLogsKeepsRecentAndLatest(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)

	recentPaths := make([]string, 0, 6)
	for i := 1; i <= 6; i++ {
		path := createSessionLog(t, dir, "Coder (Codex) 1", now.AddDate(0, 0, -i))
		recentPaths = append(recentPaths, path)
	}

	oldPaths := []string{
		createSessionLog(t, dir, "Coder (Codex) 1", now.AddDate(0, 0, -10)),
		createSessionLog(t, dir, "Coder (Codex) 1", now.AddDate(0, 0, -11)),
	}

	otherTerminal := createSessionLog(t, dir, "Release-Train-Codex 2", now.AddDate(0, 0, -20))

	manager := NewManager(ManagerOptions{Shell: "/bin/sh"})
	manager.sessionLogs = dir
	manager.retentionDays = 7

	manager.cleanupSessionLogs(now)

	for _, path := range recentPaths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected recent log to remain: %v", err)
		}
	}
	for _, path := range oldPaths {
		if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
			t.Fatalf("expected old log to be removed: %v", err)
		}
	}
	if _, err := os.Stat(otherTerminal); err != nil {
		t.Fatalf("expected other terminal log to remain: %v", err)
	}
}

func createSessionLog(t *testing.T, dir, terminalID string, timestamp time.Time) string {
	t.Helper()
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.txt", terminalID, timestamp.UTC().Format("20060102-150405")))
	if err := os.WriteFile(path, []byte("log\n"), 0o644); err != nil {
		t.Fatalf("write session log: %v", err)
	}
	if err := os.Chtimes(path, timestamp, timestamp); err != nil {
		t.Fatalf("set session log time: %v", err)
	}
	return path
}
