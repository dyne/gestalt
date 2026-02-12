package terminal

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// Runner abstracts session IO for PTY-backed and external sessions.
type Runner interface {
	Write([]byte) error
	Resize(cols, rows uint16) error
	Close() error
}

var ErrRunnerAttached = errors.New("runner already attached")

type ptyRunner struct {
	pty       Pty
	ctx       context.Context
	input     chan []byte
	closed    uint32
	closeOnce sync.Once
	onError   func(error)
}

func newPtyRunner(ctx context.Context, pty Pty, onError func(error)) *ptyRunner {
	runner := &ptyRunner{
		pty:     pty,
		ctx:     ctx,
		input:   make(chan []byte, 64),
		onError: onError,
	}
	go runner.writeLoop()
	return runner
}

func (r *ptyRunner) Write(data []byte) (err error) {
	if r == nil || r.pty == nil {
		return ErrRunnerUnavailable
	}
	if atomic.LoadUint32(&r.closed) == 1 {
		return ErrSessionClosed
	}

	defer func() {
		if recover() != nil {
			err = ErrSessionClosed
		}
	}()

	select {
	case r.input <- data:
		return nil
	case <-r.ctx.Done():
		return ErrSessionClosed
	}
}

func (r *ptyRunner) Resize(cols, rows uint16) error {
	if r == nil || r.pty == nil {
		return ErrRunnerUnavailable
	}
	return r.pty.Resize(cols, rows)
}

func (r *ptyRunner) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		atomic.StoreUint32(&r.closed, 1)
		close(r.input)
	})
	return nil
}

func (r *ptyRunner) writeLoop() {
	for data := range r.input {
		if _, err := r.pty.Write(data); err != nil {
			if r.onError != nil {
				r.onError(err)
			}
			return
		}
	}
}

type externalRunner struct {
	mu         sync.RWMutex
	attached   bool
	writeFunc  func([]byte) error
	resizeFunc func(uint16, uint16) error
	closeFunc  func() error
}

func newExternalRunner() *externalRunner {
	return &externalRunner{}
}

func (r *externalRunner) Attach(writeFn func([]byte) error, resizeFn func(uint16, uint16) error, closeFn func() error) error {
	if r == nil {
		return ErrRunnerUnavailable
	}
	r.mu.Lock()
	if r.attached {
		r.mu.Unlock()
		return ErrRunnerAttached
	}
	r.attached = true
	r.writeFunc = writeFn
	r.resizeFunc = resizeFn
	r.closeFunc = closeFn
	r.mu.Unlock()
	return nil
}

func (r *externalRunner) Detach() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.attached = false
	r.writeFunc = nil
	r.resizeFunc = nil
	r.closeFunc = nil
	r.mu.Unlock()
}

func (r *externalRunner) Write(data []byte) error {
	if r == nil {
		return ErrRunnerUnavailable
	}
	r.mu.RLock()
	writeFn := r.writeFunc
	r.mu.RUnlock()
	if writeFn == nil {
		return ErrRunnerUnavailable
	}
	return writeFn(data)
}

func (r *externalRunner) Resize(cols, rows uint16) error {
	if r == nil {
		return ErrRunnerUnavailable
	}
	r.mu.RLock()
	resizeFn := r.resizeFunc
	r.mu.RUnlock()
	if resizeFn == nil {
		return ErrRunnerUnavailable
	}
	return resizeFn(cols, rows)
}

func (r *externalRunner) Close() error {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	closeFn := r.closeFunc
	r.mu.RUnlock()
	if closeFn == nil {
		return nil
	}
	return closeFn()
}
