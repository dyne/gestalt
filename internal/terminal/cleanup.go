package terminal

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	DefaultSessionRetentionDays  = 7
	DefaultSessionRetentionCount = 5
	sessionCleanupInterval       = time.Hour
	sessionLogTimestampLayout    = "20060102-150405"
)

type sessionLogFile struct {
	path       string
	terminalID string
	modTime    time.Time
}

func (m *Manager) startSessionCleanup() {
	if m == nil || m.sessionLogs == "" || m.retentionDays <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(sessionCleanupInterval)
		defer ticker.Stop()
		m.cleanupSessionLogs(m.clock.Now())
		for range ticker.C {
			m.cleanupSessionLogs(m.clock.Now())
		}
	}()
}

func (m *Manager) cleanupSessionLogs(now time.Time) {
	if m == nil || m.sessionLogs == "" || m.retentionDays <= 0 {
		return
	}

	entries, err := os.ReadDir(m.sessionLogs)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		m.logger.Warn("session log cleanup failed", map[string]string{
			"path":  m.sessionLogs,
			"error": err.Error(),
		})
		return
	}

	threshold := now.AddDate(0, 0, -m.retentionDays)
	groups := make(map[string][]sessionLogFile)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		terminalID, ok := sessionLogTerminalID(name)
		if !ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		groups[terminalID] = append(groups[terminalID], sessionLogFile{
			path:       filepath.Join(m.sessionLogs, name),
			terminalID: terminalID,
			modTime:    info.ModTime(),
		})
	}

	for _, files := range groups {
		sort.Slice(files, func(i, j int) bool {
			return files[i].modTime.After(files[j].modTime)
		})
		keep := make(map[string]bool, len(files))
		for i, file := range files {
			if file.modTime.After(threshold) || file.modTime.Equal(threshold) {
				keep[file.path] = true
			}
			if i < DefaultSessionRetentionCount {
				keep[file.path] = true
			}
		}
		for _, file := range files {
			if keep[file.path] {
				continue
			}
			if err := os.Remove(file.path); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				m.logger.Warn("session log cleanup remove failed", map[string]string{
					"path":  file.path,
					"error": err.Error(),
				})
				continue
			}
			m.logger.Info("session log removed", map[string]string{
				"terminal_id": file.terminalID,
				"path":        file.path,
			})
		}
	}
}

func sessionLogTerminalID(filename string) (string, bool) {
	if !strings.HasSuffix(filename, ".txt") {
		return "", false
	}
	trimmed := strings.TrimSuffix(filename, ".txt")
	if len(trimmed) <= len(sessionLogTimestampLayout) {
		return "", false
	}
	timestampStart := len(trimmed) - len(sessionLogTimestampLayout)
	if timestampStart <= 0 {
		return "", false
	}
	if trimmed[timestampStart-1] != '-' {
		return "", false
	}
	timestamp := trimmed[timestampStart:]
	if _, err := time.Parse(sessionLogTimestampLayout, timestamp); err != nil {
		return "", false
	}
	terminalID := strings.TrimSpace(trimmed[:timestampStart-1])
	if terminalID == "" {
		return "", false
	}
	return terminalID, true
}
