package terminal

import (
	"bufio"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type asyncFileLogger[T any] struct {
	path           string
	file           *os.File
	writer         *bufio.Writer
	writeCh        chan T
	closeCh        chan struct{}
	done           chan struct{}
	closeOnce      sync.Once
	closed         uint32
	dropped        uint64
	blocked        uint64
	lastFlush      int64
	lastBlocked    int64
	closeErr       error
	flushInterval  time.Duration
	flushThreshold int
	policy         asyncFileLoggerPolicy
	encoder        func(T) ([]byte, error)
}

type asyncFileLoggerPolicy uint8

const (
	asyncFileLoggerDropOldest asyncFileLoggerPolicy = iota
	asyncFileLoggerBlock
)

func newAsyncFileLogger[T any](path string, file *os.File, flushInterval time.Duration, flushThreshold int, channelSize int, policy asyncFileLoggerPolicy, encoder func(T) ([]byte, error)) *asyncFileLogger[T] {
	logger := &asyncFileLogger[T]{
		path:           path,
		file:           file,
		writer:         bufio.NewWriterSize(file, flushThreshold),
		writeCh:        make(chan T, channelSize),
		closeCh:        make(chan struct{}),
		done:           make(chan struct{}),
		flushInterval:  flushInterval,
		flushThreshold: flushThreshold,
		policy:         policy,
		encoder:        encoder,
	}
	go logger.run()
	return logger
}

func (l *asyncFileLogger[T]) Write(item T) {
	if l == nil {
		return
	}
	if atomic.LoadUint32(&l.closed) == 1 {
		return
	}
	if l.policy == asyncFileLoggerBlock {
		select {
		case l.writeCh <- item:
			return
		default:
		}
		atomic.AddUint64(&l.blocked, 1)
		start := time.Now()
		select {
		case l.writeCh <- item:
			atomic.StoreInt64(&l.lastBlocked, time.Since(start).Nanoseconds())
		case <-l.closeCh:
		}
		return
	}

	select {
	case l.writeCh <- item:
	default:
		select {
		case <-l.writeCh:
			atomic.AddUint64(&l.dropped, 1)
		default:
		}
		select {
		case l.writeCh <- item:
		default:
			atomic.AddUint64(&l.dropped, 1)
		}
	}
}

func (l *asyncFileLogger[T]) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *asyncFileLogger[T]) Dropped() uint64 {
	if l == nil {
		return 0
	}
	return atomic.LoadUint64(&l.dropped)
}

func (l *asyncFileLogger[T]) Blocked() uint64 {
	if l == nil {
		return 0
	}
	return atomic.LoadUint64(&l.blocked)
}

func (l *asyncFileLogger[T]) LastFlushDuration() time.Duration {
	if l == nil {
		return 0
	}
	return time.Duration(atomic.LoadInt64(&l.lastFlush))
}

func (l *asyncFileLogger[T]) LastBlockedDuration() time.Duration {
	if l == nil {
		return 0
	}
	return time.Duration(atomic.LoadInt64(&l.lastBlocked))
}

func (l *asyncFileLogger[T]) Close() error {
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

func (l *asyncFileLogger[T]) run() {
	defer close(l.done)

	ticker := time.NewTicker(l.flushInterval)
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

	writePayload := func(item T) {
		payload, err := l.encoder(item)
		if err != nil {
			if l.closeErr == nil {
				l.closeErr = err
			}
			return
		}
		if len(payload) == 0 {
			return
		}
		n, err := l.writer.Write(payload)
		if err != nil && l.closeErr == nil {
			l.closeErr = err
		}
		if err == nil {
			pending += n
		}
		if pending >= l.flushThreshold {
			flush(false)
		}
	}

	for {
		select {
		case item := <-l.writeCh:
			writePayload(item)
		case <-ticker.C:
			flush(false)
		case <-l.closeCh:
			for {
				select {
				case item := <-l.writeCh:
					writePayload(item)
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
