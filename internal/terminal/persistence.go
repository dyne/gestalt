package terminal

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	sessionLogFlushInterval  = time.Second
	sessionLogFlushThreshold = 4 * 1024
	sessionLogChannelSize    = 256
)

type SessionLogger struct {
	logger *asyncFileLogger[[]byte]
}

func NewSessionLogger(dir, terminalID string, createdAt time.Time) (*SessionLogger, error) {
	if dir == "" {
		return nil, fmt.Errorf("session log dir is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create session log dir: %w", err)
	}

	timestamp := createdAt.UTC().Format("20060102-150405")
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.txt", terminalID, timestamp))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open session log file: %w", err)
	}

	logger := newAsyncFileLogger(path, file, sessionLogFlushInterval, sessionLogFlushThreshold, sessionLogChannelSize, asyncFileLoggerBlock, encodeSessionChunk)
	return &SessionLogger{logger: logger}, nil
}

func (l *SessionLogger) Write(chunk []byte) {
	if l == nil || l.logger == nil {
		return
	}
	l.logger.Write(chunk)
}

func (l *SessionLogger) Path() string {
	if l == nil || l.logger == nil {
		return ""
	}
	return l.logger.Path()
}

func (l *SessionLogger) DroppedChunks() uint64 {
	if l == nil || l.logger == nil {
		return 0
	}
	return l.logger.Dropped()
}

func (l *SessionLogger) BlockedWrites() uint64 {
	if l == nil || l.logger == nil {
		return 0
	}
	return l.logger.Blocked()
}

func (l *SessionLogger) LastFlushDuration() time.Duration {
	if l == nil || l.logger == nil {
		return 0
	}
	return l.logger.LastFlushDuration()
}

func (l *SessionLogger) LastBlockedDuration() time.Duration {
	if l == nil || l.logger == nil {
		return 0
	}
	return l.logger.LastBlockedDuration()
}

func (l *SessionLogger) Close() error {
	if l == nil || l.logger == nil {
		return nil
	}
	return l.logger.Close()
}

func encodeSessionChunk(chunk []byte) ([]byte, error) {
	if len(chunk) == 0 {
		return nil, nil
	}
	return chunk, nil
}
