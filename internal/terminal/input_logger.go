package terminal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	inputLogFlushInterval  = time.Second
	inputLogFlushThreshold = 4 * 1024
	inputLogChannelSize    = 256
)

type InputLogger struct {
	logger *asyncFileLogger[InputEntry]
}

func NewInputLogger(dir, name string, createdAt time.Time) (*InputLogger, error) {
	if dir == "" {
		return nil, fmt.Errorf("input log dir is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create input log dir: %w", err)
	}

	timestamp := createdAt.UTC().Format("20060102-150405")
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.jsonl", name, timestamp))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open input log file: %w", err)
	}

	logger := newAsyncFileLogger(path, file, inputLogFlushInterval, inputLogFlushThreshold, inputLogChannelSize, asyncFileLoggerDropOldest, encodeInputEntry)
	return &InputLogger{logger: logger}, nil
}

func (l *InputLogger) Write(entry InputEntry) {
	if l == nil || l.logger == nil {
		return
	}
	entry.Command = strings.TrimSpace(entry.Command)
	if entry.Command == "" {
		return
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	l.logger.Write(entry)
}

func (l *InputLogger) Path() string {
	if l == nil || l.logger == nil {
		return ""
	}
	return l.logger.Path()
}

func (l *InputLogger) DroppedEntries() uint64 {
	if l == nil || l.logger == nil {
		return 0
	}
	return l.logger.Dropped()
}

func (l *InputLogger) BlockedWrites() uint64 {
	if l == nil || l.logger == nil {
		return 0
	}
	return l.logger.Blocked()
}

func (l *InputLogger) LastFlushDuration() time.Duration {
	if l == nil || l.logger == nil {
		return 0
	}
	return l.logger.LastFlushDuration()
}

func (l *InputLogger) LastBlockedDuration() time.Duration {
	if l == nil || l.logger == nil {
		return 0
	}
	return l.logger.LastBlockedDuration()
}

func (l *InputLogger) Close() error {
	if l == nil || l.logger == nil {
		return nil
	}
	return l.logger.Close()
}

func encodeInputEntry(entry InputEntry) ([]byte, error) {
	payload, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	payload = append(payload, '\n')
	return payload, nil
}
