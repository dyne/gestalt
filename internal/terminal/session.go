package terminal

import (
	"errors"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

var ErrSessionClosed = errors.New("terminal session closed")

type Session struct {
	ID        string
	Title     string
	Role      string
	CreatedAt time.Time

	input  chan []byte
	output chan []byte

	pty     Pty
	cmd     *exec.Cmd
	buffer  *OutputBuffer
	closed  chan struct{}
	closing sync.Once

	subMu       sync.Mutex
	subscribers map[uint64]chan []byte
	nextSubID   uint64
}

type SessionInfo struct {
	ID        string
	Title     string
	Role      string
	CreatedAt time.Time
	Status    string
}

func newSession(id string, pty Pty, cmd *exec.Cmd, title, role string, bufferLines int) *Session {
	session := &Session{
		ID:          id,
		Title:       title,
		Role:        role,
		CreatedAt:   time.Now().UTC(),
		input:       make(chan []byte, 64),
		output:      make(chan []byte, 64),
		pty:         pty,
		cmd:         cmd,
		buffer:      NewOutputBuffer(bufferLines),
		closed:      make(chan struct{}),
		subscribers: make(map[uint64]chan []byte),
	}

	go session.readLoop()
	go session.writeLoop()
	go session.broadcastLoop()

	return session
}

func (s *Session) Info() SessionInfo {
	return SessionInfo{
		ID:        s.ID,
		Title:     s.Title,
		Role:      s.Role,
		CreatedAt: s.CreatedAt,
		Status:    "running",
	}
}

func (s *Session) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 128)
	id := atomic.AddUint64(&s.nextSubID, 1)

	s.subMu.Lock()
	s.subscribers[id] = ch
	s.subMu.Unlock()

	cancel := func() {
		s.subMu.Lock()
		if existing, ok := s.subscribers[id]; ok {
			delete(s.subscribers, id)
			close(existing)
		}
		s.subMu.Unlock()
	}

	return ch, cancel
}

func (s *Session) Write(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	select {
	case s.input <- data:
		return nil
	case <-s.closed:
		return ErrSessionClosed
	}
}

func (s *Session) Resize(cols, rows uint16) error {
	if s.pty == nil {
		return ErrSessionClosed
	}

	return s.pty.Resize(cols, rows)
}

func (s *Session) OutputLines() []string {
	return s.buffer.Lines()
}

func (s *Session) Close() error {
	s.closing.Do(func() {
		close(s.closed)
		close(s.input)
		if s.pty != nil {
			_ = s.pty.Close()
		}
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
	})

	return nil
}

func (s *Session) readLoop() {
	defer close(s.output)

	buf := make([]byte, 4096)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			select {
			case s.output <- chunk:
			case <-s.closed:
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
		s.buffer.Append(chunk)
		s.subMu.Lock()
		for _, ch := range s.subscribers {
			select {
			case ch <- chunk:
			default:
			}
		}
		s.subMu.Unlock()
	}

	s.subMu.Lock()
	for id, ch := range s.subscribers {
		delete(s.subscribers, id)
		close(ch)
	}
	s.subMu.Unlock()
}
