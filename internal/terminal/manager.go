package terminal

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/logging"
)

var ErrSessionNotFound = errors.New("terminal session not found")
var ErrAgentNotFound = errors.New("agent profile not found")

type ManagerOptions struct {
	Shell         string
	PtyFactory    PtyFactory
	BufferLines   int
	Clock         Clock
	Agents        map[string]agent.Agent
	Logger        *logging.Logger
	SessionLogDir string
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
	logger      *logging.Logger
	sessionLogs string
}

const (
	promptDelay      = 3 * time.Second
	interPromptDelay = 100 * time.Millisecond
	finalEnterDelay  = 500 * time.Millisecond
	promptChunkDelay = 25 * time.Millisecond
	promptChunkSize  = 64
	enterKeyDelay    = 75 * time.Millisecond
)

var onAirTimeout = 5 * time.Second

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

	logger := opts.Logger
	if logger == nil {
		logger = logging.NewLoggerWithOutput(logging.NewLogBuffer(logging.DefaultBufferSize), logging.LevelInfo, nil)
	}

	sessionLogs := strings.TrimSpace(opts.SessionLogDir)

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
		logger:      logger,
		sessionLogs: sessionLogs,
	}
}

func (m *Manager) Create(agentID, role, title string) (*Session, error) {
	shell := m.shell
	var profile *agent.Agent
	var promptNames []string
	var onAirString string
	if agentID != "" {
		agentProfile, ok := m.GetAgent(agentID)
		if !ok {
			m.logger.Warn("agent not found", map[string]string{
				"agent_id": agentID,
			})
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
		if len(agentProfile.Prompts) > 0 {
			promptNames = append(promptNames, agentProfile.Prompts...)
		}
		if strings.TrimSpace(agentProfile.OnAirString) != "" {
			onAirString = agentProfile.OnAirString
		}
	}

	command, args, err := splitCommandLine(shell)
	if err != nil {
		m.logger.Warn("shell command parse failed", map[string]string{
			"shell": shell,
			"error": err.Error(),
		})
		return nil, err
	}

	pty, cmd, err := m.factory.Start(command, args...)
	if err != nil {
		return nil, err
	}

	id := m.nextIDValue()
	createdAt := m.clock.Now().UTC()
	var sessionLogger *SessionLogger
	if m.sessionLogs != "" {
		logger, err := NewSessionLogger(m.sessionLogs, id, createdAt)
		if err != nil {
			m.logger.Warn("session log create failed", map[string]string{
				"terminal_id": id,
				"error":       err.Error(),
				"path":        m.sessionLogs,
			})
		} else {
			sessionLogger = logger
		}
	}
	session := newSession(id, pty, cmd, title, role, createdAt, m.bufferLines, profile, sessionLogger)

	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()

	fields := map[string]string{
		"terminal_id": id,
		"role":        role,
		"title":       title,
	}
	if agentID != "" {
		fields["agent_id"] = agentID
	}
	m.logger.Info("terminal created", fields)

	if len(promptNames) > 0 {
		go func() {
			if strings.TrimSpace(onAirString) != "" {
				if !waitForOnAir(session, onAirString, onAirTimeout) {
					m.logger.Error("agent onair string not found", map[string]string{
						"agent_id":     agentID,
						"onair_string": onAirString,
						"timeout_ms":   strconv.FormatInt(onAirTimeout.Milliseconds(), 10),
					})
				}
			} else {
				time.Sleep(promptDelay)
			}
			cleaned := make([]string, 0, len(promptNames))
			for _, promptName := range promptNames {
				promptName = strings.TrimSpace(promptName)
				if promptName != "" {
					cleaned = append(cleaned, promptName)
				}
			}
			wrotePrompt := false
			for i, promptName := range cleaned {
				promptPath := filepath.Join("config", "prompts", promptName+".txt")
				data, err := os.ReadFile(promptPath)
				if err != nil {
					m.logger.Warn("agent prompt file read failed", map[string]string{
						"agent_id":    agentID,
						"prompt":      promptName,
						"prompt_path": promptPath,
						"error":       err.Error(),
					})
					continue
				}
				payload := data
				for offset := 0; offset < len(payload); offset += promptChunkSize {
					end := offset + promptChunkSize
					if end > len(payload) {
						end = len(payload)
					}
					if err := session.Write(payload[offset:end]); err != nil {
						m.logger.Warn("agent prompt write failed", map[string]string{
							"agent_id": agentID,
							"prompt":   promptName,
							"error":    err.Error(),
						})
						return
					}
					if end < len(payload) {
						time.Sleep(promptChunkDelay)
					}
				}
				wrotePrompt = true
				if i < len(cleaned)-1 {
					time.Sleep(interPromptDelay)
				}
			}
			if wrotePrompt {
				time.Sleep(finalEnterDelay)
				if err := session.Write([]byte("\r")); err != nil {
					m.logger.Warn("agent prompt final enter failed", map[string]string{
						"agent_id": agentID,
						"error":    err.Error(),
					})
					return
				}
				time.Sleep(enterKeyDelay)
				if err := session.Write([]byte("\n")); err != nil {
					m.logger.Warn("agent prompt final enter failed", map[string]string{
						"agent_id": agentID,
						"error":    err.Error(),
					})
				}
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
		m.logger.Warn("terminal close error", map[string]string{
			"terminal_id": id,
			"error":       err.Error(),
		})
	}
	m.logger.Info("terminal deleted", map[string]string{
		"terminal_id": id,
	})
	return nil
}

func waitForOnAir(session *Session, target string, timeout time.Duration) bool {
	if session == nil {
		return false
	}
	if strings.TrimSpace(target) == "" {
		return true
	}
	output, cancel := session.Subscribe()
	defer cancel()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	buffer := ""
	for {
		select {
		case chunk, ok := <-output:
			if !ok {
				return false
			}
			text := strings.ReplaceAll(string(chunk), "\r\n", "\n")
			text = strings.ReplaceAll(text, "\r", "\n")
			buffer += text
			for {
				idx := strings.IndexByte(buffer, '\n')
				if idx < 0 {
					break
				}
				line := buffer[:idx]
				buffer = buffer[idx+1:]
				if strings.EqualFold(line, target) {
					return true
				}
			}
			if strings.EqualFold(buffer, target) {
				return true
			}
		case <-timer.C:
			return false
		}
	}
}
