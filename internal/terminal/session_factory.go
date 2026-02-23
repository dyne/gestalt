package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/flow"
	"gestalt/internal/logging"
	"gestalt/internal/notify"
	"gestalt/internal/process"
)

type SessionFactoryOptions struct {
	Clock            Clock
	PtyFactory       PtyFactory
	ProcessRegistry  *process.Registry
	SessionLogDir    string
	InputHistoryDir  string
	BufferLines      int
	SessionLogMax    int64
	HistoryScanMax   int64
	LogCodexEvents   bool
	OutputPolicy     OutputBackpressurePolicy
	OutputSample     uint64
	NotificationSink notify.Sink
	Logger           *logging.Logger
	NextID           func() string
}

type SessionFactory struct {
	clock            Clock
	ptyFactory       PtyFactory
	processRegistry  *process.Registry
	sessionLogDir    string
	inputHistoryDir  string
	bufferLines      int
	sessionLogMax    int64
	historyScanMax   int64
	logCodexEvents   bool
	outputPolicy     OutputBackpressurePolicy
	outputSample     uint64
	notificationSink notify.Sink
	logger           *logging.Logger
	nextID           func() string
}

func NewSessionFactory(options SessionFactoryOptions) *SessionFactory {
	clock := options.Clock
	if clock == nil {
		clock = realClock{}
	}

	ptyFactory := options.PtyFactory
	if ptyFactory == nil {
		ptyFactory = DefaultPtyFactory()
	}

	bufferLines := options.BufferLines
	if bufferLines <= 0 {
		bufferLines = DefaultBufferLines
	}

	return &SessionFactory{
		clock:            clock,
		ptyFactory:       ptyFactory,
		processRegistry:  options.ProcessRegistry,
		sessionLogDir:    strings.TrimSpace(options.SessionLogDir),
		inputHistoryDir:  strings.TrimSpace(options.InputHistoryDir),
		bufferLines:      bufferLines,
		sessionLogMax:    options.SessionLogMax,
		historyScanMax:   options.HistoryScanMax,
		logCodexEvents:   options.LogCodexEvents,
		outputPolicy:     options.OutputPolicy,
		outputSample:     options.OutputSample,
		notificationSink: options.NotificationSink,
		logger:           options.Logger,
		nextID:           options.NextID,
	}
}

func (f *SessionFactory) Start(request sessionCreateRequest, profile *agent.Agent, shell, reservedID string) (*Session, string, error) {
	command, args, err := splitCommandLine(shell)
	if err != nil {
		f.logShellParseError(request, reservedID, shell, err)
		return nil, "", err
	}
	f.logShellCommandReady(request, reservedID, shell, command, args)

	pty, cmd, err := f.ptyFactory.Start(command, args...)
	if err != nil {
		err = wrapPtyStartError(err)
		f.logShellStartError(request, reservedID, shell, command, args, err)
		return nil, "", err
	}

	id := reservedID
	if id == "" {
		if f.nextID == nil {
			return nil, "", errors.New("session id generator unavailable")
		}
		id = f.nextID()
	}

	createdAt := f.clock.Now().UTC()
	var sessionLogger *SessionLogger
	if shouldLogSessionOutput(profile) {
		sessionLogger = f.createSessionLogger(id, createdAt)
	}
	inputLogger := f.createInputLogger(id, profile, createdAt)

	outputPolicy := f.outputPolicy
	outputSample := f.outputSample
	if _, ok := pty.(*mcpPty); ok {
		outputPolicy = OutputBackpressureBlock
		outputSample = 0
	}

	session := newSession(id, pty, nil, cmd, request.Title, request.Role, createdAt, f.bufferLines, f.historyScanMax, outputPolicy, outputSample, profile, sessionLogger, inputLogger)
	session.Command = shell
	if request.AgentID != "" {
		session.AgentID = request.AgentID
	}
	if profile != nil {
		session.ConfigHash = profile.ConfigHash
	}
	if f.processRegistry != nil && cmd != nil && cmd.Process != nil {
		pid := cmd.Process.Pid
		f.processRegistry.RegisterWithWait(pid, process.GroupID(pid), "session:"+id, func(ctx context.Context) error {
			done := make(chan error, 1)
			go func() {
				done <- cmd.Wait()
			}()
			select {
			case err := <-done:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		session.setProcessRegistry(f.processRegistry)
	}

	if mcp, ok := pty.(*mcpPty); ok {
		if f.logCodexEvents && !isAgentSession(request, profile) {
			if eventLogger := f.createMCPEventLogger(id, createdAt); eventLogger != nil {
				mcp.SetEventLogger(eventLogger)
			}
		}
		attachMCPTurnHandler(session, mcp, f.logger, f.notificationSink)
	}

	return session, id, nil
}

func (f *SessionFactory) StartExternal(request sessionCreateRequest, profile *agent.Agent, shell, reservedID string) (*Session, string, error) {
	id := strings.TrimSpace(reservedID)
	if id == "" {
		if f.nextID == nil {
			return nil, "", errors.New("session id generator unavailable")
		}
		id = f.nextID()
	}

	createdAt := f.clock.Now().UTC()
	var sessionLogger *SessionLogger
	if shouldLogSessionOutput(profile) {
		sessionLogger = f.createSessionLogger(id, createdAt)
	}
	inputLogger := f.createInputLogger(id, profile, createdAt)

	outputPolicy := f.outputPolicy
	outputSample := f.outputSample

	session := newSession(id, nil, newExternalRunner(), nil, request.Title, request.Role, createdAt, f.bufferLines, f.historyScanMax, outputPolicy, outputSample, profile, sessionLogger, inputLogger)
	session.Command = shell
	if request.AgentID != "" {
		session.AgentID = request.AgentID
	}
	if profile != nil {
		session.ConfigHash = profile.ConfigHash
	}
	return session, id, nil
}

// isAgentSession reports whether a session is backed by an agent profile.
func isAgentSession(request sessionCreateRequest, profile *agent.Agent) bool {
	if profile != nil {
		return true
	}
	return strings.TrimSpace(request.AgentID) != ""
}

func shouldLogSessionOutput(profile *agent.Agent) bool {
	return false
}

func (f *SessionFactory) createMCPEventLogger(id string, createdAt time.Time) *mcpEventLogger {
	if f.sessionLogDir == "" {
		if f.logger != nil {
			f.logger.Warn("mcp event log disabled: session log dir empty", map[string]string{
				"terminal_id": id,
			})
		}
		return nil
	}
	logger, err := newMCPEventLogger(f.sessionLogDir, id, createdAt)
	if err != nil {
		if f.logger != nil {
			f.logger.Warn("mcp event log unavailable", map[string]string{
				"terminal_id": id,
				"error":       err.Error(),
			})
		}
		return nil
	}
	return logger
}

func attachMCPTurnHandler(session *Session, pty *mcpPty, logger *logging.Logger, sink notify.Sink) {
	if session == nil || pty == nil || sink == nil {
		return
	}
	pty.SetTurnHandler(func(info mcpTurnInfo) {
		payload := map[string]any{
			"thread_id": info.ThreadID,
			"tool":      info.Tool,
		}
		if strings.TrimSpace(session.Model) != "" {
			payload["model"] = session.Model
		}
		occurredAt := time.Now().UTC()
		fields := flow.BuildNotifyFields(flow.NotifyFieldInput{
			SessionID:   session.ID,
			EventID:     fmt.Sprintf("gestalt-mcp:%s:%d", session.ID, info.Turn),
			PayloadType: "agent-turn-complete",
			OccurredAt:  occurredAt,
			Payload:     payload,
		})
		event := notify.Event{
			Fields:     fields,
			OccurredAt: occurredAt,
			Level:      "info",
			Message:    "agent-turn-complete",
		}
		if err := sink.Emit(context.Background(), event); err != nil && logger != nil {
			logger.Warn("mcp notify failed", map[string]string{
				"terminal_id": session.ID,
				"error":       err.Error(),
			})
		}
	})
}

func (f *SessionFactory) logShellCommandReady(request sessionCreateRequest, reservedID, shell, command string, args []string) {
	if f.logger == nil || !f.logger.Enabled(logging.LevelDebug) {
		return
	}
	safeArgs := redactDeveloperInstructionsArgs(args)
	safeShell := joinCommandLine(command, safeArgs)
	fields := map[string]string{
		"shell":   safeShell,
		"command": command,
	}
	if len(safeArgs) > 0 {
		fields["args"] = strings.Join(safeArgs, " ")
	}
	if request.AgentID != "" {
		fields["agent_id"] = request.AgentID
	}
	if reservedID != "" {
		fields["terminal_id"] = reservedID
	}
	f.logger.Debug("shell command ready", fields)
}

func (f *SessionFactory) logShellParseError(request sessionCreateRequest, reservedID, shell string, err error) {
	if f.logger == nil {
		return
	}
	fields := map[string]string{
		"shell": redactDeveloperInstructionsShell(shell),
		"error": err.Error(),
	}
	if request.AgentID != "" {
		fields["agent_id"] = request.AgentID
	}
	if reservedID != "" {
		fields["terminal_id"] = reservedID
	}
	f.logger.Warn("shell command parse failed", fields)
}

func (f *SessionFactory) logShellStartError(request sessionCreateRequest, reservedID, shell, command string, args []string, err error) {
	if f.logger == nil {
		return
	}
	safeArgs := redactDeveloperInstructionsArgs(args)
	safeShell := joinCommandLine(command, safeArgs)
	fields := map[string]string{
		"shell":   safeShell,
		"command": command,
		"error":   err.Error(),
	}
	if len(safeArgs) > 0 {
		fields["args"] = strings.Join(safeArgs, " ")
	}
	if request.AgentID != "" {
		fields["agent_id"] = request.AgentID
	}
	if reservedID != "" {
		fields["terminal_id"] = reservedID
	}
	if stderr := stderrFromExecError(err); stderr != "" {
		fields["stderr"] = FilterTerminalOutput(stderr)
	}
	f.logger.Error("shell command start failed", fields)
}

func (f *SessionFactory) createSessionLogger(id string, createdAt time.Time) *SessionLogger {
	if f.sessionLogDir == "" {
		return nil
	}
	logger, err := NewSessionLogger(f.sessionLogDir, id, createdAt, f.sessionLogMax)
	if err != nil {
		if f.logger != nil {
			f.logger.Warn("session log create failed", map[string]string{
				"terminal_id": id,
				"error":       err.Error(),
				"path":        f.sessionLogDir,
			})
		}
		return nil
	}
	return logger
}

func (f *SessionFactory) createInputLogger(id string, profile *agent.Agent, createdAt time.Time) *InputLogger {
	if f.inputHistoryDir == "" {
		return nil
	}
	historyName := id
	if profile != nil && strings.TrimSpace(profile.Name) != "" {
		historyName = profile.Name
	}
	logger, err := NewInputLogger(f.inputHistoryDir, historyName, createdAt)
	if err != nil {
		if f.logger != nil {
			f.logger.Warn("input history log create failed", map[string]string{
				"terminal_id": id,
				"error":       err.Error(),
				"path":        f.inputHistoryDir,
			})
		}
		return nil
	}
	return logger
}
