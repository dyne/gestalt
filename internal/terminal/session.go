package terminal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/temporal"
	"gestalt/internal/temporal/workflows"

	"go.temporal.io/sdk/client"
)

var ErrSessionClosed = errors.New("terminal session closed")

type SessionState uint32

const (
	sessionStateStarting SessionState = iota
	sessionStateRunning
	sessionStateClosing
	sessionStateClosed
)

const dsrFallbackDelay = 250 * time.Millisecond
const temporalWorkflowStartTimeout = 5 * time.Second
const temporalSignalTimeout = 5 * time.Second

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

type Session struct {
	ID            string
	Title         string
	Role          string
	CreatedAt     time.Time
	LLMType       string
	LLMModel      string
	WorkflowID    *string
	WorkflowRunID *string

	ctx    context.Context
	cancel context.CancelFunc

	input  chan []byte
	output chan []byte

	pty      Pty
	cmd      *exec.Cmd
	bcast    *Broadcaster
	logger   *SessionLogger
	inputBuf *InputBuffer
	inputLog *InputLogger
	agent    *agent.Agent
	subs     int32
	dsrMu    sync.Mutex
	dsrTimer *time.Timer
	dsrOpen  bool
	closing  sync.Once
	closeErr error
	state    uint32

	workflowClient temporal.WorkflowClient
	workflowMutex  sync.RWMutex
}

type SessionInfo struct {
	ID        string
	Title     string
	Role      string
	CreatedAt time.Time
	Status    string
	LLMType   string
	LLMModel  string
	Skills    []string
}

func newSession(id string, pty Pty, cmd *exec.Cmd, title, role string, createdAt time.Time, bufferLines int, profile *agent.Agent, sessionLogger *SessionLogger, inputLogger *InputLogger) *Session {
	// readLoop -> output, writeLoop -> PTY, broadcastLoop -> subscribers.
	// Close cancels context and closes input so loops drain and exit cleanly.
	ctx, cancel := context.WithCancel(context.Background())
	llmType := ""
	llmModel := ""
	if profile != nil {
		llmType = profile.LLMType
		llmModel = profile.LLMModel
	}
	session := &Session{
		ID:        id,
		Title:     title,
		Role:      role,
		CreatedAt: createdAt,
		LLMType:   llmType,
		LLMModel:  llmModel,
		ctx:       ctx,
		cancel:    cancel,
		input:     make(chan []byte, 64),
		output:    make(chan []byte, 64),
		pty:       pty,
		cmd:       cmd,
		bcast:     NewBroadcaster(bufferLines),
		logger:    sessionLogger,
		inputBuf:  NewInputBuffer(DefaultInputBufferSize),
		inputLog:  inputLogger,
		agent:     profile,
		state:     uint32(sessionStateStarting),
	}

	go session.readLoop()
	go session.writeLoop()
	go session.broadcastLoop()
	session.setState(sessionStateRunning)

	return session
}

func (s *Session) Info() SessionInfo {
	skills := []string{}
	if s.agent != nil && len(s.agent.Skills) > 0 {
		skills = append(skills, s.agent.Skills...)
	}
	return SessionInfo{
		ID:        s.ID,
		Title:     s.Title,
		Role:      s.Role,
		CreatedAt: s.CreatedAt,
		Status:    s.State().String(),
		LLMType:   s.LLMType,
		LLMModel:  s.LLMModel,
		Skills:    skills,
	}
}

func (s *Session) Subscribe() (<-chan []byte, func()) {
	if s == nil || s.bcast == nil || s.State() == sessionStateClosed {
		ch := make(chan []byte)
		close(ch)
		return ch, func() {}
	}

	ch, cancel := s.bcast.Subscribe()
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

	defer func() {
		if r := recover(); r != nil {
			err = ErrSessionClosed
		}
	}()

	select {
	case s.input <- data:
		return nil
	case <-s.ctx.Done():
		return ErrSessionClosed
	}
}

func (s *Session) Resize(cols, rows uint16) error {
	if s.pty == nil {
		return ErrSessionClosed
	}

	if err := s.pty.Resize(cols, rows); err != nil {
		return fmt.Errorf("resize pty: %w", err)
	}
	return nil
}

func (s *Session) OutputLines() []string {
	return s.bcast.OutputLines()
}

func (s *Session) hasSubscribers() bool {
	if s == nil {
		return false
	}
	return atomic.LoadInt32(&s.subs) > 0
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

func (s *Session) StartWorkflow(temporalClient temporal.WorkflowClient, l1Task, l2Task string) error {
	if s == nil {
		return ErrSessionClosed
	}
	if temporalClient == nil {
		return errors.New("temporal client is required")
	}
	s.workflowMutex.RLock()
	workflowAlreadyStarted := s.WorkflowID != nil && s.WorkflowRunID != nil
	s.workflowMutex.RUnlock()
	if workflowAlreadyStarted {
		return nil
	}

	workflowID := "session-" + s.ID
	agentName := ""
	agentShell := ""
	if s.agent != nil {
		agentName = s.agent.Name
		agentShell = s.agent.Shell
	}
	request := workflows.SessionWorkflowRequest{
		SessionID: s.ID,
		AgentID:   agentName,
		L1Task:    l1Task,
		L2Task:    l2Task,
		Shell:     agentShell,
		StartTime: s.CreatedAt,
	}
	startOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: workflows.SessionTaskQueueName,
	}

	startContext, cancel := context.WithTimeout(context.Background(), temporalWorkflowStartTimeout)
	defer cancel()

	workflowRun, startError := temporalClient.ExecuteWorkflow(startContext, startOptions, workflows.SessionWorkflow, request)
	if startError != nil {
		return startError
	}
	if workflowRun == nil {
		return errors.New("temporal workflow run unavailable")
	}
	runID := workflowRun.GetRunID()

	s.workflowMutex.Lock()
	if s.WorkflowID == nil && s.WorkflowRunID == nil {
		s.workflowClient = temporalClient
		s.WorkflowID = &workflowID
		s.WorkflowRunID = &runID
	}
	s.workflowMutex.Unlock()

	return nil
}

func (s *Session) SendBellSignal(contextText string) error {
	bellSignal := workflows.BellSignal{
		Timestamp: time.Now().UTC(),
		Context:   contextText,
	}
	return s.sendWorkflowSignal(workflows.BellSignalName, bellSignal)
}

func (s *Session) UpdateTask(l1Task, l2Task string) error {
	taskSignal := workflows.UpdateTaskSignal{
		L1: l1Task,
		L2: l2Task,
	}
	return s.sendWorkflowSignal(workflows.UpdateTaskSignalName, taskSignal)
}

func (s *Session) SendResumeSignal(action string) error {
	resumeSignal := workflows.ResumeSignal{
		Action: action,
	}
	return s.sendWorkflowSignal(workflows.ResumeSignalName, resumeSignal)
}

func (s *Session) WorkflowIdentifiers() (string, string, bool) {
	if s == nil {
		return "", "", false
	}
	_, workflowID, workflowRunID, ok := s.workflowIdentifiers()
	return workflowID, workflowRunID, ok
}

func (s *Session) sendTerminateSignal(reason string) error {
	terminateSignal := workflows.TerminateSignal{
		Reason: reason,
	}
	return s.sendWorkflowSignal(workflows.TerminateSignalName, terminateSignal)
}

func (s *Session) sendWorkflowSignal(signalName string, payload interface{}) error {
	if s == nil {
		return ErrSessionClosed
	}
	temporalClient, workflowID, workflowRunID, ok := s.workflowIdentifiers()
	if !ok {
		return nil
	}
	signalContext, cancel := context.WithTimeout(context.Background(), temporalSignalTimeout)
	defer cancel()
	return temporalClient.SignalWorkflow(signalContext, workflowID, workflowRunID, signalName, payload)
}

func (s *Session) workflowIdentifiers() (temporal.WorkflowClient, string, string, bool) {
	s.workflowMutex.RLock()
	defer s.workflowMutex.RUnlock()

	if s.workflowClient == nil || s.WorkflowID == nil {
		return nil, "", "", false
	}
	runID := ""
	if s.WorkflowRunID != nil {
		runID = *s.WorkflowRunID
	}
	return s.workflowClient, *s.WorkflowID, runID, true
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
	fileLines, err := readLastLines(path, maxLines)
	if err != nil {
		if len(bufferLines) > 0 {
			return tailLines(bufferLines, maxLines), nil
		}
		return []string{}, err
	}
	return mergeHistoryLines(fileLines, bufferLines, maxLines), nil
}

func (s *Session) Close() error {
	s.closing.Do(func() {
		s.setState(sessionStateClosing)
		terminateError := s.sendTerminateSignal("session closed")
		s.clearDSRFallback()
		if s.cancel != nil {
			s.cancel()
		}
		close(s.input)
		closeError := s.closeResources()
		s.closeErr = errors.Join(closeError, terminateError)
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
		killErr := s.cmd.Process.Kill()
		if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			errs = append(errs, fmt.Errorf("kill process: %w", killErr))
		}
		if killErr == nil || errors.Is(killErr, os.ErrProcessDone) {
			if s.cmd.ProcessState == nil {
				if err := s.cmd.Wait(); err != nil && !errors.Is(err, os.ErrProcessDone) {
					errs = append(errs, fmt.Errorf("wait process: %w", err))
				}
			}
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
	defer close(s.output)

	buf := make([]byte, 4096)
	dsrTail := []byte{}
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
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
			select {
			case s.output <- chunk:
			case <-s.ctx.Done():
				return
			}
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

func (s *Session) writeLoop() {
	for data := range s.input {
		if _, err := s.pty.Write(data); err != nil {
			_ = s.Close()
			return
		}
	}
}

func (s *Session) broadcastLoop() {
	for chunk := range s.output {
		if s.logger != nil {
			s.logger.Write(chunk)
		}
		s.bcast.Broadcast(chunk)
	}
	s.bcast.Close()
	if s.logger != nil {
		_ = s.logger.Close()
	}
}
