package terminal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	mcpEventLogFlushInterval  = time.Second
	mcpEventLogFlushThreshold = 4 * 1024
	mcpEventLogChannelSize    = 256
)

type mcpEventLogger struct {
	logger *asyncFileLogger[string]
}

func newMCPEventLogger(dir, sessionID string, createdAt time.Time) (*mcpEventLogger, error) {
	if dir == "" {
		return nil, fmt.Errorf("session log dir is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create session log dir: %w", err)
	}
	timestamp := createdAt.UTC().Format("20060102-150405")
	path := filepath.Join(dir, fmt.Sprintf("Events-%s-%s", sessionID, timestamp))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open codex events log: %w", err)
	}
	logger := newAsyncFileLogger(path, file, mcpEventLogFlushInterval, mcpEventLogFlushThreshold, mcpEventLogChannelSize, asyncFileLoggerBlock, encodeMCPEventLine)
	return &mcpEventLogger{
		logger: logger,
	}, nil
}

func (l *mcpEventLogger) Write(line string) {
	if l == nil || l.logger == nil {
		return
	}
	if strings.TrimSpace(line) == "" {
		return
	}
	l.logger.Write(line)
}

func (l *mcpEventLogger) Path() string {
	if l == nil || l.logger == nil {
		return ""
	}
	return l.logger.Path()
}

func (l *mcpEventLogger) Close() error {
	if l == nil || l.logger == nil {
		return nil
	}
	return l.logger.Close()
}

func encodeMCPEventLine(line string) ([]byte, error) {
	if strings.TrimSpace(line) == "" {
		return nil, nil
	}
	if strings.HasSuffix(line, "\n") {
		return []byte(line), nil
	}
	return []byte(line + "\n"), nil
}
