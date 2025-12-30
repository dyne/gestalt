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
		path := createSessionLog(t, dir, "1", fmt.Sprintf("recent-%d", i), now.AddDate(0, 0, -i))
		recentPaths = append(recentPaths, path)
	}

	oldPaths := []string{
		createSessionLog(t, dir, "1", "old-1", now.AddDate(0, 0, -10)),
		createSessionLog(t, dir, "1", "old-2", now.AddDate(0, 0, -11)),
	}

	otherTerminal := createSessionLog(t, dir, "2", "old-1", now.AddDate(0, 0, -20))

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

func createSessionLog(t *testing.T, dir, terminalID, suffix string, modTime time.Time) string {
	t.Helper()
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.txt", terminalID, suffix))
	if err := os.WriteFile(path, []byte("log\n"), 0o644); err != nil {
		t.Fatalf("write session log: %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("set session log time: %v", err)
	}
	return path
}
