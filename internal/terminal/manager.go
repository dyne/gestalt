package terminal

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"gestalt/internal/agent"
	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/ports"
	"gestalt/internal/process"
	"gestalt/internal/prompt"
	"gestalt/internal/runner/launchspec"
	"gestalt/internal/skill"
	"gestalt/internal/temporal"
)

var ErrSessionNotFound = errors.New("terminal session not found")
var ErrAgentNotFound = errors.New("agent profile not found")
var ErrAgentRequired = errors.New("agent id is required")

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
	ProcessRegistry      *process.Registry
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
	SessionLogMaxBytes   int64
	HistoryScanMaxBytes  int64
	LogCodexEvents       bool
	TUIMode              string
	TUISnapshotInterval  time.Duration
	PromptFS             fs.FS
	PromptDir            string
	PortResolver         ports.PortResolver
}

// Manager is safe for concurrent use; mu guards the sessions map and lifecycle.
// ID generation may consult the sessions map and agent sequence under the mutex.
type Manager struct {
	mu              sync.RWMutex
	sessions        map[string]*Session
	agentSessions   map[string]string
	agentSequence   map[string]uint64
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
	historyScanMax  int64
	outputPolicy    OutputBackpressurePolicy
	outputSample    uint64
	portResolver    ports.PortResolver
	promptFS        fs.FS
	promptDir       string
	promptParser    *prompt.Parser
	processRegistry *process.Registry
}

type sessionCreateRequest struct {
	SessionID   string
	AgentID     string
	Role        string
	Title       string
	Shell       string
	Runner      string
	GUIModules  []string
	UseWorkflow *bool
}

type CreateOptions struct {
	AgentID     string
	Role        string
	Title       string
	Runner      string
	GUIModules  []string
	UseWorkflow *bool
}

const (
	promptDelay      = 3 * time.Second
	interPromptDelay = 100 * time.Millisecond
	finalEnterDelay  = 500 * time.Millisecond
	promptChunkDelay = 25 * time.Millisecond
	promptChunkSize  = 64
	enterKeyDelay    = 75 * time.Millisecond

	maxSessionIDLength   = 128
	maxSessionIDAttempts = 64
)

var onAirTimeout = 5 * time.Second
var defaultSnapshotSampleEvery uint64 = 10

type AgentInfo struct {
	ID          string
	Name        string
	LLMType     string
	LLMModel    string
	Interface   string
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

func resolveOutputPolicy(mode string, interval time.Duration) (OutputBackpressurePolicy, uint64) {
	trimmed := strings.ToLower(strings.TrimSpace(mode))
	if trimmed == "snapshot" {
		sampleEvery := defaultSnapshotSampleEvery
		if interval > 0 {
			intervalSamples := uint64(interval / (100 * time.Millisecond))
			if intervalSamples > 0 {
				sampleEvery = intervalSamples
			}
		}
		return OutputBackpressureSample, sampleEvery
	}
	return OutputBackpressureBlock, 0
}

func NewManager(opts ManagerOptions) *Manager {
	shell := opts.Shell
	if shell == "" {
		shell = DefaultShell()
	}

	factory := opts.PtyFactory
	if factory == nil {
		factory = NewMuxPtyFactory(DefaultPtyFactory(), StdioPtyFactory(), envBool("GESTALT_CODEX_MCP_DEBUG"))
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
	historyScanMax := opts.HistoryScanMaxBytes
	if historyScanMax < 0 {
		historyScanMax = 0
	}
	outputPolicy, outputSample := resolveOutputPolicy(opts.TUIMode, opts.TUISnapshotInterval)
	registry := opts.ProcessRegistry
	if registry == nil {
		registry = process.NewRegistry()
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
		agentSequence:   make(map[string]uint64),
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
		historyScanMax:  historyScanMax,
		outputPolicy:    outputPolicy,
		outputSample:    outputSample,
		portResolver:    portResolver,
		promptFS:        promptFS,
		promptDir:       promptDir,
		promptParser:    promptParser,
		processRegistry: registry,
	}
	manager.sessionFactory = NewSessionFactory(SessionFactoryOptions{
		Clock:           clock,
		PtyFactory:      factory,
		ProcessRegistry: registry,
		SessionLogDir:   sessionLogs,
		InputHistoryDir: inputHistoryDir,
		BufferLines:     bufferLines,
		SessionLogMax:   opts.SessionLogMaxBytes,
		HistoryScanMax:  historyScanMax,
		LogCodexEvents:  opts.LogCodexEvents,
		OutputPolicy:    outputPolicy,
		OutputSample:    outputSample,
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

func (m *Manager) ProcessRegistry() *process.Registry {
	if m == nil {
		return nil
	}
	return m.processRegistry
}

func (m *Manager) CreateWithOptions(options CreateOptions) (*Session, error) {
	return m.createSession(sessionCreateRequest{
		AgentID:     options.AgentID,
		Role:        options.Role,
		Title:       options.Title,
		Runner:      options.Runner,
		GUIModules:  options.GUIModules,
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
	runnerKind, err := normalizeRunnerKind(request.Runner)
	if err != nil {
		return nil, err
	}

	var profile *agent.Agent
	var promptNames []string
	var promptFiles []string
	var promptPayloads []string
	var developerInstructions string
	var onAirString string
	var agentName string
	var sanitizedAgentName string
	var sessionSequence uint64
	var sessionCLIConfig map[string]interface{}
	guiModules := normalizeSessionGUIModules(request.GUIModules)
	reservedID := strings.TrimSpace(request.SessionID)
	if request.AgentID == "" {
		return nil, ErrAgentRequired
	}
	if reservedID != "" {
		if err := validateSessionID(reservedID); err != nil {
			return nil, err
		}
	}
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
	if len(guiModules) == 0 && profile != nil && len(profile.GUIModules) > 0 {
		guiModules = append([]string(nil), profile.GUIModules...)
	}
	if runnerKind == launchspec.RunnerKindExternal && len(guiModules) == 0 {
		guiModules = append([]string(nil), defaultExternalGUIModules...)
	}

	useWorkflow := resolveWorkflowPreference(request.UseWorkflow)
	if request.UseWorkflow == nil && profile != nil {
		useWorkflow = resolveWorkflowPreference(profile.UseWorkflow)
	}

	if agentName != "" {
		sanitizedAgentName = sanitizeSessionName(agentName)
		if sanitizedAgentName == "" {
			return nil, errors.New("agent name is required")
		}
	}

	if reservedID == "" && sanitizedAgentName != "" {
		nextID, sequence, err := m.nextAgentSessionID(sanitizedAgentName)
		if err != nil {
			return nil, err
		}
		reservedID = nextID
		sessionSequence = sequence
	} else if reservedID != "" && sanitizedAgentName != "" {
		sequence, ok := parseAgentSessionSequence(reservedID, sanitizedAgentName)
		if !ok {
			return nil, errors.New("session id does not match agent name")
		}
		sessionSequence = sequence
	}

	if reservedID == "" && profile != nil && strings.EqualFold(strings.TrimSpace(profile.CLIType), "codex") {
		reservedID = m.nextIDValue()
	}
	if reservedID == "" && runnerKind == launchspec.RunnerKindExternal {
		reservedID = m.nextIDValue()
	}

	if profile != nil && strings.EqualFold(strings.TrimSpace(profile.CLIType), "codex") {
		result, err := m.buildCodexDeveloperInstructions(profile, reservedID)
		if err != nil {
			m.logger.Warn("codex developer instructions render failed", map[string]string{
				"agent_id": request.AgentID,
				"error":    err.Error(),
			})
		} else {
			promptFiles = append(promptFiles, result.PromptFiles...)
		}
		developerInstructions = result.Instructions
		if sessionCLIConfig == nil {
			sessionCLIConfig = map[string]interface{}{}
		}
		if _, ok := sessionCLIConfig["developer_instructions"]; ok {
			m.logger.Warn("codex developer instructions overwritten", map[string]string{
				"agent_id": request.AgentID,
			})
		}
		sessionCLIConfig["developer_instructions"] = developerInstructions
	}

	agentMCP := useAgentMCP(profile)

	if !shellOverrideSet && profile != nil {
		cliType := strings.TrimSpace(profile.CLIType)
		if strings.EqualFold(cliType, "codex") && !agentMCP {
			if sessionCLIConfig == nil {
				sessionCLIConfig = map[string]interface{}{}
			}
			sessionCLIConfig["notify"] = buildNotifyArgs(reservedID)
		}
		if cliType != "" && len(sessionCLIConfig) > 0 {
			generated := agent.BuildShellCommand(cliType, sessionCLIConfig)
			if strings.TrimSpace(generated) != "" {
				shell = generated
				if m.logger != nil {
					m.logger.Debug("agent shell command generated", map[string]string{
						"agent_id": request.AgentID,
						"shell":    redactDeveloperInstructionsShell(shell),
					})
				}
			} else if strings.TrimSpace(profile.Shell) != "" {
				shell = profile.Shell
			}
		} else if strings.TrimSpace(profile.Shell) != "" {
			shell = profile.Shell
		}
	}
	if !shellOverrideSet && agentMCP {
		shell = withCodexMCP(shell)
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

	var session *Session
	var id string
	if runnerKind == launchspec.RunnerKindExternal {
		session, id, err = m.sessionFactory.StartExternal(request, profile, shell, reservedID)
	} else {
		session, id, err = m.sessionFactory.Start(request, profile, shell, reservedID)
	}
	if err != nil {
		releaseReservation()
		return nil, err
	}
	if len(promptFiles) > 0 {
		session.PromptFiles = append(session.PromptFiles, promptFiles...)
	}
	if len(guiModules) > 0 {
		session.GUIModules = append([]string(nil), guiModules...)
	}
	if developerInstructions != "" {
		if mcp, ok := session.pty.(*mcpPty); ok {
			mcp.SetDeveloperInstructions(developerInstructions)
		}
	}

	m.mu.Lock()
	m.sessions[id] = session
	if sanitizedAgentName != "" && sessionSequence > 0 {
		if current := m.agentSequence[sanitizedAgentName]; sessionSequence > current {
			m.agentSequence[sanitizedAgentName] = sessionSequence
		}
	}
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

	if runnerKind == launchspec.RunnerKindExternal {
		cliType := ""
		if profile != nil {
			cliType = profile.CLIType
		}
		if len(promptNames) > 0 && !strings.EqualFold(strings.TrimSpace(cliType), "codex") {
			payloads, files := m.buildExternalPromptPayloads(promptNames, session.ID)
			if len(payloads) > 0 {
				promptPayloads = payloads
			}
			if len(files) > 0 {
				session.PromptFiles = append(session.PromptFiles, files...)
			}
		}
		session.LaunchSpec = m.buildLaunchSpec(session, profile, sessionCLIConfig, developerInstructions, promptPayloads)
	} else {
		m.startPromptInjection(session, request.AgentID, profile, promptNames, onAirString)
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

func (m *Manager) readPromptFile(promptName, sessionID string) ([]byte, []string, error) {
	if m.promptParser == nil {
		return nil, nil, errors.New("prompt parser unavailable")
	}
	result, err := m.promptParser.RenderWithContext(promptName, prompt.RenderContext{SessionID: sessionID})
	if err != nil {
		return nil, nil, err
	}
	return result.Content, result.Files, nil
}

func sanitizeSessionName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(trimmed))
	for _, r := range trimmed {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return strings.TrimSpace(builder.String())
}

func validateSessionID(id string) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return errors.New("session id is required")
	}
	if strings.ContainsAny(trimmed, "/\\") {
		return errors.New("session id contains invalid characters")
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return errors.New("session id contains invalid characters")
		}
	}
	if len(trimmed) > maxSessionIDLength {
		return fmt.Errorf("session id exceeds %d characters", maxSessionIDLength)
	}
	return nil
}

func parseAgentSessionSequence(sessionID, agentName string) (uint64, bool) {
	if sessionID == "" || agentName == "" {
		return 0, false
	}
	prefix := agentName + " "
	if !strings.HasPrefix(sessionID, prefix) {
		return 0, false
	}
	sequence := strings.TrimSpace(sessionID[len(prefix):])
	if sequence == "" {
		return 0, false
	}
	value, err := strconv.ParseUint(sequence, 10, 64)
	if err != nil || value == 0 {
		return 0, false
	}
	return value, true
}

func (m *Manager) nextAgentSessionID(agentName string) (string, uint64, error) {
	sanitized := sanitizeSessionName(agentName)
	if sanitized == "" {
		return "", 0, errors.New("agent name is required")
	}
	start := uint64(0)
	m.mu.RLock()
	if value, ok := m.agentSequence[sanitized]; ok {
		start = value
	}
	m.mu.RUnlock()
	for attempt := uint64(0); attempt < maxSessionIDAttempts; attempt++ {
		sequence := start + attempt + 1
		sessionID := fmt.Sprintf("%s %d", sanitized, sequence)
		if len(sessionID) > maxSessionIDLength {
			return "", 0, fmt.Errorf("session id exceeds %d characters", maxSessionIDLength)
		}
		m.mu.RLock()
		_, exists := m.sessions[sessionID]
		m.mu.RUnlock()
		if !exists {
			return sessionID, sequence, nil
		}
	}
	return "", 0, errors.New("session id collision")
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

// RegisterSession adds a pre-built session to the manager.
func (m *Manager) RegisterSession(session *Session) {
	if m == nil || session == nil || strings.TrimSpace(session.ID) == "" {
		return
	}
	m.mu.Lock()
	m.sessions[session.ID] = session
	m.mu.Unlock()
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

	lines, err := readLastLines(path, maxLines, m.historyScanMax)
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

	lines, startOffset, err := readLastLinesBefore(path, maxLines, *beforeCursor, m.historyScanMax)
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
		interfaceValue, err := profile.ResolveInterface()
		if err != nil {
			interfaceValue = agent.AgentInterfaceCLI
		}
		infos = append(infos, AgentInfo{
			ID:          id,
			Name:        profile.Name,
			LLMType:     profile.CLIType,
			LLMModel:    profile.LLMModel,
			Interface:   interfaceValue,
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
		sequenceAdjusted := false
		sanitizedAgentName := ""
		if session != nil && session.agent != nil && session.agent.Name != "" {
			agentName := session.agent.Name
			if existingID, ok := m.agentSessions[agentName]; ok && existingID == id {
				delete(m.agentSessions, agentName)
			}
			sanitizedAgentName = sanitizeSessionName(agentName)
			if sanitizedAgentName != "" {
				if parsed, ok := parseAgentSessionSequence(id, sanitizedAgentName); ok {
					if current, ok := m.agentSequence[sanitizedAgentName]; ok && current == parsed {
						sequenceAdjusted = true
					}
				}
			}
		}
		if sequenceAdjusted {
			maxSequence := uint64(0)
			for sessionID := range m.sessions {
				parsed, ok := parseAgentSessionSequence(sessionID, sanitizedAgentName)
				if ok && parsed > maxSequence {
					maxSequence = parsed
				}
			}
			if maxSequence == 0 {
				delete(m.agentSequence, sanitizedAgentName)
			} else {
				m.agentSequence[sanitizedAgentName] = maxSequence
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
	m.logOutputFilterStats(session)
	m.emitSessionStopped(id, session, agentID, agentName, closeErr)
	return nil
}

func (m *Manager) CloseAll() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	sessions := make(map[string]*Session, len(m.sessions))
	for id, session := range m.sessions {
		sessions[id] = session
	}
	m.sessions = make(map[string]*Session)
	m.agentSessions = make(map[string]string)
	m.mu.Unlock()

	var errs []error
	for id, session := range sessions {
		if session == nil {
			continue
		}
		agentID := session.AgentID
		agentName := ""
		if session.agent != nil {
			agentName = session.agent.Name
		}
		closeErr := session.Close()
		m.logOutputFilterStats(session)
		m.emitSessionStopped(id, session, agentID, agentName, closeErr)
		if closeErr != nil {
			errs = append(errs, fmt.Errorf("close session %s: %w", id, closeErr))
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) logOutputFilterStats(session *Session) {
	if m == nil || m.logger == nil || session == nil {
		return
	}
	stats := session.OutputFilterStats()
	if stats.FilterName == "" {
		return
	}
	fields := map[string]string{
		"terminal_id":       session.ID,
		"filter_name":       stats.FilterName,
		"filter_in_bytes":   strconv.FormatUint(stats.InBytes, 10),
		"filter_out_bytes":  strconv.FormatUint(stats.OutBytes, 10),
		"filter_drop_bytes": strconv.FormatUint(stats.DroppedBytes, 10),
	}
	m.logger.Info("terminal output filter stats", fields)
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

func buildNotifyArgs(sessionID string) []string {
	args := []string{"gestalt-notify", "--session-id", strings.TrimSpace(sessionID)}
	return args
}

func useAgentMCP(profile *agent.Agent) bool {
	if profile == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(profile.CLIType), "codex") {
		return false
	}
	iface, err := profile.RuntimeInterface(envBool("GESTALT_CODEX_FORCE_TUI"))
	if err != nil {
		return false
	}
	return strings.EqualFold(iface, agent.AgentInterfaceMCP)
}

func withCodexMCP(shell string) string {
	trimmed := strings.TrimSpace(shell)
	if trimmed == "" {
		return shell
	}
	fields, err := parseCommandLine(trimmed)
	if err != nil || len(fields) == 0 || fields[0] != "codex" {
		return shell
	}
	if len(fields) > 1 && fields[1] == "mcp-server" {
		return trimmed
	}
	if len(trimmed) == len("codex") {
		return "codex mcp-server"
	}
	return "codex mcp-server" + trimmed[len("codex"):]
}

func envBool(key string) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return false
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return parsed
}
