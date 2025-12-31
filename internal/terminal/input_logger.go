package terminal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	inputLogFlushInterval  = time.Second
	inputLogFlushThreshold = 4 * 1024
	inputLogChannelSize    = 256
)

type InputLogger struct {
	path      string
	file      *os.File
	writer    *bufio.Writer
	writeCh   chan InputEntry
	closeCh   chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	closed    uint32
	dropped   uint64
	lastFlush int64
	closeErr  error
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

	logger := &InputLogger{
		path:    path,
		file:    file,
		writer:  bufio.NewWriterSize(file, inputLogFlushThreshold),
		writeCh: make(chan InputEntry, inputLogChannelSize),
		closeCh: make(chan struct{}),
		done:    make(chan struct{}),
	}
	go logger.run()
	return logger, nil
}

func (l *InputLogger) Write(entry InputEntry) {
	if l == nil {
		return
	}
	entry.Command = strings.TrimSpace(entry.Command)
	if entry.Command == "" {
		return
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	if atomic.LoadUint32(&l.closed) == 1 {
		return
	}
	select {
	case l.writeCh <- entry:
	default:
		select {
		case <-l.writeCh:
			atomic.AddUint64(&l.dropped, 1)
		default:
		}
		select {
		case l.writeCh <- entry:
		default:
			atomic.AddUint64(&l.dropped, 1)
		}
	}
}

func (l *InputLogger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *InputLogger) DroppedEntries() uint64 {
	if l == nil {
		return 0
	}
	return atomic.LoadUint64(&l.dropped)
}

func (l *InputLogger) LastFlushDuration() time.Duration {
	if l == nil {
		return 0
	}
	return time.Duration(atomic.LoadInt64(&l.lastFlush))
}

func (l *InputLogger) Close() error {
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

func (l *InputLogger) run() {
	defer close(l.done)

	ticker := time.NewTicker(inputLogFlushInterval)
	defer ticker.Stop()

	pending := 0
	flush := func(force bool) {
		if pending == 0 && !force {
			return
		}
		start := time.Now()
		if err := l.writer.Flush(); err != nil && l.closeErr == nil {
			l.closeErr = err
		}
		atomic.StoreInt64(&l.lastFlush, time.Since(start).Nanoseconds())
		pending = 0
	}

	writeEntry := func(entry InputEntry) {
		payload, err := json.Marshal(entry)
		if err != nil {
			if l.closeErr == nil {
				l.closeErr = err
			}
			return
		}
		payload = append(payload, '\n')
		n, err := l.writer.Write(payload)
		if err != nil && l.closeErr == nil {
			l.closeErr = err
		}
		if err == nil {
			pending += n
		}
		if pending >= inputLogFlushThreshold {
			flush(false)
		}
	}

	for {
		select {
		case entry := <-l.writeCh:
			writeEntry(entry)
		case <-ticker.C:
			flush(false)
		case <-l.closeCh:
			for {
				select {
				case entry := <-l.writeCh:
					writeEntry(entry)
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
