package terminal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	sessionLogFlushInterval  = time.Second
	sessionLogFlushThreshold = 4 * 1024
	sessionLogChannelSize    = 256
)

type SessionLogger struct {
	logger       *asyncFileLogger[[]byte]
	maxBytes     int64
	bytesWritten int64
}

func NewSessionLogger(dir, terminalID string, createdAt time.Time, maxBytes int64) (*SessionLogger, error) {
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
	return &SessionLogger{
		logger:   logger,
		maxBytes: maxBytes,
	}, nil
}

func newRawSessionLogger(path string, maxBytes int64) (*SessionLogger, error) {
	if path == "" {
		return nil, fmt.Errorf("session log path is empty")
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".txt")
	rawPath := filepath.Join(dir, base+".raw.txt")
	file, err := os.OpenFile(rawPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open raw session log file: %w", err)
	}
	logger := newAsyncFileLogger(rawPath, file, sessionLogFlushInterval, sessionLogFlushThreshold, sessionLogChannelSize, asyncFileLoggerBlock, encodeSessionChunk)
	return &SessionLogger{
		logger:   logger,
		maxBytes: maxBytes,
	}, nil
}

func (l *SessionLogger) Write(chunk []byte) {
	if l == nil || l.logger == nil {
		return
	}
	if len(chunk) == 0 {
		return
	}
	if l.maxBytes > 0 {
		remaining := l.maxBytes - l.bytesWritten
		if remaining <= 0 {
			return
		}
		if int64(len(chunk)) > remaining {
			chunk = chunk[:remaining]
		}
	}
	l.bytesWritten += int64(len(chunk))
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
