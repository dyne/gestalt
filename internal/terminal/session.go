package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

var ErrSessionClosed = errors.New("terminal session closed")

type SessionState uint32

const (
	sessionStateStarting SessionState = iota
	sessionStateRunning
	sessionStateClosing
	sessionStateClosed
)

func (s SessionState) String() string {
	switch s {
	case sessionStateStarting:
		return "starting"
	case sessionStateClosing:
		return "closing"
	case sessionStateClosed:
		return "closed"
	default:
		return "running"
	}
}

type Session struct {
	ID        string
	Title     string
	Role      string
	CreatedAt time.Time

	ctx    context.Context
	cancel context.CancelFunc

	input  chan []byte
	output chan []byte

	pty      Pty
	cmd      *exec.Cmd
	bcast    *Broadcaster
	closing  sync.Once
	closeErr error
	state    uint32
}

type SessionInfo struct {
	ID        string
	Title     string
	Role      string
	CreatedAt time.Time
	Status    string
}

func newSession(id string, pty Pty, cmd *exec.Cmd, title, role string, createdAt time.Time, bufferLines int) *Session {
	// readLoop -> output, writeLoop -> PTY, broadcastLoop -> subscribers.
	// Close cancels context and closes input so loops drain and exit cleanly.
	ctx, cancel := context.WithCancel(context.Background())
	session := &Session{
		ID:        id,
		Title:     title,
		Role:      role,
		CreatedAt: createdAt,
		ctx:       ctx,
		cancel:    cancel,
		input:     make(chan []byte, 64),
		output:    make(chan []byte, 64),
		pty:       pty,
		cmd:       cmd,
		bcast:     NewBroadcaster(bufferLines),
		state:     uint32(sessionStateStarting),
	}

	go session.readLoop()
	go session.writeLoop()
	go session.broadcastLoop()
	session.setState(sessionStateRunning)

	return session
}

func (s *Session) Info() SessionInfo {
	return SessionInfo{
		ID:        s.ID,
		Title:     s.Title,
		Role:      s.Role,
		CreatedAt: s.CreatedAt,
		Status:    s.State().String(),
	}
}

func (s *Session) Subscribe() (<-chan []byte, func()) {
	return s.bcast.Subscribe()
}

func (s *Session) Write(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	select {
	case s.input <- data:
		return nil
	case <-s.ctx.Done():
		return ErrSessionClosed
	}
}

func (s *Session) Resize(cols, rows uint16) error {
	if s.pty == nil {
		return ErrSessionClosed
	}

	if err := s.pty.Resize(cols, rows); err != nil {
		return fmt.Errorf("resize pty: %w", err)
	}
	return nil
}

func (s *Session) OutputLines() []string {
	return s.bcast.OutputLines()
}

func (s *Session) Close() error {
	s.closing.Do(func() {
		s.setState(sessionStateClosing)
		if s.cancel != nil {
			s.cancel()
		}
		close(s.input)
		s.closeErr = s.closeResources()
		s.setState(sessionStateClosed)
	})

	return s.closeErr
}

func (s *Session) State() SessionState {
	return SessionState(atomic.LoadUint32(&s.state))
}

func (s *Session) setState(state SessionState) {
	atomic.StoreUint32(&s.state, uint32(state))
}

func (s *Session) closeResources() error {
	var errs []error
	if s.pty != nil {
		if err := s.pty.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			errs = append(errs, fmt.Errorf("close pty: %w", err))
		}
	}
	if s.cmd != nil && s.cmd.Process != nil {
		killErr := s.cmd.Process.Kill()
		if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			errs = append(errs, fmt.Errorf("kill process: %w", killErr))
		}
		if killErr == nil || errors.Is(killErr, os.ErrProcessDone) {
			if s.cmd.ProcessState == nil {
				if err := s.cmd.Wait(); err != nil && !errors.Is(err, os.ErrProcessDone) {
					errs = append(errs, fmt.Errorf("wait process: %w", err))
				}
			}
		}
	}
	return errors.Join(errs...)
}

func (s *Session) readLoop() {
	defer close(s.output)

	buf := make([]byte, 4096)
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		n, err := s.pty.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			select {
			case s.output <- chunk:
			case <-s.ctx.Done():
				return
			}
		}
		if err != nil {
			_ = s.Close()
			return
		}
	}
}

func (s *Session) writeLoop() {
	for data := range s.input {
		if _, err := s.pty.Write(data); err != nil {
			_ = s.Close()
			return
		}
	}
}

func (s *Session) broadcastLoop() {
	for chunk := range s.output {
		s.bcast.Broadcast(chunk)
	}
	s.bcast.Close()
}
