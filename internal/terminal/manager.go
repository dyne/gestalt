package terminal

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/prompt"
	"gestalt/internal/skill"
	"gestalt/internal/temporal"
)

var ErrSessionNotFound = errors.New("terminal session not found")
var ErrAgentNotFound = errors.New("agent profile not found")

type AgentAlreadyRunningError struct {
	AgentName  string
	TerminalID string
}

func (e *AgentAlreadyRunningError) Error() string {
	return fmt.Sprintf("agent %q already running in terminal %s", e.AgentName, e.TerminalID)
}

type ManagerOptions struct {
	Shell                string
	PtyFactory           PtyFactory
	BufferLines          int
	Clock                Clock
	Agents               map[string]agent.Agent
	AgentsDir            string
	Skills               map[string]*skill.Skill
	Logger               *logging.Logger
	TemporalClient       temporal.WorkflowClient
	TemporalEnabled      bool
	SessionLogDir        string
	InputHistoryDir      string
	SessionRetentionDays int
	PromptFS             fs.FS
	PromptDir            string
}

// Manager is safe for concurrent use; mu guards the sessions map and lifecycle.
// ID generation uses an atomic counter and does not require the mutex.
type Manager struct {
	mu              sync.RWMutex
	sessions        map[string]*Session
	agentSessions   map[string]string
	nextID          uint64
	shell           string
	factory         PtyFactory
	bufferLines     int
	clock           Clock
	agents          map[string]agent.Agent
	agentCache      *agent.AgentCache
	agentsDir       string
	skills          map[string]*skill.Skill
	logger          *logging.Logger
	agentBus        *event.Bus[event.AgentEvent]
	terminalBus     *event.Bus[event.TerminalEvent]
	workflowBus     *event.Bus[event.WorkflowEvent]
	temporalClient  temporal.WorkflowClient
	temporalEnabled bool
	sessionLogs     string
	inputHistoryDir string
	retentionDays   int
	promptFS        fs.FS
	promptDir       string
	promptParser    *prompt.Parser
}

type sessionCreateRequest struct {
	SessionID   string
	AgentID     string
	Role        string
	Title       string
	Shell       string
	UseWorkflow *bool
}

type CreateOptions struct {
	AgentID     string
	Role        string
	Title       string
	UseWorkflow *bool
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
	ID          string
	Name        string
	LLMType     string
	LLMModel    string
	UseWorkflow bool
}

type SkillMetadata struct {
	Name        string
	Description string
	Path        string
	License     string
}

func resolveWorkflowPreference(preference *bool) bool {
	if preference == nil {
		return true
	}
	return *preference
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

	temporalClient := opts.TemporalClient
	temporalEnabled := opts.TemporalEnabled
	if temporalEnabled && temporalClient == nil {
		temporalEnabled = false
		logger.Warn("temporal enabled without client", nil)
	}

	sessionLogs := strings.TrimSpace(opts.SessionLogDir)
	inputHistoryDir := strings.TrimSpace(opts.InputHistoryDir)
	retentionDays := opts.SessionRetentionDays
	if retentionDays <= 0 {
		retentionDays = DefaultSessionRetentionDays
	}

	promptFS := opts.PromptFS
	promptDir := strings.TrimSpace(opts.PromptDir)
	if promptDir == "" {
		promptDir = filepath.Join("config", "prompts")
	}
	if promptFS != nil {
		promptDir = filepath.ToSlash(promptDir)
	}
	promptParser := prompt.NewParser(promptFS, promptDir, ".")

	agentBus := event.NewBus[event.AgentEvent](context.Background(), event.BusOptions{
		Name: "agent_events",
	})
	terminalBus := event.NewBus[event.TerminalEvent](context.Background(), event.BusOptions{
		Name: "terminal_events",
	})
	workflowBus := event.NewBus[event.WorkflowEvent](context.Background(), event.BusOptions{
		Name: "workflow_events",
	})

	agents := make(map[string]agent.Agent)
	for id, profile := range opts.Agents {
		agents[id] = profile
	}
	agentsDir := strings.TrimSpace(opts.AgentsDir)
	agentCache := agent.NewAgentCache(agents)
	skills := make(map[string]*skill.Skill)
	for id, entry := range opts.Skills {
		skills[id] = entry
	}

	manager := &Manager{
		sessions:        make(map[string]*Session),
		agentSessions:   make(map[string]string),
		shell:           shell,
		factory:         factory,
		bufferLines:     bufferLines,
		clock:           clock,
		agents:          agents,
		agentCache:      agentCache,
		agentsDir:       agentsDir,
		skills:          skills,
		logger:          logger,
		agentBus:        agentBus,
		terminalBus:     terminalBus,
		workflowBus:     workflowBus,
		temporalClient:  temporalClient,
		temporalEnabled: temporalEnabled,
		sessionLogs:     sessionLogs,
		inputHistoryDir: inputHistoryDir,
		retentionDays:   retentionDays,
		promptFS:        promptFS,
		promptDir:       promptDir,
		promptParser:    promptParser,
	}
	manager.startSessionCleanup()
	return manager
}

func (m *Manager) Create(agentID, role, title string) (*Session, error) {
	return m.createSession(sessionCreateRequest{
		AgentID: agentID,
		Role:    role,
		Title:   title,
	})
}

func (m *Manager) CreateWithOptions(options CreateOptions) (*Session, error) {
	return m.createSession(sessionCreateRequest{
		AgentID:     options.AgentID,
		Role:        options.Role,
		Title:       options.Title,
		UseWorkflow: options.UseWorkflow,
	})
}

func (m *Manager) CreateWithID(sessionID, agentID, role, title, shell string) (*Session, error) {
	trimmedID := strings.TrimSpace(sessionID)
	if trimmedID == "" {
		return nil, errors.New("session id is required")
	}
	if existingSession, ok := m.Get(trimmedID); ok {
		return existingSession, nil
	}
	return m.createSession(sessionCreateRequest{
		SessionID: trimmedID,
		AgentID:   agentID,
		Role:      role,
		Title:     title,
		Shell:     shell,
	})
}

func (m *Manager) createSession(request sessionCreateRequest) (*Session, error) {
	if request.SessionID != "" {
		if existingSession, ok := m.Get(request.SessionID); ok {
			return existingSession, nil
		}
	}

	shell := m.shell
	shellOverride := strings.TrimSpace(request.Shell)
	shellOverrideSet := shellOverride != ""
	if shellOverrideSet {
		shell = shellOverride
	}

	var profile *agent.Agent
	var promptNames []string
	var onAirString string
	var agentName string
	reservedID := strings.TrimSpace(request.SessionID)
	if request.AgentID != "" {
		agentProfile, ok := m.GetAgent(request.AgentID)
		if !ok || agentProfile.Name == "" {
			m.logger.Warn("agent not found or invalid", map[string]string{
				"agent_id": request.AgentID,
			})
			return nil, ErrAgentNotFound
		}
		profileCopy := agentProfile
		profile = &profileCopy
		if !shellOverrideSet && strings.TrimSpace(agentProfile.Shell) != "" {
			shell = agentProfile.Shell
		}
		if strings.TrimSpace(agentProfile.Name) != "" {
			request.Title = agentProfile.Name
			agentName = agentProfile.Name
		}
		if len(agentProfile.Prompts) > 0 {
			promptNames = append(promptNames, agentProfile.Prompts...)
		}
		if strings.TrimSpace(agentProfile.OnAirString) != "" {
			onAirString = agentProfile.OnAirString
		}
	}

	useWorkflow := resolveWorkflowPreference(request.UseWorkflow)
	if request.UseWorkflow == nil && profile != nil {
		useWorkflow = resolveWorkflowPreference(profile.UseWorkflow)
	}

	if agentName != "" {
		if reservedID == "" {
			reservedID = m.nextIDValue()
		}
		m.mu.Lock()
		if existingID, ok := m.agentSessions[agentName]; ok {
			m.mu.Unlock()
			return nil, &AgentAlreadyRunningError{AgentName: agentName, TerminalID: existingID}
		}
		m.agentSessions[agentName] = reservedID
		m.mu.Unlock()
	}

	releaseReservation := func() {
		if agentName == "" || reservedID == "" {
			return
		}
		m.mu.Lock()
		if existingID, ok := m.agentSessions[agentName]; ok && existingID == reservedID {
			delete(m.agentSessions, agentName)
		}
		m.mu.Unlock()
	}

	command, args, err := splitCommandLine(shell)
	if err != nil {
		m.logger.Warn("shell command parse failed", map[string]string{
			"shell": shell,
			"error": err.Error(),
		})
		releaseReservation()
		return nil, err
	}

	pty, cmd, err := m.factory.Start(command, args...)
	if err != nil {
		releaseReservation()
		return nil, err
	}

	id := reservedID
	if id == "" {
		id = m.nextIDValue()
	}
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
	var inputLogger *InputLogger
	if m.inputHistoryDir != "" {
		historyName := id
		if profile != nil && strings.TrimSpace(profile.Name) != "" {
			historyName = profile.Name
		}
		logger, err := NewInputLogger(m.inputHistoryDir, historyName, createdAt)
		if err != nil {
			m.logger.Warn("input history log create failed", map[string]string{
				"terminal_id": id,
				"error":       err.Error(),
				"path":        m.inputHistoryDir,
			})
		} else {
			inputLogger = logger
		}
	}
	session := newSession(id, pty, cmd, request.Title, request.Role, createdAt, m.bufferLines, profile, sessionLogger, inputLogger)
	if request.AgentID != "" {
		session.AgentID = request.AgentID
	}

	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()

	fields := map[string]string{
		"terminal_id": id,
		"role":        request.Role,
		"title":       request.Title,
	}
	if request.AgentID != "" {
		fields["agent_id"] = request.AgentID
	}
	m.logger.Info("terminal created", fields)
	if m.terminalBus != nil {
		m.terminalBus.Publish(event.NewTerminalEvent(id, "terminal_created"))
	}
	if request.AgentID != "" && m.agentBus != nil {
		m.agentBus.Publish(event.NewAgentEvent(request.AgentID, agentName, "agent_started"))
	}

	if useWorkflow && m.temporalEnabled && m.temporalClient != nil {
		startError := session.StartWorkflow(m.temporalClient, "", "")
		if startError != nil {
			m.logger.Warn("temporal workflow start failed", map[string]string{
				"terminal_id": id,
				"error":       startError.Error(),
			})
		} else if workflowID, workflowRunID, ok := session.WorkflowIdentifiers(); ok {
			m.logger.Info("workflow started", map[string]string{
				"terminal_id": id,
				"workflow_id": workflowID,
				"run_id":      workflowRunID,
			})
		}
	}

	// Inject skill metadata and prompts if agent is configured
	if profile != nil && (len(profile.Skills) > 0 || len(promptNames) > 0) {
		go func() {
			// Wait for shell to be ready
			if strings.TrimSpace(onAirString) != "" {
				if !waitForOnAir(session, onAirString, onAirTimeout) {
					m.logger.Error("agent onair string not found", map[string]string{
						"agent_id":     request.AgentID,
						"onair_string": onAirString,
						"timeout_ms":   strconv.FormatInt(onAirTimeout.Milliseconds(), 10),
					})
				}
			} else {
				time.Sleep(promptDelay)
			}

			// Inject skill metadata first if agent has skills
			if len(profile.Skills) > 0 {
				agentSkills := make([]*skill.Skill, 0, len(profile.Skills))
				for _, skillName := range profile.Skills {
					if skillEntry, ok := m.skills[skillName]; ok {
						agentSkills = append(agentSkills, skillEntry)
					}
				}

				if len(agentSkills) > 0 {
					skillXML := skill.GeneratePromptXML(agentSkills)
					if skillXML != "" {
						skillData := []byte(skillXML)
						if err := writePromptPayload(session, skillData); err != nil {
							m.logger.Warn("agent skill metadata write failed", map[string]string{
								"agent_id": request.AgentID,
								"error":    err.Error(),
							})
						} else {
							separator := "\n\n" + strings.Repeat("-", 72) + "\n\n"
							if err := writePromptPayload(session, []byte(separator)); err != nil {
								m.logger.Warn("agent prompt separator write failed", map[string]string{
									"agent_id": request.AgentID,
									"error":    err.Error(),
								})
							}
							m.logger.Info("agent skill metadata injected", map[string]string{
								"agent_id":    request.AgentID,
								"skill_count": strconv.Itoa(len(agentSkills)),
							})
						}
						time.Sleep(interPromptDelay)
					}
				}
			}

			// Inject custom prompts
			if len(promptNames) > 0 {
				wrotePrompt := false
				cleaned := make([]string, 0, len(promptNames))
				for _, promptName := range promptNames {
					promptName = strings.TrimSpace(promptName)
					if promptName != "" {
						cleaned = append(cleaned, promptName)
					}
				}
				for i, promptName := range cleaned {
					data, files, err := m.readPromptFile(promptName)
					if err != nil {
						m.logger.Warn("agent prompt file read failed", map[string]string{
							"agent_id": request.AgentID,
							"prompt":   promptName,
							"error":    err.Error(),
						})
						continue
					}
					if err := writePromptPayload(session, data); err != nil {
						m.logger.Warn("agent prompt write failed", map[string]string{
							"agent_id": request.AgentID,
							"prompt":   promptName,
							"error":    err.Error(),
						})
						return
					}
					session.PromptFiles = append(session.PromptFiles, files...)
					m.logger.Info("agent prompt rendered", map[string]string{
						"agent_id":     request.AgentID,
						"agent_name":   profile.Name,
						"prompt_files": strings.Join(files, ", "),
						"file_count":   strconv.Itoa(len(files)),
					})
					wrotePrompt = true
					if i < len(cleaned)-1 {
						time.Sleep(interPromptDelay)
					}
				}
				if wrotePrompt {
					time.Sleep(finalEnterDelay)
					if err := session.Write([]byte("\r")); err != nil {
						m.logger.Warn("agent prompt final enter failed", map[string]string{
							"agent_id": request.AgentID,
							"error":    err.Error(),
						})
						return
					}
					time.Sleep(enterKeyDelay)
					if err := session.Write([]byte("\n")); err != nil {
						m.logger.Warn("agent prompt final enter failed", map[string]string{
							"agent_id": request.AgentID,
							"error":    err.Error(),
						})
					}
				}
			}
		}()
	}

	return session, nil
}

func writePromptPayload(session *Session, payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	for offset := 0; offset < len(payload); offset += promptChunkSize {
		end := offset + promptChunkSize
		if end > len(payload) {
			end = len(payload)
		}
		if err := session.Write(payload[offset:end]); err != nil {
			return err
		}
		if end < len(payload) {
			time.Sleep(promptChunkDelay)
		}
	}
	return nil
}

func (m *Manager) readPromptFile(promptName string) ([]byte, []string, error) {
	if m.promptParser == nil {
		return nil, nil, errors.New("prompt parser unavailable")
	}
	result, err := m.promptParser.Render(promptName)
	if err != nil {
		return nil, nil, err
	}
	return result.Content, result.Files, nil
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

func (m *Manager) GetSessionByAgent(agentName string) (*Session, bool) {
	if strings.TrimSpace(agentName) == "" {
		return nil, false
	}
	m.mu.RLock()
	id, ok := m.agentSessions[agentName]
	if !ok {
		m.mu.RUnlock()
		return nil, false
	}
	session, ok := m.sessions[id]
	m.mu.RUnlock()
	return session, ok
}

func (m *Manager) GetAgentTerminal(agentName string) (string, bool) {
	if strings.TrimSpace(agentName) == "" {
		return "", false
	}
	m.mu.RLock()
	id, ok := m.agentSessions[agentName]
	if !ok {
		m.mu.RUnlock()
		return "", false
	}
	_, exists := m.sessions[id]
	m.mu.RUnlock()
	if !exists {
		m.mu.Lock()
		if existingID, ok := m.agentSessions[agentName]; ok && existingID == id {
			delete(m.agentSessions, agentName)
		}
		m.mu.Unlock()
		return "", false
	}
	return id, true
}

func (m *Manager) HistoryLines(id string, maxLines int) ([]string, error) {
	if maxLines <= 0 {
		maxLines = DefaultHistoryLines
	}
	if session, ok := m.Get(id); ok {
		return session.HistoryLines(maxLines)
	}
	if m.sessionLogs == "" {
		return nil, ErrSessionNotFound
	}

	path, err := latestSessionLogPath(m.sessionLogs, id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	lines, err := readLastLines(path, maxLines)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return lines, nil
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

func (m *Manager) SessionPersistenceEnabled() bool {
	if m == nil {
		return false
	}
	return m.sessionLogs != ""
}

func (m *Manager) Logger() *logging.Logger {
	if m == nil {
		return nil
	}
	return m.logger
}

func (m *Manager) AgentBus() *event.Bus[event.AgentEvent] {
	if m == nil {
		return nil
	}
	return m.agentBus
}

func (m *Manager) TerminalBus() *event.Bus[event.TerminalEvent] {
	if m == nil {
		return nil
	}
	return m.terminalBus
}

func (m *Manager) WorkflowBus() *event.Bus[event.WorkflowEvent] {
	if m == nil {
		return nil
	}
	return m.workflowBus
}

func (m *Manager) TemporalEnabled() bool {
	if m == nil {
		return false
	}
	return m.temporalEnabled
}

func (m *Manager) TemporalClient() temporal.WorkflowClient {
	if m == nil {
		return nil
	}
	return m.temporalClient
}

func (m *Manager) GetAgent(id string) (agent.Agent, bool) {
	m.mu.RLock()
	profile, ok := m.agents[id]
	m.mu.RUnlock()

	return profile, ok
}

func (m *Manager) LoadAgentForSession(agentID string) (*agent.Agent, bool, error) {
	if m == nil {
		return nil, false, errors.New("manager is nil")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, false, errors.New("agent id is required")
	}
	if m.agentCache == nil {
		profile, ok := m.GetAgent(agentID)
		if !ok {
			return nil, false, ErrAgentNotFound
		}
		profileCopy := profile
		return &profileCopy, false, nil
	}
	profile, reloaded, err := m.agentCache.LoadOrReload(agentID, m.agentsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, ErrAgentNotFound
		}
		return nil, false, err
	}
	if profile == nil {
		return nil, false, ErrAgentNotFound
	}
	m.mu.Lock()
	m.agents[agentID] = *profile
	m.mu.Unlock()
	return profile, reloaded, nil
}

func (m *Manager) ListAgents() []AgentInfo {
	m.mu.RLock()
	infos := make([]AgentInfo, 0, len(m.agents))
	for id, profile := range m.agents {
		infos = append(infos, AgentInfo{
			ID:          id,
			Name:        profile.Name,
			LLMType:     profile.CLIType,
			LLMModel:    profile.LLMModel,
			UseWorkflow: resolveWorkflowPreference(profile.UseWorkflow),
		})
	}
	m.mu.RUnlock()

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].ID < infos[j].ID
	})
	return infos
}

func (m *Manager) GetSkill(name string) (*skill.Skill, bool) {
	m.mu.RLock()
	entry, ok := m.skills[name]
	m.mu.RUnlock()

	return entry, ok
}

func (m *Manager) ListSkills() []SkillMetadata {
	m.mu.RLock()
	infos := make([]SkillMetadata, 0, len(m.skills))
	for _, entry := range m.skills {
		if entry == nil {
			continue
		}
		infos = append(infos, SkillMetadata{
			Name:        entry.Name,
			Description: entry.Description,
			Path:        entry.Path,
			License:     entry.License,
		})
	}
	m.mu.RUnlock()

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})
	return infos
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	session, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
		if session != nil && session.agent != nil && session.agent.Name != "" {
			agentName := session.agent.Name
			if existingID, ok := m.agentSessions[agentName]; ok && existingID == id {
				delete(m.agentSessions, agentName)
			}
		}
	}
	m.mu.Unlock()

	if !ok {
		return ErrSessionNotFound
	}

	agentID := ""
	agentName := ""
	if session != nil {
		agentID = session.AgentID
		if session.agent != nil {
			agentName = session.agent.Name
		}
	}

	if err := session.Close(); err != nil {
		m.logger.Warn("terminal close error", map[string]string{
			"terminal_id": id,
			"error":       err.Error(),
		})
		if m.terminalBus != nil {
			terminalEvent := event.NewTerminalEvent(id, "terminal_error")
			terminalEvent.Data = map[string]any{
				"error": err.Error(),
			}
			m.terminalBus.Publish(terminalEvent)
		}
		if agentID != "" && m.agentBus != nil {
			agentEvent := event.NewAgentEvent(agentID, agentName, "agent_error")
			agentEvent.Context = map[string]any{
				"error": err.Error(),
			}
			m.agentBus.Publish(agentEvent)
		}
	}
	if m.terminalBus != nil {
		m.terminalBus.Publish(event.NewTerminalEvent(id, "terminal_closed"))
	}
	if agentID != "" && m.agentBus != nil {
		m.agentBus.Publish(event.NewAgentEvent(agentID, agentName, "agent_stopped"))
	}
	if workflowID, workflowRunID, ok := session.WorkflowIdentifiers(); ok {
		m.logger.Info("workflow stopped", map[string]string{
			"terminal_id": id,
			"workflow_id": workflowID,
			"run_id":      workflowRunID,
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
