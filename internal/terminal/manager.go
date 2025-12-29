package terminal

import (
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
)

var ErrSessionNotFound = errors.New("terminal session not found")

type ManagerOptions struct {
	Shell       string
	PtyFactory  PtyFactory
	BufferLines int
}

type Manager struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	nextID      uint64
	shell       string
	factory     PtyFactory
	bufferLines int
}

func NewManager(opts ManagerOptions) *Manager {
	shell := opts.Shell
	if shell == "" {
		shell = DefaultShell()
	}

	factory := opts.PtyFactory
	if factory == nil {
		factory = DefaultPtyFactory()
	}

	bufferLines := opts.BufferLines
	if bufferLines <= 0 {
		bufferLines = DefaultBufferLines
	}

	return &Manager{
		sessions:    make(map[string]*Session),
		shell:       shell,
		factory:     factory,
		bufferLines: bufferLines,
	}
}

func (m *Manager) Create(role, title string) (*Session, error) {
	pty, cmd, err := m.factory.Start(m.shell)
	if err != nil {
		return nil, err
	}

	id := strconv.FormatUint(atomic.AddUint64(&m.nextID, 1), 10)
	session := newSession(id, pty, cmd, title, role, m.bufferLines)

	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()

	return session, nil
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	session, ok := m.sessions[id]
	m.mu.RUnlock()

	return session, ok
}

func (m *Manager) List() []SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]SessionInfo, 0, len(m.sessions))
	for _, session := range m.sessions {
		infos = append(infos, session.Info())
	}

	return infos
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	session, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	if !ok {
		return ErrSessionNotFound
	}

	return session.Close()
}
