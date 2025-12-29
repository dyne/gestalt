package terminal

import (
	"errors"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gestalt/internal/agent"
)

var ErrSessionNotFound = errors.New("terminal session not found")
var ErrAgentNotFound = errors.New("agent profile not found")

type ManagerOptions struct {
	Shell       string
	PtyFactory  PtyFactory
	BufferLines int
	Clock       Clock
	Agents      map[string]agent.Agent
}

// Manager is safe for concurrent use; mu guards the sessions map and lifecycle.
// ID generation uses an atomic counter and does not require the mutex.
type Manager struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	nextID      uint64
	shell       string
	factory     PtyFactory
	bufferLines int
	clock       Clock
	agents      map[string]agent.Agent
}

const promptDelay = 75 * time.Millisecond

type AgentInfo struct {
	ID       string
	Name     string
	LLMType  string
	LLMModel string
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

	clock := opts.Clock
	if clock == nil {
		clock = realClock{}
	}

	agents := make(map[string]agent.Agent)
	for id, profile := range opts.Agents {
		agents[id] = profile
	}

	return &Manager{
		sessions:    make(map[string]*Session),
		shell:       shell,
		factory:     factory,
		bufferLines: bufferLines,
		clock:       clock,
		agents:      agents,
	}
}

func (m *Manager) Create(agentID, role, title string) (*Session, error) {
	shell := m.shell
	var profile *agent.Agent
	var prompt []byte
	if agentID != "" {
		agentProfile, ok := m.GetAgent(agentID)
		if !ok {
			return nil, ErrAgentNotFound
		}
		profileCopy := agentProfile
		profile = &profileCopy
		if strings.TrimSpace(agentProfile.Shell) != "" {
			shell = agentProfile.Shell
		}
		if strings.TrimSpace(agentProfile.Name) != "" {
			title = agentProfile.Name
		}
		if strings.TrimSpace(agentProfile.PromptFile) != "" {
			data, err := os.ReadFile(agentProfile.PromptFile)
			if err != nil {
				log.Printf("agent %s prompt file %s: %v", agentID, agentProfile.PromptFile, err)
			} else {
				prompt = ensureTrailingNewline(data)
			}
		}
	}

	pty, cmd, err := m.factory.Start(shell)
	if err != nil {
		return nil, err
	}

	id := m.nextIDValue()
	createdAt := m.clock.Now().UTC()
	session := newSession(id, pty, cmd, title, role, createdAt, m.bufferLines, profile)

	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()

	if len(prompt) > 0 {
		go func() {
			time.Sleep(promptDelay)
			if err := session.Write(prompt); err != nil {
				log.Printf("agent %s prompt write: %v", agentID, err)
			}
		}()
	}

	return session, nil
}

func (m *Manager) nextIDValue() string {
	return strconv.FormatUint(atomic.AddUint64(&m.nextID, 1), 10)
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

func (m *Manager) GetAgent(id string) (agent.Agent, bool) {
	m.mu.RLock()
	profile, ok := m.agents[id]
	m.mu.RUnlock()

	return profile, ok
}

func (m *Manager) ListAgents() []AgentInfo {
	m.mu.RLock()
	infos := make([]AgentInfo, 0, len(m.agents))
	for id, profile := range m.agents {
		infos = append(infos, AgentInfo{
			ID:       id,
			Name:     profile.Name,
			LLMType:  profile.LLMType,
			LLMModel: profile.LLMModel,
		})
	}
	m.mu.RUnlock()

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].ID < infos[j].ID
	})
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

	if err := session.Close(); err != nil {
		log.Printf("close session %s: %v", id, err)
	}
	return nil
}

func ensureTrailingNewline(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	if data[len(data)-1] == '\n' {
		return data
	}
	out := make([]byte, len(data)+1)
	copy(out, data)
	out[len(out)-1] = '\n'
	return out
}
