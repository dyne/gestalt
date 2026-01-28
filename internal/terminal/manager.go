package terminal

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/ports"
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
	PortResolver         ports.PortResolver
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
	sessionFactory  *SessionFactory
	agentRegistry   *agent.Registry
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
	portResolver    ports.PortResolver
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
var numberedAgentIDPattern = regexp.MustCompile(`^(.+?)-(\d+)$`)

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
		logger.Warn("temporal enabled without client", map[string]string{
			"gestalt.category": "workflow",
			"gestalt.source":   "backend",
		})
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
	portResolver := opts.PortResolver
	promptParser := prompt.NewParser(promptFS, promptDir, ".", portResolver)

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
	agentRegistry := agent.NewRegistry(agent.RegistryOptions{
		Agents:    agents,
		AgentsDir: opts.AgentsDir,
	})
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
		agentRegistry:   agentRegistry,
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
		portResolver:    portResolver,
		promptFS:        promptFS,
		promptDir:       promptDir,
		promptParser:    promptParser,
	}
	manager.sessionFactory = NewSessionFactory(SessionFactoryOptions{
		Clock:           clock,
		PtyFactory:      factory,
		SessionLogDir:   sessionLogs,
		InputHistoryDir: inputHistoryDir,
		BufferLines:     bufferLines,
		Logger:          logger,
		NextID:          manager.nextIDValue,
	})
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
	var sessionCLIConfig map[string]interface{}
	reservedID := strings.TrimSpace(request.SessionID)
	if request.AgentID != "" {
		agentProfile, ok := m.GetAgent(request.AgentID)
		if !ok || agentProfile.Name == "" {
			m.logger.Warn("agent not found or invalid", map[string]string{
				"gestalt.category": "agent",
				"gestalt.source":   "backend",
				"agent.id":         request.AgentID,
				"agent_id":         request.AgentID,
			})
			return nil, ErrAgentNotFound
		}
		profileCopy := agentProfile
		profile = &profileCopy
		if len(agentProfile.CLIConfig) > 0 {
			sessionCLIConfig = copyCLIConfig(agentProfile.CLIConfig)
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

	if profile != nil && profile.Singleton != nil && !*profile.Singleton {
		baseAgentID := strings.TrimSpace(request.AgentID)
		if matches := numberedAgentIDPattern.FindStringSubmatch(baseAgentID); len(matches) == 3 {
			baseAgentID = matches[1]
		}
		if baseAgentID != "" {
			m.mu.Lock()
			maxNumber := 0
			for id := range m.sessions {
				matches := numberedAgentIDPattern.FindStringSubmatch(id)
				if len(matches) != 3 {
					continue
				}
				if matches[1] != baseAgentID {
					continue
				}
				number, err := strconv.Atoi(matches[2])
				if err != nil {
					continue
				}
				if number > maxNumber {
					maxNumber = number
				}
			}
			nextID := fmt.Sprintf("%s-%d", baseAgentID, maxNumber+1)
			reservedID = nextID
			m.mu.Unlock()
		}
	}

	if reservedID == "" && agentName != "" && (profile == nil || profile.Singleton == nil || *profile.Singleton) {
		reservedID = m.nextIDValue()
	}

	if !shellOverrideSet && profile != nil {
		cliType := strings.TrimSpace(profile.CLIType)
		if strings.EqualFold(cliType, "codex") {
			if sessionCLIConfig == nil {
				sessionCLIConfig = map[string]interface{}{}
			}
			sessionCLIConfig["notify"] = buildNotifyArgs(reservedID, request.AgentID, profile.Name)
		}
		if cliType != "" && len(sessionCLIConfig) > 0 {
			generated := agent.BuildShellCommand(cliType, sessionCLIConfig)
			if strings.TrimSpace(generated) != "" {
				shell = generated
				if m.logger != nil {
					m.logger.Debug("agent shell command generated", map[string]string{
						"agent_id": request.AgentID,
						"shell":    shell,
					})
				}
			} else if strings.TrimSpace(profile.Shell) != "" {
				shell = profile.Shell
			}
		} else if strings.TrimSpace(profile.Shell) != "" {
			shell = profile.Shell
		}
	}

	if agentName != "" && (profile.Singleton == nil || *profile.Singleton) {
		m.mu.Lock()
		if existingID, ok := m.agentSessions[agentName]; ok {
			m.mu.Unlock()
			return nil, &AgentAlreadyRunningError{AgentName: agentName, TerminalID: existingID}
		}
		m.agentSessions[agentName] = reservedID
		m.mu.Unlock()
	}

	releaseReservation := func() {
		if agentName == "" || reservedID == "" || profile == nil || (profile.Singleton != nil && !*profile.Singleton) {
			return
		}
		m.mu.Lock()
		if existingID, ok := m.agentSessions[agentName]; ok && existingID == reservedID {
			delete(m.agentSessions, agentName)
		}
		m.mu.Unlock()
	}

	session, id, err := m.sessionFactory.Start(request, profile, shell, reservedID)
	if err != nil {
		releaseReservation()
		return nil, err
	}

	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()

	m.emitSessionStarted(id, request, agentName, shell)

	if useWorkflow && m.temporalEnabled && m.temporalClient != nil {
		startError := session.StartWorkflow(m.temporalClient, "", "")
		if startError != nil {
			m.logger.Warn("temporal workflow start failed", map[string]string{
				"gestalt.category":    "workflow",
				"gestalt.source":      "backend",
				"terminal.id":         id,
				"terminal_id":         id,
				"workflow.session_id": id,
				"error":               startError.Error(),
			})
		} else if workflowID, workflowRunID, ok := session.WorkflowIdentifiers(); ok {
			m.logger.Info("workflow started", map[string]string{
				"gestalt.category":    "workflow",
				"gestalt.source":      "backend",
				"terminal.id":         id,
				"terminal_id":         id,
				"workflow.id":         workflowID,
				"workflow.session_id": id,
				"workflow_id":         workflowID,
				"run_id":              workflowRunID,
			})
		}
	}

	m.startPromptInjection(session, request.AgentID, profile, promptNames, onAirString)

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

func renderOutputTail(logger *logging.Logger, lines []string, maxLines, maxBytes int) string {
	if len(lines) == 0 || maxLines <= 0 || maxBytes <= 0 {
		return ""
	}
	start := len(lines) - maxLines
	if start < 0 {
		start = 0
	}
	joined := strings.Join(lines[start:], "\n")
	filtered := FilterTerminalOutput(joined)
	if logger != nil && len(filtered) < len(joined) {
		reduced := len(joined) - len(filtered)
		if reduced >= 128 {
			logger.Debug("terminal output tail filtered", map[string]string{
				"before_bytes":  strconv.Itoa(len(joined)),
				"after_bytes":   strconv.Itoa(len(filtered)),
				"reduced_bytes": strconv.Itoa(reduced),
			})
		}
	}
	joined = filtered
	if len(joined) <= maxBytes {
		return joined
	}
	if maxBytes <= 3 {
		return joined[len(joined)-maxBytes:]
	}
	return "..." + joined[len(joined)-(maxBytes-3):]
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

func (m *Manager) HistoryCursor(id string) (*int64, error) {
	if m == nil || m.sessionLogs == "" {
		return nil, nil
	}

	path := ""
	if session, ok := m.Get(id); ok {
		if session.logger != nil {
			path = session.logger.Path()
		}
	} else {
		latest, err := latestSessionLogPath(m.sessionLogs, id)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, nil
			}
			return nil, err
		}
		path = latest
	}

	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	size := info.Size()
	return &size, nil
}

func (m *Manager) HistoryPage(id string, maxLines int, beforeCursor *int64) ([]string, *int64, error) {
	if maxLines <= 0 {
		maxLines = DefaultHistoryLines
	}
	if beforeCursor == nil {
		lines, err := m.HistoryLines(id, maxLines)
		if err != nil {
			return nil, nil, err
		}
		cursor, err := m.HistoryCursor(id)
		if err != nil {
			return lines, nil, err
		}
		return lines, cursor, nil
	}
	if m == nil || m.sessionLogs == "" {
		lines, err := m.HistoryLines(id, maxLines)
		return lines, nil, err
	}

	path := ""
	if session, ok := m.Get(id); ok {
		if session.logger != nil {
			path = session.logger.Path()
		}
	} else {
		latest, err := latestSessionLogPath(m.sessionLogs, id)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, nil, ErrSessionNotFound
			}
			return nil, nil, err
		}
		path = latest
	}

	if path == "" {
		lines, err := m.HistoryLines(id, maxLines)
		return lines, nil, err
	}

	lines, startOffset, err := readLastLinesBefore(path, maxLines, *beforeCursor)
	if err != nil {
		return nil, nil, err
	}
	cursor := startOffset
	return lines, &cursor, nil
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
	if m == nil || m.agentRegistry == nil {
		return agent.Agent{}, false
	}
	return m.agentRegistry.Get(id)
}

func (m *Manager) LoadAgentForSession(agentID string) (*agent.Agent, bool, error) {
	if m == nil {
		return nil, false, errors.New("manager is nil")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, false, errors.New("agent id is required")
	}
	if m.agentRegistry == nil {
		return nil, false, ErrAgentNotFound
	}
	profile, reloaded, err := m.agentRegistry.LoadOrReload(agentID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, ErrAgentNotFound
		}
		return nil, false, err
	}
	if profile == nil {
		return nil, false, ErrAgentNotFound
	}
	return profile, reloaded, nil
}

func (m *Manager) ListAgents() []AgentInfo {
	if m == nil || m.agentRegistry == nil {
		return nil
	}
	agents := m.agentRegistry.Snapshot()
	infos := make([]AgentInfo, 0, len(agents))
	for id, profile := range agents {
		infos = append(infos, AgentInfo{
			ID:          id,
			Name:        profile.Name,
			LLMType:     profile.CLIType,
			LLMModel:    profile.LLMModel,
			UseWorkflow: resolveWorkflowPreference(profile.UseWorkflow),
		})
	}

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

	closeErr := session.Close()
	m.emitSessionStopped(id, session, agentID, agentName, closeErr)
	return nil
}

func stderrFromExecError(err error) string {
	if err == nil {
		return ""
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return strings.TrimSpace(string(exitErr.Stderr))
	}
	return ""
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

func copyCLIConfig(config map[string]interface{}) map[string]interface{} {
	if len(config) == 0 {
		return nil
	}
	cloned := make(map[string]interface{}, len(config))
	for key, value := range config {
		cloned[key] = value
	}
	return cloned
}

func buildNotifyArgs(terminalID, agentID, agentName string) []string {
	args := []string{"gestalt-notify", "--terminal-id", strings.TrimSpace(terminalID), "--agent-id", strings.TrimSpace(agentID)}
	if strings.TrimSpace(agentName) != "" {
		args = append(args, "--agent-name", strings.TrimSpace(agentName))
	}
	return args
}
