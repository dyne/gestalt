package terminal

import (
	"errors"
	"strings"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/logging"
)

type SessionFactoryOptions struct {
	Clock           Clock
	PtyFactory      PtyFactory
	SessionLogDir   string
	InputHistoryDir string
	BufferLines     int
	Logger          *logging.Logger
	NextID          func() string
}

type SessionFactory struct {
	clock           Clock
	ptyFactory      PtyFactory
	sessionLogDir   string
	inputHistoryDir string
	bufferLines     int
	logger          *logging.Logger
	nextID          func() string
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
		clock:           clock,
		ptyFactory:      ptyFactory,
		sessionLogDir:   strings.TrimSpace(options.SessionLogDir),
		inputHistoryDir: strings.TrimSpace(options.InputHistoryDir),
		bufferLines:     bufferLines,
		logger:          options.Logger,
		nextID:          options.NextID,
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
	sessionLogger := f.createSessionLogger(id, createdAt)
	inputLogger := f.createInputLogger(id, profile, createdAt)

	session := newSession(id, pty, cmd, request.Title, request.Role, createdAt, f.bufferLines, profile, sessionLogger, inputLogger)
	session.Command = shell
	if request.AgentID != "" {
		session.AgentID = request.AgentID
	}
	if profile != nil {
		session.ConfigHash = profile.ConfigHash
	}

	return session, id, nil
}

func (f *SessionFactory) logShellCommandReady(request sessionCreateRequest, reservedID, shell, command string, args []string) {
	if f.logger == nil || !f.logger.Enabled(logging.LevelDebug) {
		return
	}
	fields := map[string]string{
		"shell":   shell,
		"command": command,
	}
	if len(args) > 0 {
		fields["args"] = strings.Join(args, " ")
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
		"shell": shell,
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
	fields := map[string]string{
		"shell":   shell,
		"command": command,
		"error":   err.Error(),
	}
	if len(args) > 0 {
		fields["args"] = strings.Join(args, " ")
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
	logger, err := NewSessionLogger(f.sessionLogDir, id, createdAt)
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
