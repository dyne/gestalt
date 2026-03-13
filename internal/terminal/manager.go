package terminal

import (
	"context"
	"errors"
	"flag"
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
	"gestalt/internal/notify"
	"gestalt/internal/ports"
	"gestalt/internal/process"
	"gestalt/internal/prompt"
	"gestalt/internal/runner/launchspec"
	"gestalt/internal/runner/tmux"
	"gestalt/internal/runner/tmuxsession"
	"gestalt/internal/skill"
)

var ErrSessionNotFound = errors.New("terminal session not found")
var ErrAgentNotFound = errors.New("agent profile not found")
var ErrAgentRequired = errors.New("agent id is required")
var ErrSessionNotTmuxManaged = errors.New("session is not tmux-managed")
var ErrTmuxSessionNotFound = errors.New("tmux session not found")
var ErrTmuxWindowNotFound = errors.New("tmux window not found")
var ErrTmuxUnavailable = errors.New("tmux unavailable")

type AgentAlreadyRunningError struct {
	AgentName  string
	TerminalID string
}

func (e *AgentAlreadyRunningError) Error() string {
	return fmt.Sprintf("agent %q already running in terminal %s", e.AgentName, e.TerminalID)
}

// ExternalTmuxError indicates tmux setup failure for external CLI sessions.
type ExternalTmuxError struct {
	Message string
	Err     error
}

func (e *ExternalTmuxError) Error() string {
	if e == nil {
		return "tmux create window failed"
	}
	if e.Err == nil {
		return e.Message
	}
	if strings.TrimSpace(e.Message) == "" {
		return e.Err.Error()
	}
	return e.Message + ": " + e.Err.Error()
}

func (e *ExternalTmuxError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type ManagerOptions struct {
	Shell                   string
	PtyFactory              PtyFactory
	ProcessRegistry         *process.Registry
	BufferLines             int
	Clock                   Clock
	Agents                  map[string]agent.Agent
	AgentsDir               string
	Skills                  map[string]*skill.Skill
	Logger                  *logging.Logger
	SessionLogDir           string
	InputHistoryDir         string
	SessionRetentionDays    int
	SessionLogMaxBytes      int64
	HistoryScanMaxBytes     int64
	LogCodexEvents          bool
	NotificationSink        notify.Sink
	TUIMode                 string
	TUISnapshotInterval     time.Duration
	PromptFS                fs.FS
	PromptDir               string
	PortResolver            ports.PortResolver
	StartExternalTmuxWindow func(*launchspec.LaunchSpec) error
	TmuxClientFactory       func() TmuxClient
}

// TmuxClient defines tmux operations used by manager activation flows.
type TmuxClient interface {
	HasSession(name string) (bool, error)
	HasWindow(sessionName, windowName string) (bool, error)
	SelectWindow(target string) error
	LoadBuffer(data []byte) error
	PasteBuffer(target string) error
	ResizePane(target string, cols, rows uint16) error
}

// Manager is safe for concurrent use; mu guards the sessions map and lifecycle.
type Manager struct {
	mu                      sync.RWMutex
	sessions                map[string]*Session
	agentSessions           map[string]string
	nextID                  uint64
	shell                   string
	factory                 PtyFactory
	bufferLines             int
	clock                   Clock
	sessionFactory          *SessionFactory
	agentRegistry           *agent.Registry
	skills                  map[string]*skill.Skill
	logger                  *logging.Logger
	notificationSink        notify.Sink
	agentBus                *event.Bus[event.AgentEvent]
	terminalBus             *event.Bus[event.TerminalEvent]
	workflowBus             *event.Bus[event.WorkflowEvent]
	chatBus                 *event.Bus[event.ChatEvent]
	sessionLogs             string
	inputHistoryDir         string
	retentionDays           int
	historyScanMax          int64
	outputPolicy            OutputBackpressurePolicy
	outputSample            uint64
	portResolver            ports.PortResolver
	promptFS                fs.FS
	promptDir               string
	promptParser            *prompt.Parser
	processRegistry         *process.Registry
	startExternalTmuxWindow func(*launchspec.LaunchSpec) error
	tmuxClientFactory       func() TmuxClient
	agentsHubMu             sync.Mutex
	agentsHubID             string
}

type sessionCreateRequest struct {
	SessionID string
	AgentID   string
	Role      string
	Title     string
	Shell     string
	Runner    string
}

type CreateOptions struct {
	AgentID string
	Role    string
	Title   string
	Runner  string
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
	notifyDefaultHost    = "127.0.0.1"
	notifyDefaultPort    = 57417
)

var onAirTimeout = 5 * time.Second
var defaultSnapshotSampleEvery uint64 = 10

type AgentInfo struct {
	ID        string
	Name      string
	LLMType   string
	Model     string
	Interface string
	Hidden    bool
}

type SkillMetadata struct {
	Name        string
	Description string
	Path        string
	License     string
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

	notificationSink := opts.NotificationSink
	if notificationSink == nil {
		notificationSink = notify.NewOTelSink(nil)
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
	chatBus := event.NewBus[event.ChatEvent](context.Background(), event.BusOptions{
		Name: "chat_events",
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
		sessions:                make(map[string]*Session),
		agentSessions:           make(map[string]string),
		shell:                   shell,
		factory:                 factory,
		bufferLines:             bufferLines,
		clock:                   clock,
		agentRegistry:           agentRegistry,
		skills:                  skills,
		logger:                  logger,
		notificationSink:        notificationSink,
		agentBus:                agentBus,
		terminalBus:             terminalBus,
		workflowBus:             workflowBus,
		chatBus:                 chatBus,
		sessionLogs:             sessionLogs,
		inputHistoryDir:         inputHistoryDir,
		retentionDays:           retentionDays,
		historyScanMax:          historyScanMax,
		outputPolicy:            outputPolicy,
		outputSample:            outputSample,
		portResolver:            portResolver,
		promptFS:                promptFS,
		promptDir:               promptDir,
		promptParser:            promptParser,
		processRegistry:         registry,
		startExternalTmuxWindow: opts.StartExternalTmuxWindow,
		tmuxClientFactory:       opts.TmuxClientFactory,
	}
	if manager.startExternalTmuxWindow == nil {
		if runningUnderGoTest() {
			manager.startExternalTmuxWindow = func(*launchspec.LaunchSpec) error { return nil }
		} else {
			manager.startExternalTmuxWindow = tmuxsession.StartWindow
		}
	}
	if manager.tmuxClientFactory == nil {
		if runningUnderGoTest() {
			manager.tmuxClientFactory = func() TmuxClient { return noopTmuxClient{} }
		} else {
			manager.tmuxClientFactory = func() TmuxClient {
				return tmux.NewClient()
			}
		}
	}
	manager.sessionFactory = NewSessionFactory(SessionFactoryOptions{
		Clock:            clock,
		PtyFactory:       factory,
		ProcessRegistry:  registry,
		SessionLogDir:    sessionLogs,
		InputHistoryDir:  inputHistoryDir,
		BufferLines:      bufferLines,
		SessionLogMax:    opts.SessionLogMaxBytes,
		HistoryScanMax:   historyScanMax,
		LogCodexEvents:   opts.LogCodexEvents,
		OutputPolicy:     outputPolicy,
		OutputSample:     outputSample,
		NotificationSink: notificationSink,
		Logger:           logger,
		NextID:           manager.nextIDValue,
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
		AgentID: options.AgentID,
		Role:    options.Role,
		Title:   options.Title,
		Runner:  options.Runner,
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
	runnerKind := launchspec.RunnerKindExternal

	var profile *agent.Agent
	var promptNames []string
	var promptPayloads []string
	var codexPromptFiles []string
	var agentName string
	var sanitizedAgentName string
	reservedID := strings.TrimSpace(request.SessionID)
	if request.AgentID == "" {
		return nil, ErrAgentRequired
	}
	if reservedID != "" {
		if err := validateSessionID(reservedID); err != nil {
			return nil, err
		}
	}
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
	if strings.TrimSpace(agentProfile.Name) != "" {
		request.Title = agentProfile.Name
		agentName = agentProfile.Name
	}
	if len(agentProfile.Prompts) > 0 {
		promptNames = append(promptNames, agentProfile.Prompts...)
	}
	if agentName != "" {
		sanitizedAgentName = sanitizeSessionName(agentName)
		if sanitizedAgentName == "" {
			return nil, errors.New("agent name is required")
		}
	}

	if reservedID == "" && sanitizedAgentName != "" {
		reservedID = canonicalAgentSessionID(sanitizedAgentName)
	} else if reservedID != "" && sanitizedAgentName != "" {
		if reservedID != canonicalAgentSessionID(sanitizedAgentName) {
			return nil, errors.New("session id does not match agent name")
		}
	}

	if reservedID == "" {
		reservedID = m.nextIDValue()
	}
	if !shellOverrideSet && profile != nil {
		if strings.EqualFold(strings.TrimSpace(profile.RuntimeType()), "codex") {
			cfg := make(map[string]interface{}, len(profile.CLIConfig)+2)
			for key, value := range profile.CLIConfig {
				cfg[key] = value
			}
			cfg["notify"] = m.buildNotifyArgs(reservedID)
			if m.portResolver != nil {
				if otelPort, ok := m.portResolver.Get("otel-grpc"); ok && otelPort > 0 {
					cfg["otel"] = map[string]interface{}{
						"exporter": map[string]interface{}{
							"otlp-grpc": map[string]interface{}{
								"endpoint": fmt.Sprintf("http://127.0.0.1:%d", otelPort),
							},
						},
					}
				}
			}
			developerInstructions, buildErr := m.buildCodexDeveloperInstructions(profile, reservedID)
			if buildErr != nil {
				if agentName != "" && reservedID != "" {
					m.mu.Lock()
					if existingID, ok := m.agentSessions[agentName]; ok && existingID == reservedID {
						delete(m.agentSessions, agentName)
					}
					m.mu.Unlock()
				}
				return nil, buildErr
			}
			shellArgs := agent.BuildCodexArgs(cfg, developerInstructions.Instructions)
			shell = joinCommandLine("codex", shellArgs)
			profile.Shell = shell
			if len(developerInstructions.PromptFiles) > 0 {
				codexPromptFiles = append(codexPromptFiles, developerInstructions.PromptFiles...)
				promptNames = nil
			}
		} else if strings.TrimSpace(profile.Shell) != "" {
			shell = profile.Shell
		}
	}
	if agentName != "" {
		for {
			m.mu.RLock()
			existingID, ok := m.agentSessions[agentName]
			var existingSession *Session
			if ok {
				existingSession = m.sessions[existingID]
			}
			m.mu.RUnlock()

			if !ok {
				break
			}
			if existingSession == nil {
				m.mu.Lock()
				if mappedID, stillMapped := m.agentSessions[agentName]; stillMapped && mappedID == existingID {
					delete(m.agentSessions, agentName)
				}
				m.mu.Unlock()
				continue
			}
			if m.isStaleExternalTmuxSession(existingSession) {
				_ = m.Delete(existingID)
				continue
			}
			return nil, &AgentAlreadyRunningError{AgentName: agentName, TerminalID: existingID}
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
		if agentName == "" || reservedID == "" || profile == nil {
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
	var err error
	if runnerKind == launchspec.RunnerKindExternal {
		session, id, err = m.sessionFactory.StartExternal(request, profile, shell, reservedID)
	} else {
		session, id, err = m.sessionFactory.Start(request, profile, shell, reservedID)
	}
	if err != nil {
		releaseReservation()
		return nil, err
	}
	if len(codexPromptFiles) > 0 {
		session.PromptFiles = append(session.PromptFiles, codexPromptFiles...)
	}
	if runnerKind == launchspec.RunnerKindExternal {
		if len(promptNames) > 0 {
			payloads, files := m.buildExternalPromptPayloads(promptNames, session.ID)
			if len(payloads) > 0 {
				promptPayloads = payloads
			}
			if len(files) > 0 {
				session.PromptFiles = append(session.PromptFiles, files...)
			}
		}
		session.LaunchSpec = m.buildLaunchSpec(session, promptPayloads)
		if m.startExternalTmuxWindow != nil {
			if err := m.startExternalTmuxWindow(session.LaunchSpec); err != nil {
				_ = session.Close()
				releaseReservation()
				return nil, wrapExternalTmuxError(err)
			}
			if err := m.ensureAgentsHubSession(); err != nil {
				_ = session.Close()
				releaseReservation()
				return nil, wrapExternalTmuxError(err)
			}
			if err := m.attachTmuxBridge(session); err != nil {
				_ = session.Close()
				releaseReservation()
				return nil, wrapExternalTmuxError(err)
			}
		}
	}
	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()

	m.emitSessionStarted(id, request, agentName, shell)

	return session, nil
}

func (m *Manager) attachTmuxBridge(session *Session) error {
	if m == nil || session == nil || !isTmuxManagedSession(session) {
		return nil
	}
	tmuxSessionName, err := tmuxsession.WorkdirSessionName()
	if err != nil {
		return err
	}
	target := tmuxSessionName + ":" + session.ID
	var bridgeWriteMu sync.Mutex
	writeFn := func(data []byte) error {
		if len(data) == 0 {
			return nil
		}
		// tmux input uses a two-step write (load-buffer + paste-buffer), so
		// keep it atomic per session to avoid interleaving under concurrent writes.
		bridgeWriteMu.Lock()
		defer bridgeWriteMu.Unlock()
		clientFactory := m.tmuxClientFactory
		if clientFactory == nil {
			return ErrTmuxUnavailable
		}
		client := clientFactory()
		if client == nil {
			return ErrTmuxUnavailable
		}
		if err := client.LoadBuffer(data); err != nil {
			return classifyTmuxBridgeError(err)
		}
		return classifyTmuxBridgeError(client.PasteBuffer(target))
	}
	resizeFn := func(cols, rows uint16) error {
		clientFactory := m.tmuxClientFactory
		if clientFactory == nil {
			return ErrTmuxUnavailable
		}
		client := clientFactory()
		if client == nil {
			return ErrTmuxUnavailable
		}
		return classifyTmuxBridgeError(client.ResizePane(target, cols, rows))
	}
	return session.AttachExternalRunner(writeFn, resizeFn, nil)
}

func classifyTmuxBridgeError(err error) error {
	if err == nil {
		return nil
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "can't find window") || strings.Contains(lower, "window not found") {
		return fmt.Errorf("%w: %v", ErrTmuxWindowNotFound, err)
	}
	if strings.Contains(lower, "can't find session") || strings.Contains(lower, "session not found") {
		return fmt.Errorf("%w: %v", ErrTmuxSessionNotFound, err)
	}
	if strings.Contains(lower, "executable file not found") ||
		strings.Contains(lower, "tmux runner unavailable") ||
		strings.Contains(lower, "tmux client unavailable") {
		return fmt.Errorf("%w: %v", ErrTmuxUnavailable, err)
	}
	return err
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

func canonicalAgentSessionID(agentName string) string {
	return fmt.Sprintf("%s 1", agentName)
}

func (m *Manager) nextIDValue() string {
	return strconv.FormatUint(atomic.AddUint64(&m.nextID, 1), 10)
}

func (m *Manager) ensureAgentsHubSession() error {
	if m == nil {
		return nil
	}

	m.agentsHubMu.Lock()
	defer m.agentsHubMu.Unlock()

	m.mu.RLock()
	existingID := strings.TrimSpace(m.agentsHubID)
	if existingID != "" {
		if _, ok := m.sessions[existingID]; ok {
			m.mu.RUnlock()
			return nil
		}
	}
	m.mu.RUnlock()

	tmuxSessionName, err := tmuxsession.WorkdirSessionName()
	if err != nil {
		return err
	}
	shell := joinCommandLine("tmux", []string{"attach", "-t", tmuxSessionName})
	request := sessionCreateRequest{
		SessionID: m.nextIDValue(),
		Role:      "agents-hub",
		Title:     "Agents",
	}
	session, id, err := m.sessionFactory.Start(request, nil, shell, request.SessionID)
	if err != nil {
		return err
	}

	m.mu.Lock()
	if existingID := strings.TrimSpace(m.agentsHubID); existingID != "" {
		if _, ok := m.sessions[existingID]; ok {
			m.mu.Unlock()
			_ = session.Close()
			return nil
		}
	}
	m.sessions[id] = session
	m.agentsHubID = id
	m.mu.Unlock()

	m.emitSessionStarted(id, request, "", shell)
	if m.terminalBus != nil {
		hubEvent := event.NewTerminalEvent(id, "agents_hub_ready")
		hubEvent.Data = map[string]any{
			"agents_session_id":   id,
			"agents_tmux_session": tmuxSessionName,
		}
		m.terminalBus.Publish(hubEvent)
	}
	return nil
}

// AgentsHubStatus reports the shared agents hub session ID and tmux session name.
func (m *Manager) AgentsHubStatus() (string, string) {
	if m == nil {
		return "", ""
	}
	m.mu.RLock()
	sessionID := strings.TrimSpace(m.agentsHubID)
	m.mu.RUnlock()
	if sessionID == "" {
		return "", ""
	}
	tmuxSessionName, err := tmuxsession.WorkdirSessionName()
	if err != nil {
		return sessionID, ""
	}
	return sessionID, tmuxSessionName
}

// ActivateSessionWindow selects the tmux window for a tmux-managed external CLI session.
func (m *Manager) ActivateSessionWindow(id string) error {
	if m == nil {
		return ErrSessionNotFound
	}
	session, ok := m.Get(id)
	if !ok || session == nil {
		return ErrSessionNotFound
	}
	if !strings.EqualFold(strings.TrimSpace(session.Runner), string(launchspec.RunnerKindExternal)) ||
		!strings.EqualFold(strings.TrimSpace(session.Interface), agent.AgentInterfaceCLI) {
		return ErrSessionNotTmuxManaged
	}

	tmuxSessionName, err := tmuxsession.WorkdirSessionName()
	if err != nil {
		return wrapExternalTmuxError(err)
	}
	clientFactory := m.tmuxClientFactory
	if clientFactory == nil {
		return wrapExternalTmuxError(errors.New("tmux client unavailable"))
	}
	client := clientFactory()
	if client == nil {
		return wrapExternalTmuxError(errors.New("tmux client unavailable"))
	}
	hasSession, err := client.HasSession(tmuxSessionName)
	if err != nil {
		return wrapExternalTmuxError(err)
	}
	if !hasSession {
		return ErrTmuxSessionNotFound
	}
	target := tmuxSessionName + ":" + session.ID
	if err := client.SelectWindow(target); err != nil {
		return wrapExternalTmuxError(err)
	}
	return nil
}

// PruneMissingExternalTmuxSessions removes tmux-managed external sessions whose windows no longer exist.
func (m *Manager) PruneMissingExternalTmuxSessions() {
	if m == nil {
		return
	}
	if runningUnderGoTest() {
		return
	}

	var candidates []*Session
	m.mu.RLock()
	for _, session := range m.sessions {
		if isTmuxManagedSession(session) {
			candidates = append(candidates, session)
		}
	}
	m.mu.RUnlock()

	if len(candidates) == 0 {
		return
	}

	tmuxSessionName, err := tmuxsession.WorkdirSessionName()
	if err != nil {
		if m.logger != nil {
			m.logger.Warn("tmux session name unavailable", map[string]string{
				"error": err.Error(),
			})
		}
		return
	}

	clientFactory := m.tmuxClientFactory
	if clientFactory == nil {
		return
	}
	client := clientFactory()
	if client == nil {
		return
	}

	hasSession, err := client.HasSession(tmuxSessionName)
	if err != nil {
		if m.logger != nil {
			m.logger.Warn("tmux session check failed", map[string]string{
				"tmux_session": tmuxSessionName,
				"error":        err.Error(),
			})
		}
		return
	}
	if !hasSession {
		for _, session := range candidates {
			_ = m.Delete(session.ID)
		}
		return
	}

	for _, session := range candidates {
		exists, err := client.HasWindow(tmuxSessionName, session.ID)
		if err != nil {
			if m.logger != nil {
				m.logger.Warn("tmux window check failed", map[string]string{
					"tmux_session": tmuxSessionName,
					"window":       session.ID,
					"error":        err.Error(),
				})
			}
			continue
		}
		if !exists {
			_ = m.Delete(session.ID)
		}
	}
}

func (m *Manager) isStaleExternalTmuxSession(session *Session) bool {
	if m == nil || !isTmuxManagedSession(session) {
		return false
	}
	tmuxSessionName, err := tmuxsession.WorkdirSessionName()
	if err != nil {
		return false
	}
	clientFactory := m.tmuxClientFactory
	if clientFactory == nil {
		return false
	}
	client := clientFactory()
	if client == nil {
		return false
	}
	hasSession, err := client.HasSession(tmuxSessionName)
	if err != nil {
		return false
	}
	if !hasSession {
		return true
	}
	exists, err := client.HasWindow(tmuxSessionName, session.ID)
	if err != nil {
		return false
	}
	return !exists
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
			ID:        id,
			Name:      profile.Name,
			LLMType:   profile.RuntimeType(),
			Model:     profile.Model,
			Interface: agent.AgentInterfaceCLI,
			Hidden:    profile.Hidden,
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
		if m.agentsHubID == id {
			m.agentsHubID = ""
		}
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
	m.agentsHubID = ""
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
		m.emitSessionStopped(id, session, agentID, agentName, closeErr)
		if closeErr != nil {
			errs = append(errs, fmt.Errorf("close session %s: %w", id, closeErr))
		}
	}
	return errors.Join(errs...)
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

func (m *Manager) buildNotifyArgs(sessionID string) []string {
	port := notifyDefaultPort
	if m != nil && m.portResolver != nil {
		if resolvedPort, ok := m.portResolver.Get("frontend"); ok && resolvedPort > 0 {
			port = resolvedPort
		}
	}
	return []string{
		"gestalt-notify",
		"--host", notifyDefaultHost,
		"--port", strconv.Itoa(port),
		"--session-id", strings.TrimSpace(sessionID),
	}
}

func isTmuxManagedSession(session *Session) bool {
	if session == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(session.Runner), string(launchspec.RunnerKindExternal)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(session.Interface), agent.AgentInterfaceCLI) {
		return false
	}
	return session.agent != nil
}

func wrapExternalTmuxError(err error) error {
	if err == nil {
		return nil
	}
	message := "tmux create window failed"
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "executable file not found") ||
		strings.Contains(lower, "tmux runner unavailable") ||
		strings.Contains(lower, "tmux client unavailable") {
		message = "tmux unavailable"
	}
	return &ExternalTmuxError{
		Message: message,
		Err:     err,
	}
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

type noopTmuxClient struct{}

func (noopTmuxClient) HasSession(name string) (bool, error)                   { return true, nil }
func (noopTmuxClient) HasWindow(sessionName, windowName string) (bool, error) { return true, nil }
func (noopTmuxClient) SelectWindow(target string) error                       { return nil }
func (noopTmuxClient) LoadBuffer(data []byte) error                           { return nil }
func (noopTmuxClient) PasteBuffer(target string) error                        { return nil }
func (noopTmuxClient) ResizePane(target string, cols, rows uint16) error      { return nil }

func runningUnderGoTest() bool {
	return flag.Lookup("test.v") != nil
}
