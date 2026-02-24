package terminal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/event"
	"gestalt/internal/process"
	"gestalt/internal/runner/launchspec"
)

var ErrSessionClosed = errors.New("terminal session closed")
var ErrRunnerUnavailable = errors.New("terminal runner unavailable")

type SessionState uint32

const (
	sessionStateStarting SessionState = iota
	sessionStateRunning
	sessionStateClosing
	sessionStateClosed
)

const dsrFallbackDelay = 250 * time.Millisecond
const terminalOutputSubscriberBuffer = 256
const terminalOutputWriteTimeout = 3 * time.Minute
const terminalOutputSlowThreshold = 5 * time.Second
const sessionProcessShutdownTimeout = 1 * time.Second

func (s SessionState) String() string {
	switch s {
	case sessionStateStarting:
		return "starting"
	case sessionStateClosing:
		return "closing"
	case sessionStateClosed:
		return "closed"
	default:
		return "running"
	}
}

type SessionMeta struct {
	ID          string
	AgentID     string
	Title       string
	Role        string
	CreatedAt   time.Time
	LLMType     string
	Model       string
	Interface   string
	Command     string
	Runner      string
	ConfigHash  string
	PromptFiles []string
	LaunchSpec  *launchspec.LaunchSpec
	agent       *agent.Agent
}

type SessionIO struct {
	ctx             context.Context
	cancel          context.CancelFunc
	runner          Runner
	outputPublisher *OutputPublisher
	pty             Pty
	cmd             *exec.Cmd
	pid             int
	pgid            int
	processRegistry *process.Registry
	outputBus       *event.Bus[[]byte]
	outputBuffer    *OutputBuffer
	logger          *SessionLogger
	inputBuf        *InputBuffer
	inputLog        *InputLogger
	historyScanMax  int64
	subs            int32
	dsrMu           sync.Mutex
	dsrTimer        *time.Timer
	dsrOpen         bool
	closing         sync.Once
	closeErr        error
	state           uint32
}

// PlanProgress records the most recent plan progress update for a session.
type PlanProgress struct {
	PlanFile  string
	L1        string
	L2        string
	TaskLevel int
	TaskState string
	UpdatedAt time.Time
}

type Session struct {
	SessionMeta
	SessionIO
	progressMu  sync.RWMutex
	progress    PlanProgress
	hasProgress bool
}

type SessionInfo struct {
	ID          string
	Title       string
	Role        string
	CreatedAt   time.Time
	Status      string
	LLMType     string
	Model       string
	Interface   string
	Runner      string
	Command     string
	Skills      []string
	PromptFiles []string
}

func newSession(id string, pty Pty, runner Runner, cmd *exec.Cmd, title, role string, createdAt time.Time, bufferLines int, historyScanMax int64, outputPolicy OutputBackpressurePolicy, outputSampleEvery uint64, profile *agent.Agent, sessionLogger *SessionLogger, inputLogger *InputLogger) *Session {
	// readLoop -> output publisher; runner handles input delivery.
	// Close cancels context and closes runner resources.
	ctx, cancel := context.WithCancel(context.Background())
	llmType := ""
	model := ""
	if profile != nil {
		llmType = profile.RuntimeType()
		model = profile.Model
	}
	interfaceValue := agent.AgentInterfaceCLI
	runnerKind := launchspec.RunnerKindServer
	if runner != nil {
		if _, ok := runner.(*externalRunner); ok {
			runnerKind = launchspec.RunnerKindExternal
		}
	}
	outputBuffer := NewOutputBuffer(bufferLines)
	outputBus := event.NewBus[[]byte](ctx, event.BusOptions{
		Name:                    "terminal_output",
		SubscriberBufferSize:    terminalOutputSubscriberBuffer,
		BlockOnFull:             true,
		WriteTimeout:            terminalOutputWriteTimeout,
		SlowSubscriberThreshold: terminalOutputSlowThreshold,
	})
	outputPublisher := NewOutputPublisher(OutputPublisherOptions{
		Logger:      sessionLogger,
		Buffer:      outputBuffer,
		Bus:         outputBus,
		MaxQueue:    defaultOutputQueueSize,
		Policy:      outputPolicy,
		SampleEvery: outputSampleEvery,
	})
	pid := 0
	pgid := 0
	if cmd != nil && cmd.Process != nil {
		pid = cmd.Process.Pid
		pgid = processGroupID(pid)
	}
	session := &Session{
		SessionMeta: SessionMeta{
			ID:        id,
			Title:     title,
			Role:      role,
			CreatedAt: createdAt,
			LLMType:   llmType,
			Model:     model,
			Interface: interfaceValue,
			Runner:    string(runnerKind),
			agent:     profile,
		},
		SessionIO: SessionIO{
			ctx:             ctx,
			cancel:          cancel,
			runner:          runner,
			outputPublisher: outputPublisher,
			pty:             pty,
			cmd:             cmd,
			pid:             pid,
			pgid:            pgid,
			outputBus:       outputBus,
			outputBuffer:    outputBuffer,
			logger:          sessionLogger,
			inputBuf:        NewInputBuffer(DefaultInputBufferSize),
			inputLog:        inputLogger,
			historyScanMax:  historyScanMax,
			state:           uint32(sessionStateStarting),
		},
	}

	if session.runner == nil && pty != nil {
		session.runner = newPtyRunner(ctx, pty, func(err error) {
			_ = session.Close()
		})
	}
	if pty != nil {
		go session.readLoop()
	}
	session.setState(sessionStateRunning)

	return session
}

func (s *Session) Info() SessionInfo {
	skills := []string{}
	if s.agent != nil && len(s.agent.Skills) > 0 {
		skills = append(skills, s.agent.Skills...)
	}
	promptFiles := []string{}
	if len(s.PromptFiles) > 0 {
		promptFiles = append(promptFiles, s.PromptFiles...)
	}
	interfaceValue := strings.TrimSpace(s.Interface)
	if interfaceValue == "" {
		interfaceValue = agent.AgentInterfaceCLI
	}
	return SessionInfo{
		ID:          s.ID,
		Title:       s.Title,
		Role:        s.Role,
		CreatedAt:   s.CreatedAt,
		Status:      s.State().String(),
		LLMType:     s.LLMType,
		Model:       s.Model,
		Interface:   interfaceValue,
		Runner:      s.Runner,
		Command:     s.Command,
		Skills:      skills,
		PromptFiles: promptFiles,
	}
}

func (s *Session) SendBellSignal(_ string) error {
	return nil
}

func (s *Session) AgentName() string {
	if s == nil || s.agent == nil {
		return ""
	}
	return s.agent.Name
}

// SetPlanProgress stores the latest plan progress for the session.
func (s *Session) SetPlanProgress(progress PlanProgress) {
	if s == nil {
		return
	}
	s.progressMu.Lock()
	s.progress = progress
	s.hasProgress = true
	s.progressMu.Unlock()
}

// PlanProgress returns the latest progress record, if present.
func (s *Session) PlanProgress() (PlanProgress, bool) {
	if s == nil {
		return PlanProgress{}, false
	}
	s.progressMu.RLock()
	defer s.progressMu.RUnlock()
	if !s.hasProgress {
		return PlanProgress{}, false
	}
	return s.progress, true
}

func (s *Session) IsMCP() bool {
	_ = s
	return false
}

func (s *Session) Subscribe() (<-chan []byte, func()) {
	if s == nil || s.outputBus == nil || s.State() == sessionStateClosed {
		ch := make(chan []byte)
		close(ch)
		return ch, func() {}
	}

	ch, cancel := s.outputBus.Subscribe()
	atomic.AddInt32(&s.subs, 1)
	var once sync.Once
	wrapped := func() {
		once.Do(func() {
			cancel()
			atomic.AddInt32(&s.subs, -1)
		})
	}
	return ch, wrapped
}

func (s *Session) Write(data []byte) (err error) {
	if len(data) == 0 {
		return nil
	}
	if s == nil {
		return ErrSessionClosed
	}
	state := s.State()
	if state == sessionStateClosing || state == sessionStateClosed {
		return ErrSessionClosed
	}
	if containsDSRResponse(data) {
		s.clearDSRFallback()
	}
	if s.runner == nil {
		return ErrRunnerUnavailable
	}

	defer func() {
		if r := recover(); r != nil {
			err = ErrSessionClosed
		}
	}()

	if err := s.runner.Write(data); err != nil {
		return err
	}
	return nil
}

func (s *Session) Resize(cols, rows uint16) error {
	if s.runner == nil {
		return ErrRunnerUnavailable
	}

	if err := s.runner.Resize(cols, rows); err != nil {
		return fmt.Errorf("resize runner: %w", err)
	}
	return nil
}

func (s *Session) PublishOutputChunk(chunk []byte) {
	if s == nil || s.outputPublisher == nil || len(chunk) == 0 {
		return
	}
	s.outputPublisher.PublishWithContext(s.ctx, chunk)
}

func (s *Session) AttachExternalRunner(writeFn func([]byte) error, resizeFn func(uint16, uint16) error, closeFn func() error) error {
	if s == nil {
		return ErrSessionClosed
	}
	runner, ok := s.runner.(*externalRunner)
	if !ok {
		return ErrRunnerUnavailable
	}
	return runner.Attach(writeFn, resizeFn, closeFn)
}

func (s *Session) DetachExternalRunner() {
	if s == nil {
		return
	}
	if runner, ok := s.runner.(*externalRunner); ok {
		runner.Detach()
	}
}

func (s *Session) OutputLines() []string {
	if s == nil || s.outputBuffer == nil {
		return nil
	}
	return s.outputBuffer.Lines()
}

func (s *Session) hasSubscribers() bool {
	if s == nil {
		return false
	}
	return atomic.LoadInt32(&s.subs) > 0
}

func (s *Session) SubscriberCount() int32 {
	if s == nil {
		return 0
	}
	return atomic.LoadInt32(&s.subs)
}

func (s *Session) RecordInput(command string) {
	if s == nil {
		return
	}
	entry := InputEntry{
		Command:   command,
		Timestamp: time.Now().UTC(),
	}
	if s.inputBuf != nil {
		s.inputBuf.AppendEntry(entry)
	}
	if s.inputLog != nil {
		s.inputLog.Write(entry)
	}
}

func (s *Session) GetInputHistory() []InputEntry {
	if s == nil || s.inputBuf == nil {
		return []InputEntry{}
	}
	return s.inputBuf.GetAll()
}

func (s *Session) HistoryLines(maxLines int) ([]string, error) {
	bufferLines := s.OutputLines()
	if s.logger == nil {
		return tailLines(bufferLines, maxLines), nil
	}
	path := s.logger.Path()
	if path == "" {
		return tailLines(bufferLines, maxLines), nil
	}
	fileLines, err := readLastLines(path, maxLines, s.historyScanMax)
	if err != nil {
		if len(bufferLines) > 0 {
			return tailLines(bufferLines, maxLines), nil
		}
		return []string{}, err
	}
	return fileLines, nil
}

func (s *Session) LogPath() string {
	if s == nil || s.logger == nil {
		return ""
	}
	return s.logger.Path()
}

func (s *Session) setProcessRegistry(registry *process.Registry) {
	if s == nil {
		return
	}
	s.processRegistry = registry
}

func (s *Session) Close() error {
	s.closing.Do(func() {
		s.setState(sessionStateClosing)
		s.clearDSRFallback()
		if s.cancel != nil {
			s.cancel()
		}
		if s.runner != nil {
			_ = s.runner.Close()
		}
		closeError := s.closeResources()
		s.closeErr = closeError
		s.setState(sessionStateClosed)
	})

	return s.closeErr
}

func (s *Session) State() SessionState {
	return SessionState(atomic.LoadUint32(&s.state))
}

func (s *Session) setState(state SessionState) {
	atomic.StoreUint32(&s.state, uint32(state))
}

func (s *Session) closeResources() error {
	var errs []error
	if s.pty != nil {
		if err := s.pty.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			errs = append(errs, fmt.Errorf("close pty: %w", err))
		}
	}
	if s.cmd != nil && s.cmd.Process != nil {
		if err := terminateProcessTree(s.cmd, s.pid, s.pgid, sessionProcessShutdownTimeout); err != nil {
			errs = append(errs, fmt.Errorf("terminate process: %w", err))
		}
		if s.processRegistry != nil && s.pid > 0 {
			s.processRegistry.Unregister(s.pid)
		}
	}
	if s.inputLog != nil {
		if err := s.inputLog.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close input log: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (s *Session) readLoop() {
	defer func() {
		if s.outputPublisher != nil {
			s.outputPublisher.Close()
		}
	}()

	buf := make([]byte, 4096)
	dsrTail := []byte{}
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if containsDSR(dsrTail, chunk) {
				if s.hasSubscribers() {
					s.scheduleDSRFallback()
				} else {
					_ = s.Write([]byte("\x1b[1;1R"))
				}
			}
			dsrTail = updateDSRTail(dsrTail, chunk)
			s.PublishOutputChunk(chunk)
		}
		if err != nil {
			_ = s.Close()
			return
		}
	}
}

func containsDSR(tail, chunk []byte) bool {
	if len(chunk) == 0 {
		return false
	}
	combined := make([]byte, 0, len(tail)+len(chunk))
	combined = append(combined, tail...)
	combined = append(combined, chunk...)
	return bytes.Contains(combined, []byte("\x1b[6n"))
}

func containsDSRResponse(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	for i := 0; i < len(data); i++ {
		if data[i] != 0x1b || i+2 >= len(data) || data[i+1] != '[' {
			continue
		}
		j := i + 2
		if j < len(data) && data[j] == '?' {
			j++
		}
		start := j
		for j < len(data) && data[j] >= '0' && data[j] <= '9' {
			j++
		}
		if j == start || j >= len(data) || data[j] != ';' {
			continue
		}
		j++
		start = j
		for j < len(data) && data[j] >= '0' && data[j] <= '9' {
			j++
		}
		if j == start || j >= len(data) || data[j] != 'R' {
			continue
		}
		return true
	}
	return false
}

func updateDSRTail(tail, chunk []byte) []byte {
	const dsrSize = 4
	const tailSize = dsrSize - 1
	if tailSize <= 0 {
		return nil
	}
	if len(chunk) >= tailSize {
		return append([]byte(nil), chunk[len(chunk)-tailSize:]...)
	}
	combined := make([]byte, 0, len(tail)+len(chunk))
	combined = append(combined, tail...)
	combined = append(combined, chunk...)
	if len(combined) <= tailSize {
		return combined
	}
	return append([]byte(nil), combined[len(combined)-tailSize:]...)
}

func (s *Session) scheduleDSRFallback() {
	if s == nil {
		return
	}
	s.dsrMu.Lock()
	s.dsrOpen = true
	if s.dsrTimer != nil {
		s.dsrTimer.Stop()
	}
	s.dsrTimer = time.AfterFunc(dsrFallbackDelay, func() {
		s.dsrMu.Lock()
		pending := s.dsrOpen
		s.dsrOpen = false
		s.dsrMu.Unlock()
		if pending {
			_ = s.Write([]byte("\x1b[1;1R"))
		}
	})
	s.dsrMu.Unlock()
}

func (s *Session) clearDSRFallback() {
	if s == nil {
		return
	}
	s.dsrMu.Lock()
	s.dsrOpen = false
	if s.dsrTimer != nil {
		s.dsrTimer.Stop()
	}
	s.dsrMu.Unlock()
}
