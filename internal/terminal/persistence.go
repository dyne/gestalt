package terminal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	sessionLogFlushInterval = time.Second
	sessionLogFlushThreshold = 4 * 1024
	sessionLogChannelSize = 256
)

type SessionLogger struct {
	path      string
	file      *os.File
	writer    *bufio.Writer
	writeCh   chan []byte
	closeCh   chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	closed    uint32
	closeErr  error
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

	logger := &SessionLogger{
		path:    path,
		file:    file,
		writer:  bufio.NewWriterSize(file, sessionLogFlushThreshold),
		writeCh: make(chan []byte, sessionLogChannelSize),
		closeCh: make(chan struct{}),
		done:    make(chan struct{}),
	}
	go logger.run()
	return logger, nil
}

func (l *SessionLogger) Write(chunk []byte) {
	if l == nil || len(chunk) == 0 {
		return
	}
	if atomic.LoadUint32(&l.closed) == 1 {
		return
	}
	select {
	case l.writeCh <- chunk:
	default:
	}
}

func (l *SessionLogger) Close() error {
	if l == nil {
		return nil
	}
	l.closeOnce.Do(func() {
		atomic.StoreUint32(&l.closed, 1)
		close(l.closeCh)
		<-l.done
	})
	return l.closeErr
}

func (l *SessionLogger) run() {
	defer close(l.done)

	ticker := time.NewTicker(sessionLogFlushInterval)
	defer ticker.Stop()

	pending := 0
	flush := func(force bool) {
		if pending == 0 && !force {
			return
		}
		if err := l.writer.Flush(); err != nil && l.closeErr == nil {
			l.closeErr = err
		}
		pending = 0
	}

	for {
		select {
		case chunk := <-l.writeCh:
			if len(chunk) == 0 {
				continue
			}
			n, err := l.writer.Write(chunk)
			if err != nil && l.closeErr == nil {
				l.closeErr = err
			}
			if err == nil {
				pending += n
			}
			if pending >= sessionLogFlushThreshold {
				flush(false)
			}
		case <-ticker.C:
			flush(false)
		case <-l.closeCh:
			for {
				select {
				case chunk := <-l.writeCh:
					if len(chunk) == 0 {
						continue
					}
					n, err := l.writer.Write(chunk)
					if err != nil && l.closeErr == nil {
						l.closeErr = err
					}
					if err == nil {
						pending += n
					}
				default:
					flush(true)
					if err := l.file.Close(); err != nil && l.closeErr == nil {
						l.closeErr = err
					}
					return
				}
			}
		}
	}
}
