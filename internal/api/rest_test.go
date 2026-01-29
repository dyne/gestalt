package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/otel"
	"gestalt/internal/skill"
	temporalcore "gestalt/internal/temporal"
	"gestalt/internal/temporal/workflows"
	"gestalt/internal/terminal"
	"gestalt/internal/version"

	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakePty struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

const testAgentID = "codex"

var testAgentsDirOnce sync.Once
var testAgentsDir string

func ensureTestAgentsDir() string {
	testAgentsDirOnce.Do(func() {
		dir, err := os.MkdirTemp("", "gestalt-test-agents-*")
		if err != nil {
			panic(err)
		}
		testAgentsDir = dir
		agentTOML := "name = \"Codex\"\nshell = \"/bin/sh\"\ncli_type = \"codex\"\n"
		if err := os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(agentTOML), 0644); err != nil {
			panic(err)
		}
	})
	return testAgentsDir
}

func newTestManager(options terminal.ManagerOptions) *terminal.Manager {
	if options.Agents == nil {
		options.Agents = map[string]agent.Agent{
			testAgentID: {Name: "Codex"},
		}
	}
	if options.AgentsDir == "" {
		options.AgentsDir = ensureTestAgentsDir()
	}
	return terminal.NewManager(options)
}

func escapeID(id string) string {
	return url.PathEscape(id)
}

func terminalPath(id string) string {
	return "/api/sessions/" + escapeID(id)
}

func newFakePty() *fakePty {
	reader, writer := io.Pipe()
	return &fakePty{reader: reader, writer: writer}
}

func (p *fakePty) Read(data []byte) (int, error) {
	return p.reader.Read(data)
}

func (p *fakePty) Write(data []byte) (int, error) {
	return p.writer.Write(data)
}

func (p *fakePty) Close() error {
	_ = p.reader.Close()
	return p.writer.Close()
}

func (p *fakePty) Resize(cols, rows uint16) error {
	return nil
}

type fakeFactory struct {
	mu       sync.Mutex
	ptys     []*fakePty
	commands []string
}

func (f *fakeFactory) Start(command string, args ...string) (terminal.Pty, *exec.Cmd, error) {
	pty := newFakePty()

	f.mu.Lock()
	f.ptys = append(f.ptys, pty)
	f.commands = append(f.commands, command)
	f.mu.Unlock()

	return pty, nil, nil
}

type fakeEncodedValue struct {
	payload interface{}
}

func (value fakeEncodedValue) HasValue() bool {
	return value.payload != nil
}

func (value fakeEncodedValue) Get(valuePtr interface{}) error {
	if value.payload == nil {
		return errors.New("no payload")
	}
	data, err := json.Marshal(value.payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, valuePtr)
}

type fakeWorkflowRun struct {
	workflowID string
	runID      string
}

func (run *fakeWorkflowRun) GetID() string {
	return run.workflowID
}

func (run *fakeWorkflowRun) GetRunID() string {
	return run.runID
}

func (run *fakeWorkflowRun) Get(ctx context.Context, valuePtr interface{}) error {
	return nil
}

func (run *fakeWorkflowRun) GetWithOptions(ctx context.Context, valuePtr interface{}, options client.WorkflowRunGetOptions) error {
	return nil
}

func boolPtr(value bool) *bool {
	return &value
}

type fakeWorkflowQueryClient struct {
	runID        string
	queryResults map[string]workflows.SessionWorkflowState
}

func (client *fakeWorkflowQueryClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	return &fakeWorkflowRun{workflowID: options.ID, runID: client.runID}, nil
}

func (client *fakeWorkflowQueryClient) SignalWithStartWorkflow(ctx context.Context, workflowID, signalName string, signalArg interface{}, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	return &fakeWorkflowRun{workflowID: workflowID, runID: client.runID}, nil
}

func (client *fakeWorkflowQueryClient) GetWorkflowHistory(ctx context.Context, workflowID string, runID string, isLongPoll bool, filterType enumspb.HistoryEventFilterType) client.HistoryEventIterator {
	return nil
}

func (client *fakeWorkflowQueryClient) QueryWorkflow(ctx context.Context, workflowID, runID, queryType string, args ...interface{}) (converter.EncodedValue, error) {
	if client.queryResults == nil {
		return nil, errors.New("no query results")
	}
	result, ok := client.queryResults[workflowID]
	if !ok {
		return nil, errors.New("workflow not found")
	}
	return fakeEncodedValue{payload: result}, nil
}

func (client *fakeWorkflowQueryClient) SignalWorkflow(ctx context.Context, workflowID, runID, signalName string, arg interface{}) error {
	return nil
}

func (client *fakeWorkflowQueryClient) Close() {
}

type workflowSignalRecord struct {
	workflowID string
	runID      string
	signalName string
	payload    interface{}
}

type workflowSignalWithStartRecord struct {
	workflowID   string
	signalName   string
	payload      interface{}
	workflowType interface{}
	options      client.StartWorkflowOptions
}

type fakeWorkflowSignalClient struct {
	runID   string
	signals []workflowSignalRecord
	started []workflowSignalWithStartRecord
}

func (client *fakeWorkflowSignalClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	return &fakeWorkflowRun{workflowID: options.ID, runID: client.runID}, nil
}

func (client *fakeWorkflowSignalClient) SignalWithStartWorkflow(ctx context.Context, workflowID, signalName string, signalArg interface{}, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	client.started = append(client.started, workflowSignalWithStartRecord{
		workflowID:   workflowID,
		signalName:   signalName,
		payload:      signalArg,
		workflowType: workflow,
		options:      options,
	})
	return &fakeWorkflowRun{workflowID: workflowID, runID: client.runID}, nil
}

func (client *fakeWorkflowSignalClient) GetWorkflowHistory(ctx context.Context, workflowID string, runID string, isLongPoll bool, filterType enumspb.HistoryEventFilterType) client.HistoryEventIterator {
	return nil
}

func (client *fakeWorkflowSignalClient) QueryWorkflow(ctx context.Context, workflowID, runID, queryType string, args ...interface{}) (converter.EncodedValue, error) {
	return nil, errors.New("query not supported")
}

func (client *fakeWorkflowSignalClient) SignalWorkflow(ctx context.Context, workflowID, runID, signalName string, arg interface{}) error {
	client.signals = append(client.signals, workflowSignalRecord{
		workflowID: workflowID,
		runID:      runID,
		signalName: signalName,
		payload:    arg,
	})
	return nil
}

func (client *fakeWorkflowSignalClient) Close() {
}

type fakeHistoryIterator struct {
	events []*historypb.HistoryEvent
	index  int
}

func (iterator *fakeHistoryIterator) HasNext() bool {
	return iterator.index < len(iterator.events)
}

func (iterator *fakeHistoryIterator) Next() (*historypb.HistoryEvent, error) {
	if !iterator.HasNext() {
		return nil, errors.New("no more events")
	}
	event := iterator.events[iterator.index]
	iterator.index++
	return event, nil
}

type fakeWorkflowHistoryClient struct {
	runID  string
	events []*historypb.HistoryEvent
}

func (client *fakeWorkflowHistoryClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	return &fakeWorkflowRun{workflowID: options.ID, runID: client.runID}, nil
}

func (client *fakeWorkflowHistoryClient) SignalWithStartWorkflow(ctx context.Context, workflowID, signalName string, signalArg interface{}, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	return &fakeWorkflowRun{workflowID: workflowID, runID: client.runID}, nil
}

func (client *fakeWorkflowHistoryClient) GetWorkflowHistory(ctx context.Context, workflowID string, runID string, isLongPoll bool, filterType enumspb.HistoryEventFilterType) client.HistoryEventIterator {
	return &fakeHistoryIterator{events: client.events}
}

func (client *fakeWorkflowHistoryClient) QueryWorkflow(ctx context.Context, workflowID, runID, queryType string, args ...interface{}) (converter.EncodedValue, error) {
	return nil, errors.New("query not supported")
}

func (client *fakeWorkflowHistoryClient) SignalWorkflow(ctx context.Context, workflowID, runID, signalName string, arg interface{}) error {
	return nil
}

func (client *fakeWorkflowHistoryClient) Close() {
}

func TestStatusHandlerRequiresAuth(t *testing.T) {
	handler := &RestHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleStatus)(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.Code)
	}
}

func TestStatusHandlerReturnsCount(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	created, err := manager.Create(testAgentID, "", "")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleStatus)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload statusResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.SessionCount != 1 {
		t.Fatalf("expected 1 session, got %d", payload.SessionCount)
	}
	expectedDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if payload.WorkingDir != expectedDir {
		t.Fatalf("expected working dir %q, got %q", expectedDir, payload.WorkingDir)
	}
	info := version.GetVersionInfo()
	if payload.Version != info.Version {
		t.Fatalf("expected version %q, got %q", info.Version, payload.Version)
	}
	if payload.Major != info.Major || payload.Minor != info.Minor || payload.Patch != info.Patch {
		t.Fatalf("expected version %d.%d.%d, got %d.%d.%d", info.Major, info.Minor, info.Patch, payload.Major, payload.Minor, payload.Patch)
	}
	if payload.Built != info.Built {
		t.Fatalf("expected built %q, got %q", info.Built, payload.Built)
	}
	if payload.GitCommit != info.GitCommit {
		t.Fatalf("expected git commit %q, got %q", info.GitCommit, payload.GitCommit)
	}
}

func TestStatusHandlerIncludesCollectorStatus(t *testing.T) {
	manager := newTestManager(terminal.ManagerOptions{Shell: "/bin/sh"})
	handler := &RestHandler{Manager: manager}
	exitTime := time.Date(2026, 1, 29, 12, 0, 0, 0, time.UTC)
	otel.SetCollectorStatus(otel.CollectorStatus{
		PID:          4242,
		Running:      true,
		StartTime:    time.Date(2026, 1, 29, 11, 0, 0, 0, time.UTC),
		LastExitTime: exitTime,
		LastExitErr:  "boom",
		RestartCount: 2,
		HTTPEndpoint: "127.0.0.1:4318",
	})
	defer otel.ClearCollectorStatus()

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleStatus)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload statusResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.OTelCollectorRunning {
		t.Fatalf("expected collector running")
	}
	if payload.OTelCollectorPID != 4242 {
		t.Fatalf("expected collector pid 4242, got %d", payload.OTelCollectorPID)
	}
	if payload.OTelCollectorHTTPEndpoint != "127.0.0.1:4318" {
		t.Fatalf("expected collector endpoint %q, got %q", "127.0.0.1:4318", payload.OTelCollectorHTTPEndpoint)
	}
	if payload.OTelCollectorRestartCount != 2 {
		t.Fatalf("expected restart count 2, got %d", payload.OTelCollectorRestartCount)
	}
	expectedLastExit := exitTime.Format(time.RFC3339) + ": boom"
	if payload.OTelCollectorLastExit != expectedLastExit {
		t.Fatalf("expected last exit %q, got %q", expectedLastExit, payload.OTelCollectorLastExit)
	}
}

func TestStatusHandlerIncludesTemporalURL(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	handler := &RestHandler{
		Manager:        manager,
		TemporalUIPort: 8233,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("X-Forwarded-Host", "example.com:57417")
	req.Header.Set("X-Forwarded-Proto", "https")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleStatus)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload statusResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.TemporalUIURL != "https://example.com:8233" {
		t.Fatalf("expected temporal url %q, got %q", "https://example.com:8233", payload.TemporalUIURL)
	}
}

func TestStatusHandlerIncludesTemporalDevServerStatus(t *testing.T) {
	manager := newTestManager(terminal.ManagerOptions{Shell: "/bin/sh"})
	handler := &RestHandler{
		Manager:      manager,
		TemporalHost: "localhost:7233",
	}
	temporalcore.SetDevServerStatus(temporalcore.DevServerStatus{
		PID:     2222,
		Running: true,
	})
	defer temporalcore.ClearDevServerStatus()

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleStatus)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload statusResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.TemporalDevServerRunning {
		t.Fatalf("expected temporal dev server running")
	}
	if payload.TemporalDevServerPID != 2222 {
		t.Fatalf("expected temporal dev server pid 2222, got %d", payload.TemporalDevServerPID)
	}
	if payload.TemporalHost != "localhost:7233" {
		t.Fatalf("expected temporal host %q, got %q", "localhost:7233", payload.TemporalHost)
	}
}

func TestWorkflowsEndpointReturnsSummary(t *testing.T) {
	factory := &fakeFactory{}
	temporalClient := &fakeWorkflowQueryClient{
		runID:        "run-42",
		queryResults: make(map[string]workflows.SessionWorkflowState),
	}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:           "/bin/sh",
		PtyFactory:      factory,
		TemporalClient:  temporalClient,
		TemporalEnabled: true,
	})
	useWorkflow := true
	created, err := manager.CreateWithOptions(terminal.CreateOptions{
		AgentID:     testAgentID,
		UseWorkflow: &useWorkflow,
	})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	workflowID := created.ID
	startTime := time.Date(2025, 2, 2, 9, 0, 0, 0, time.UTC)
	bellTime := startTime.Add(5 * time.Minute)
	temporalClient.queryResults[workflowID] = workflows.SessionWorkflowState{
		SessionID: created.ID,
		AgentID:   "Codex",
		CurrentL1: "L1",
		CurrentL2: "L2",
		Status:    workflows.SessionStatusPaused,
		StartTime: startTime,
		BellEvents: []workflows.BellEvent{
			{Timestamp: bellTime, Context: "bell context"},
		},
		TaskEvents: []workflows.TaskEvent{
			{Timestamp: startTime, L1: "L1", L2: "L2"},
		},
	}

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, "/api/workflows", nil)
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleWorkflows)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload []workflowSummary
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(payload))
	}
	got := payload[0]
	if got.SessionID != created.ID {
		t.Fatalf("expected session id %q, got %q", created.ID, got.SessionID)
	}
	if got.WorkflowID != workflowID {
		t.Fatalf("expected workflow id %q, got %q", workflowID, got.WorkflowID)
	}
	if got.WorkflowRunID != temporalClient.runID {
		t.Fatalf("expected workflow run id %q, got %q", temporalClient.runID, got.WorkflowRunID)
	}
	if got.AgentName != "Codex" {
		t.Fatalf("expected agent name %q, got %q", "Codex", got.AgentName)
	}
	if got.CurrentL1 != "L1" || got.CurrentL2 != "L2" {
		t.Fatalf("unexpected tasks: %q/%q", got.CurrentL1, got.CurrentL2)
	}
	if got.Status != workflows.SessionStatusPaused {
		t.Fatalf("expected status %q, got %q", workflows.SessionStatusPaused, got.Status)
	}
	if !got.StartTime.Equal(startTime) {
		t.Fatalf("expected start time %v, got %v", startTime, got.StartTime)
	}
	if len(got.BellEvents) != 1 {
		t.Fatalf("expected 1 bell event, got %d", len(got.BellEvents))
	}
	if got.BellEvents[0].Context != "bell context" || !got.BellEvents[0].Timestamp.Equal(bellTime) {
		t.Fatalf("unexpected bell event: %#v", got.BellEvents[0])
	}
	if len(got.TaskEvents) != 1 {
		t.Fatalf("expected 1 task event, got %d", len(got.TaskEvents))
	}
	if got.TaskEvents[0].L1 != "L1" || got.TaskEvents[0].L2 != "L2" {
		t.Fatalf("unexpected task event: %#v", got.TaskEvents[0])
	}
}

func TestTerminalWorkflowResumeEndpoint(t *testing.T) {
	factory := &fakeFactory{}
	temporalClient := &fakeWorkflowSignalClient{runID: "run-9"}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:           "/bin/sh",
		PtyFactory:      factory,
		TemporalClient:  temporalClient,
		TemporalEnabled: true,
	})
	useWorkflow := true
	created, err := manager.CreateWithOptions(terminal.CreateOptions{
		AgentID:     testAgentID,
		UseWorkflow: &useWorkflow,
	})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/workflow/resume", strings.NewReader(`{"action":"continue"}`))
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}
	if len(temporalClient.signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(temporalClient.signals))
	}
	signal := temporalClient.signals[0]
	if signal.signalName != workflows.ResumeSignalName {
		t.Fatalf("expected resume signal, got %q", signal.signalName)
	}
	payload, ok := signal.payload.(workflows.ResumeSignal)
	if !ok || payload.Action != workflows.ResumeActionContinue {
		t.Fatalf("unexpected resume payload: %#v", signal.payload)
	}
}

func TestTerminalWorkflowResumeEndpointMissingWorkflow(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	created, err := manager.Create(testAgentID, "", "")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/workflow/resume", strings.NewReader(`{"action":"continue"}`))
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", res.Code)
	}
}

func TestTerminalNotifyEndpoint(t *testing.T) {
	factory := &fakeFactory{}
	temporalClient := &fakeWorkflowSignalClient{runID: "run-11"}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:           "/bin/sh",
		PtyFactory:      factory,
		TemporalClient:  temporalClient,
		TemporalEnabled: true,
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex", Shell: "/bin/bash", CLIType: "codex"},
		},
	})
	useWorkflow := true
	created, err := manager.CreateWithOptions(terminal.CreateOptions{
		AgentID:     "codex",
		UseWorkflow: &useWorkflow,
	})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	body := `{"session_id":"` + created.ID + `","agent_id":"codex","agent_name":"Codex","source":"manual","event_type":"plan-L1-wip","occurred_at":"2025-04-01T10:00:00Z","payload":{"plan_file":"plan.org"},"raw":"{}","event_id":"manual:1"}`
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/notify", strings.NewReader(body))
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}
	if len(temporalClient.signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(temporalClient.signals))
	}
	signal := temporalClient.signals[0]
	if signal.signalName != workflows.NotifySignalName {
		t.Fatalf("expected notify signal, got %q", signal.signalName)
	}
	payload, ok := signal.payload.(workflows.NotifySignal)
	if !ok {
		t.Fatalf("unexpected notify payload: %#v", signal.payload)
	}
	if payload.SessionID != created.ID {
		t.Fatalf("expected session id %q, got %q", created.ID, payload.SessionID)
	}
	if payload.AgentID != "codex" {
		t.Fatalf("expected agent id codex, got %q", payload.AgentID)
	}
	if payload.AgentName != "Codex" {
		t.Fatalf("expected agent name Codex, got %q", payload.AgentName)
	}
	if payload.EventType != "plan-L1-wip" {
		t.Fatalf("expected event type plan-L1-wip, got %q", payload.EventType)
	}
	if payload.Source != "manual" {
		t.Fatalf("expected source manual, got %q", payload.Source)
	}
	if payload.EventID != "manual:1" {
		t.Fatalf("expected event id manual:1, got %q", payload.EventID)
	}
	if !payload.Timestamp.Equal(time.Date(2025, 4, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected notify timestamp: %v", payload.Timestamp)
	}
	if !strings.Contains(string(payload.Payload), "\"plan_file\":\"plan.org\"") {
		t.Fatalf("unexpected payload: %s", string(payload.Payload))
	}
}

func TestTerminalNotifyEndpointMissingTerminal(t *testing.T) {
	manager := newTestManager(terminal.ManagerOptions{Shell: "/bin/sh"})
	handler := &RestHandler{Manager: manager}

	req := httptest.NewRequest(http.MethodPost, "/api/sessions/missing/notify", strings.NewReader(`{"session_id":"missing","agent_id":"codex","source":"manual","event_type":"plan-L1-wip"}`))
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", res.Code)
	}
}

func TestTerminalNotifyEndpointMissingWorkflow(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex", Shell: "/bin/bash", CLIType: "codex"},
		},
	})
	created, err := manager.CreateWithOptions(terminal.CreateOptions{AgentID: "codex"})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	body := `{"session_id":"` + created.ID + `","agent_id":"codex","source":"manual","event_type":"plan-L1-wip"}`
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/notify", strings.NewReader(body))
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", res.Code)
	}
}

func TestTerminalNotifyEndpointBadJSON(t *testing.T) {
	factory := &fakeFactory{}
	temporalClient := &fakeWorkflowSignalClient{runID: "run-12"}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:           "/bin/sh",
		PtyFactory:      factory,
		TemporalClient:  temporalClient,
		TemporalEnabled: true,
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex", Shell: "/bin/bash", CLIType: "codex"},
		},
	})
	useWorkflow := true
	created, err := manager.CreateWithOptions(terminal.CreateOptions{
		AgentID:     "codex",
		UseWorkflow: &useWorkflow,
	})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/notify", strings.NewReader("{"))
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.Code)
	}
}

func TestTerminalWorkflowHistoryEndpoint(t *testing.T) {
	factory := &fakeFactory{}
	dataConverter := converter.GetDefaultDataConverter()
	taskPayloads, err := dataConverter.ToPayloads(workflows.UpdateTaskSignal{L1: "L1", L2: "L2"})
	if err != nil {
		t.Fatalf("build task payloads: %v", err)
	}
	bellTime := time.Date(2025, 4, 1, 10, 0, 0, 0, time.UTC)
	bellPayloads, err := dataConverter.ToPayloads(workflows.BellSignal{Timestamp: bellTime, Context: "bell"})
	if err != nil {
		t.Fatalf("build bell payloads: %v", err)
	}
	notifyTime := time.Date(2025, 4, 1, 9, 31, 30, 0, time.UTC)
	notifyPayloads, err := dataConverter.ToPayloads(workflows.NotifySignal{
		Timestamp: notifyTime,
		EventType: "agent-turn-complete",
		Source:    "codex-notify",
	})
	if err != nil {
		t.Fatalf("build notify payloads: %v", err)
	}
	resumePayloads, err := dataConverter.ToPayloads(workflows.ResumeSignal{Action: workflows.ResumeActionContinue})
	if err != nil {
		t.Fatalf("build resume payloads: %v", err)
	}

	eventTime := time.Date(2025, 4, 1, 9, 30, 0, 0, time.UTC)
	events := []*historypb.HistoryEvent{
		{
			EventId:   1,
			EventTime: timestamppb.New(eventTime),
			EventType: enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED,
			Attributes: &historypb.HistoryEvent_WorkflowExecutionSignaledEventAttributes{
				WorkflowExecutionSignaledEventAttributes: &historypb.WorkflowExecutionSignaledEventAttributes{
					SignalName: workflows.UpdateTaskSignalName,
					Input:      taskPayloads,
				},
			},
		},
		{
			EventId:   2,
			EventTime: timestamppb.New(eventTime.Add(time.Minute)),
			EventType: enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED,
			Attributes: &historypb.HistoryEvent_WorkflowExecutionSignaledEventAttributes{
				WorkflowExecutionSignaledEventAttributes: &historypb.WorkflowExecutionSignaledEventAttributes{
					SignalName: workflows.BellSignalName,
					Input:      bellPayloads,
				},
			},
		},
		{
			EventId:   3,
			EventTime: timestamppb.New(eventTime.Add(90 * time.Second)),
			EventType: enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED,
			Attributes: &historypb.HistoryEvent_WorkflowExecutionSignaledEventAttributes{
				WorkflowExecutionSignaledEventAttributes: &historypb.WorkflowExecutionSignaledEventAttributes{
					SignalName: workflows.NotifySignalName,
					Input:      notifyPayloads,
				},
			},
		},
		{
			EventId:   4,
			EventTime: timestamppb.New(eventTime.Add(2 * time.Minute)),
			EventType: enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED,
			Attributes: &historypb.HistoryEvent_WorkflowExecutionSignaledEventAttributes{
				WorkflowExecutionSignaledEventAttributes: &historypb.WorkflowExecutionSignaledEventAttributes{
					SignalName: workflows.ResumeSignalName,
					Input:      resumePayloads,
				},
			},
		},
	}

	temporalClient := &fakeWorkflowHistoryClient{
		runID:  "run-55",
		events: events,
	}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:           "/bin/sh",
		PtyFactory:      factory,
		TemporalClient:  temporalClient,
		TemporalEnabled: true,
	})
	useWorkflow := true
	created, err := manager.CreateWithOptions(terminal.CreateOptions{
		AgentID:     testAgentID,
		UseWorkflow: &useWorkflow,
	})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, terminalPath(created.ID)+"/workflow/history", nil)
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload []workflowHistoryEntry
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 4 {
		t.Fatalf("expected 4 history events, got %d", len(payload))
	}
	if payload[0].Type != "task_update" || payload[0].L1 != "L1" || payload[0].L2 != "L2" {
		t.Fatalf("unexpected task event: %#v", payload[0])
	}
	if payload[1].Type != "bell" || payload[1].Context != "bell" {
		t.Fatalf("unexpected bell event: %#v", payload[1])
	}
	if !payload[1].Timestamp.Equal(bellTime) {
		t.Fatalf("expected bell timestamp %v, got %v", bellTime, payload[1].Timestamp)
	}
	if payload[2].Type != "notify" || payload[2].Context != "agent-turn-complete" {
		t.Fatalf("unexpected notify event: %#v", payload[2])
	}
	if !payload[2].Timestamp.Equal(notifyTime) {
		t.Fatalf("expected notify timestamp %v, got %v", notifyTime, payload[2].Timestamp)
	}
	if payload[3].Type != "resume" || payload[3].Action != workflows.ResumeActionContinue {
		t.Fatalf("unexpected resume event: %#v", payload[3])
	}
}

func TestStatusHandlerIncludesGitInfo(t *testing.T) {
	manager := newTestManager(terminal.ManagerOptions{Shell: "/bin/sh"})
	handler := &RestHandler{
		Manager:   manager,
		GitOrigin: "origin",
		GitBranch: "main",
	}
	handler.setGitBranch("feature")

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleStatus)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload statusResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.GitOrigin != "origin" {
		t.Fatalf("expected origin, got %q", payload.GitOrigin)
	}
	if payload.GitBranch != "feature" {
		t.Fatalf("expected branch feature, got %q", payload.GitBranch)
	}
}

func TestTerminalOutputEndpoint(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	created, err := manager.Create(testAgentID, "", "")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	factory.mu.Lock()
	pty := factory.ptys[0]
	factory.mu.Unlock()
	if _, err := pty.Write([]byte("hello\n")); err != nil {
		t.Fatalf("write pty: %v", err)
	}

	if !waitForOutput(created) {
		t.Fatalf("expected output buffer to receive data")
	}

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, terminalPath(created.ID)+"/output", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload terminalOutputResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !containsLine(payload.Lines, "hello") {
		t.Fatalf("expected output lines to contain hello, got %v", payload.Lines)
	}
}

func TestTerminalHistoryEndpoint(t *testing.T) {
	factory := &fakeFactory{}
	logDir := t.TempDir()
	manager := newTestManager(terminal.ManagerOptions{
		Shell:         "/bin/sh",
		PtyFactory:    factory,
		SessionLogDir: logDir,
	})
	created, err := manager.Create(testAgentID, "", "")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	factory.mu.Lock()
	pty := factory.ptys[0]
	factory.mu.Unlock()
	if _, err := pty.Write([]byte("hello\n")); err != nil {
		t.Fatalf("write pty: %v", err)
	}

	if !waitForOutput(created) {
		t.Fatalf("expected output buffer to receive data")
	}
	if !waitForHistoryCursor(manager, created.ID, int64(len("hello\n")), 2*time.Second) {
		t.Fatalf("expected history cursor to advance")
	}

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, terminalPath(created.ID)+"/history?lines=5", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload terminalOutputResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !containsLine(payload.Lines, "hello") {
		t.Fatalf("expected history lines to contain hello, got %v", payload.Lines)
	}
	if payload.Cursor == nil {
		t.Fatalf("expected cursor to be set when session persistence is enabled")
	}
}

func TestTerminalHistoryPagination(t *testing.T) {
	factory := &fakeFactory{}
	logDir := t.TempDir()
	manager := newTestManager(terminal.ManagerOptions{
		Shell:         "/bin/sh",
		PtyFactory:    factory,
		SessionLogDir: logDir,
	})
	created, err := manager.Create(testAgentID, "", "")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	factory.mu.Lock()
	pty := factory.ptys[0]
	factory.mu.Unlock()
	payloads := []string{"one", "two", "three", "four", "five"}
	for _, entry := range payloads {
		if _, err := pty.Write([]byte(entry + "\n")); err != nil {
			t.Fatalf("write pty: %v", err)
		}
	}

	if !waitForOutput(created) {
		t.Fatalf("expected output buffer to receive data")
	}
	if !waitForHistoryCursor(manager, created.ID, int64(len("one\n")*len(payloads)), 2*time.Second) {
		t.Fatalf("expected history cursor to advance")
	}

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, terminalPath(created.ID)+"/history?lines=2", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var page terminalOutputResponse
	if err := json.NewDecoder(res.Body).Decode(&page); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(page.Lines) < 2 || page.Lines[len(page.Lines)-2] != "four" || page.Lines[len(page.Lines)-1] != "five" {
		t.Fatalf("unexpected history page: %v", page.Lines)
	}
	if page.Cursor == nil {
		t.Fatalf("expected cursor in history response")
	}

	req = httptest.NewRequest(http.MethodGet, terminalPath(created.ID)+"/history?lines=2&before_cursor="+strconv.FormatInt(*page.Cursor, 10), nil)
	req.Header.Set("Authorization", "Bearer secret")
	res = httptest.NewRecorder()
	restHandler("secret", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var pageWithCursor terminalOutputResponse
	if err := json.NewDecoder(res.Body).Decode(&pageWithCursor); err != nil {
		t.Fatalf("decode paged response: %v", err)
	}
	if pageWithCursor.Cursor == nil {
		t.Fatalf("expected cursor in paged response")
	}
	if *pageWithCursor.Cursor >= *page.Cursor {
		t.Fatalf("expected paged cursor to move backward")
	}

	req = httptest.NewRequest(http.MethodGet, terminalPath(created.ID)+"/history?lines=2&before_cursor="+strconv.FormatInt(*pageWithCursor.Cursor, 10), nil)
	req.Header.Set("Authorization", "Bearer secret")
	res = httptest.NewRecorder()
	restHandler("secret", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var older terminalOutputResponse
	if err := json.NewDecoder(res.Body).Decode(&older); err != nil {
		t.Fatalf("decode older response: %v", err)
	}
	if len(older.Lines) < 2 || older.Lines[len(older.Lines)-2] != "two" || older.Lines[len(older.Lines)-1] != "three" {
		t.Fatalf("unexpected older page: %v", older.Lines)
	}
}

func TestTerminalInputHistoryEndpoint(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	created, err := manager.Create(testAgentID, "", "")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	created.RecordInput("one")
	created.RecordInput("two")

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, terminalPath(created.ID)+"/input-history?limit=1", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload []inputHistoryEntry
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 || payload[0].Command != "two" {
		t.Fatalf("expected last command, got %v", payload)
	}
	if payload[0].Timestamp.IsZero() {
		t.Fatalf("expected timestamp to be set")
	}
}

func TestTerminalInputHistoryPostEndpoint(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	created, err := manager.Create(testAgentID, "", "")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(
		http.MethodPost,
		terminalPath(created.ID)+"/input-history",
		strings.NewReader(`{"command":"echo hi"}`),
	)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}

	getReq := httptest.NewRequest(
		http.MethodGet,
		terminalPath(created.ID)+"/input-history?limit=1",
		nil,
	)
	getReq.Header.Set("Authorization", "Bearer secret")
	getRes := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminal)(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRes.Code)
	}

	var payload []inputHistoryEntry
	if err := json.NewDecoder(getRes.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 || payload[0].Command != "echo hi" {
		t.Fatalf("unexpected history payload: %v", payload)
	}
}

func TestCreateTerminalWithoutAgent(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	handler := &RestHandler{Manager: manager}

	req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{"title":"plain","role":"shell"}`))
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminals)(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.Code)
	}
}

func TestCreateTerminalWorkflowFlag(t *testing.T) {
	factory := &fakeFactory{}
	temporalClient := &fakeWorkflowSignalClient{runID: "run-1"}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:           "/bin/sh",
		PtyFactory:      factory,
		TemporalClient:  temporalClient,
		TemporalEnabled: true,
	})
	handler := &RestHandler{Manager: manager}

	req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{"agent":"codex"}`))
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminals)(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.Code)
	}

	var payload terminalSummary
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	session, ok := manager.Get(payload.ID)
	if !ok {
		t.Fatalf("expected session %q", payload.ID)
	}
	workflowID, runID, ok := session.WorkflowIdentifiers()
	if !ok {
		t.Fatalf("expected workflow identifiers")
	}
	if workflowID != payload.ID {
		t.Fatalf("expected workflow id %q, got %q", payload.ID, workflowID)
	}
	if runID != temporalClient.runID {
		t.Fatalf("expected run id %q, got %q", temporalClient.runID, runID)
	}
	if err := manager.Delete(payload.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{"agent":"codex","workflow":true}`))
	req.Header.Set("Authorization", "Bearer secret")
	res = httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminals)(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.Code)
	}

	var payloadExplicit terminalSummary
	if err := json.NewDecoder(res.Body).Decode(&payloadExplicit); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	explicitSession, ok := manager.Get(payloadExplicit.ID)
	if !ok {
		t.Fatalf("expected session %q", payloadExplicit.ID)
	}
	if _, _, ok := explicitSession.WorkflowIdentifiers(); !ok {
		t.Fatalf("expected workflow identifiers for explicit enable")
	}
	if err := manager.Delete(payloadExplicit.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{"agent":"codex","workflow":false}`))
	req.Header.Set("Authorization", "Bearer secret")
	res = httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminals)(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.Code)
	}

	var payloadDisabled terminalSummary
	if err := json.NewDecoder(res.Body).Decode(&payloadDisabled); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	disabledSession, ok := manager.Get(payloadDisabled.ID)
	if !ok {
		t.Fatalf("expected session %q", payloadDisabled.ID)
	}
	if _, _, ok := disabledSession.WorkflowIdentifiers(); ok {
		t.Fatalf("expected no workflow identifiers")
	}
	if err := manager.Delete(payloadDisabled.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}
}

func TestCreateTerminalWithAgent(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	agentTOML := "name = \"Codex\"\nshell = \"/bin/zsh\"\ncli_type = \"codex\"\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "codex.toml"), []byte(agentTOML), 0644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:    "Codex",
				Shell:   "/bin/zsh",
				CLIType: "codex",
			},
		},
		AgentsDir: agentsDir,
	})
	handler := &RestHandler{Manager: manager}

	req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{"title":"ignored","role":"shell","agent":"codex"}`))
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminals)(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.Code)
	}

	var payload terminalSummary
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Title != "Codex" {
		t.Fatalf("expected title Codex, got %q", payload.Title)
	}
	if payload.ID == "" {
		t.Fatalf("missing id")
	}
	defer func() {
		_ = manager.Delete(payload.ID)
	}()

	factory.mu.Lock()
	defer factory.mu.Unlock()
	if len(factory.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(factory.commands))
	}
	if factory.commands[0] != "codex" {
		t.Fatalf("expected codex, got %q", factory.commands[0])
	}
}

func TestCreateTerminalUsesAgentWorkflowDefault(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	agentTOML := "name = \"Codex\"\nshell = \"/bin/zsh\"\ncli_type = \"codex\"\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "codex.toml"), []byte(agentTOML), 0644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	factory := &fakeFactory{}
	temporalClient := &fakeWorkflowSignalClient{runID: "run-2"}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:           "/bin/sh",
		PtyFactory:      factory,
		TemporalClient:  temporalClient,
		TemporalEnabled: true,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:    "Codex",
				Shell:   "/bin/zsh",
				CLIType: "codex",
			},
		},
		AgentsDir: agentsDir,
	})
	handler := &RestHandler{Manager: manager}

	req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{"agent":"codex"}`))
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminals)(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.Code)
	}

	var payload terminalSummary
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	session, ok := manager.Get(payload.ID)
	if !ok {
		t.Fatalf("expected session %q", payload.ID)
	}
	defer func() {
		_ = manager.Delete(payload.ID)
	}()
	if _, _, ok := session.WorkflowIdentifiers(); !ok {
		t.Fatalf("expected workflow identifiers")
	}
}

func TestCreateTerminalDuplicateAgent(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	agentTOML := "name = \"Codex\"\nshell = \"/bin/zsh\"\ncli_type = \"codex\"\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "codex.toml"), []byte(agentTOML), 0644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:    "Codex",
				Shell:   "/bin/zsh",
				CLIType: "codex",
			},
		},
		AgentsDir: agentsDir,
	})
	handler := &RestHandler{Manager: manager}

	req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{"agent":"codex"}`))
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminals)(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.Code)
	}

	var created terminalSummary
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("missing id")
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	req = httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{"agent":"codex"}`))
	req.Header.Set("Authorization", "Bearer secret")
	res = httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminals)(res, req)
	if res.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", res.Code)
	}

	var payload errorResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.SessionID != created.ID {
		t.Fatalf("expected session_id %q, got %q", created.ID, payload.SessionID)
	}
	if payload.Message == "" {
		t.Fatalf("expected error message")
	}
	if payload.Code != "conflict" {
		t.Fatalf("expected error code %q, got %q", "conflict", payload.Code)
	}
}

func TestListTerminalsIncludesLLMMetadata(t *testing.T) {
	factory := &fakeFactory{}
	nonSingleton := false
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:      "Codex",
				Shell:     "/bin/zsh",
				CLIType:   "codex",
				LLMModel:  "default",
				Singleton: &nonSingleton,
			},
		},
	})

	defaultSession, err := manager.Create(testAgentID, "build", "plain")
	if err != nil {
		t.Fatalf("create default session: %v", err)
	}
	agentSession, err := manager.Create("codex", "build", "ignored")
	if err != nil {
		t.Fatalf("create agent session: %v", err)
	}
	defer func() {
		_ = manager.Delete(defaultSession.ID)
		_ = manager.Delete(agentSession.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminals)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload []terminalSummary
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	var agentSummary *terminalSummary
	for i := range payload {
		if payload[i].ID == agentSession.ID {
			agentSummary = &payload[i]
			break
		}
	}
	if agentSummary == nil {
		t.Fatalf("expected agent session in list")
	}
	if agentSummary.LLMType != "codex" {
		t.Fatalf("expected llm_type codex, got %q", agentSummary.LLMType)
	}
	if agentSummary.LLMModel != "default" {
		t.Fatalf("expected llm_model default, got %q", agentSummary.LLMModel)
	}
}

func TestListTerminalsIncludesPromptFiles(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})

	session, err := manager.Create(testAgentID, "build", "plain")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	session.PromptFiles = []string{"main.tmpl", "fragment.txt"}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminals)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload []terminalSummary
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	var summary *terminalSummary
	for i := range payload {
		if payload[i].ID == session.ID {
			summary = &payload[i]
			break
		}
	}
	if summary == nil {
		t.Fatalf("expected session in list")
	}
	if len(summary.PromptFiles) != 2 {
		t.Fatalf("expected 2 prompt files, got %d", len(summary.PromptFiles))
	}
	if summary.PromptFiles[0] != "main.tmpl" || summary.PromptFiles[1] != "fragment.txt" {
		t.Fatalf("unexpected prompt files: %#v", summary.PromptFiles)
	}
}

func TestAgentsEndpoint(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:        "Codex",
				Shell:       "/bin/zsh",
				CLIType:     "codex",
				LLMModel:    "default",
				UseWorkflow: boolPtr(true),
			},
			"copilot": {
				Name:        "Copilot",
				Shell:       "/bin/bash",
				CLIType:     "copilot",
				LLMModel:    "default",
				UseWorkflow: boolPtr(false),
			},
		},
	})
	created, err := manager.Create("codex", "shell", "Codex")
	if err != nil {
		t.Fatalf("create codex session: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleAgents)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload []agentSummary
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(payload))
	}
	var codex *agentSummary
	var copilot *agentSummary
	for i := range payload {
		if payload[i].ID == "codex" {
			codex = &payload[i]
		}
		if payload[i].ID == "copilot" {
			copilot = &payload[i]
		}
	}
	if codex == nil || copilot == nil {
		t.Fatalf("missing expected agents")
	}
	if !codex.Running {
		t.Fatalf("expected codex to be running")
	}
	if codex.SessionID != created.ID {
		t.Fatalf("expected codex terminal id %q, got %q", created.ID, codex.SessionID)
	}
	if !codex.UseWorkflow {
		t.Fatalf("expected codex use_workflow to be true")
	}
	if copilot.Running {
		t.Fatalf("expected copilot to be stopped")
	}
	if copilot.SessionID != "" {
		t.Fatalf("expected copilot terminal id to be empty, got %q", copilot.SessionID)
	}
	if copilot.UseWorkflow {
		t.Fatalf("expected copilot use_workflow to be false")
	}
}

func TestAgentInputEndpoint(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:  "Codex",
				Shell: "/bin/bash",
			},
		},
	})
	created, err := manager.Create("codex", "shell", "Codex")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, "/api/agents/Codex/input", strings.NewReader("hello"))
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleAgentInput)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload agentInputResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Bytes != len("hello") {
		t.Fatalf("expected %d bytes, got %d", len("hello"), payload.Bytes)
	}
}

func TestSkillsEndpoint(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "git-workflows")
	if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0755); err != nil {
		t.Fatalf("mkdir references: %v", err)
	}

	manager := newTestManager(terminal.ManagerOptions{
		Skills: map[string]*skill.Skill{
			"git-workflows": {
				Name:        "git-workflows",
				Description: "Helpful git workflows",
				Path:        skillDir,
				License:     "MIT",
			},
		},
	})

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleSkills)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload []skillSummary
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(payload))
	}
	if payload[0].Name != "git-workflows" {
		t.Fatalf("unexpected skill name: %q", payload[0].Name)
	}
	if !payload[0].HasScripts || !payload[0].HasReferences || payload[0].HasAssets {
		t.Fatalf("unexpected directory flags: %+v", payload[0])
	}
}

func TestSkillsEndpointFiltersByAgent(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, "git-workflows")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	reviewDir := filepath.Join(root, "code-review")
	if err := os.MkdirAll(reviewDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	manager := newTestManager(terminal.ManagerOptions{
		Agents: map[string]agent.Agent{
			"codex": {
				Name:   "Codex",
				Shell:  "/bin/bash",
				Skills: []string{"git-workflows"},
			},
		},
		Skills: map[string]*skill.Skill{
			"git-workflows": {
				Name:        "git-workflows",
				Description: "Helpful git workflows",
				Path:        gitDir,
			},
			"code-review": {
				Name:        "code-review",
				Description: "Review code carefully",
				Path:        reviewDir,
			},
		},
	})

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, "/api/skills?agent=codex", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleSkills)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload []skillSummary
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 || payload[0].Name != "git-workflows" {
		t.Fatalf("unexpected skills response: %+v", payload)
	}
}

func TestSkillsEndpointUnknownAgent(t *testing.T) {
	manager := newTestManager(terminal.ManagerOptions{})
	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, "/api/skills?agent=missing", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleSkills)(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", res.Code)
	}
}

func TestPlansEndpointReturnsEmptyList(t *testing.T) {
	root := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	handler := &RestHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/plans", nil)
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handlePlansList)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload plansListResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Plans) != 0 {
		t.Fatalf("expected empty plans list, got %d", len(payload.Plans))
	}
}

func TestPlansEndpointReturnsSortedPlans(t *testing.T) {
	root := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	plansDir := filepath.Join(root, ".gestalt", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir plans dir: %v", err)
	}

	alpha := `#+TITLE: Alpha
#+SUBTITLE: First
#+DATE: 2026-01-02
* TODO [#A] Alpha L1
** TODO [#B] Alpha L2
`
	beta := `#+TITLE: Beta
#+SUBTITLE: Second
#+DATE: 2026-01-03
* WIP [#A] Beta L1
** DONE [#C] Beta L2
`
	if err := os.WriteFile(filepath.Join(plansDir, "2026-01-02-alpha.org"), []byte(alpha), 0o644); err != nil {
		t.Fatalf("write alpha plan: %v", err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "2026-01-03-beta.org"), []byte(beta), 0o644); err != nil {
		t.Fatalf("write beta plan: %v", err)
	}

	handler := &RestHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/plans", nil)
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handlePlansList)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload plansListResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Plans) != 2 {
		t.Fatalf("expected 2 plans, got %d", len(payload.Plans))
	}
	if payload.Plans[0].Filename != "2026-01-03-beta.org" {
		t.Fatalf("expected beta plan first, got %q", payload.Plans[0].Filename)
	}
	if payload.Plans[0].Title != "Beta" {
		t.Fatalf("expected beta title, got %q", payload.Plans[0].Title)
	}
	if payload.Plans[0].L1Count != 1 || payload.Plans[0].L2Count != 1 {
		t.Fatalf("unexpected beta counts: L1 %d L2 %d", payload.Plans[0].L1Count, payload.Plans[0].L2Count)
	}
	if payload.Plans[0].PriorityC != 1 {
		t.Fatalf("expected beta priority C to be 1")
	}
}

func TestTerminalBellEndpointReturnsNoContent(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	created, err := manager.Create(testAgentID, "", "")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/bell", nil)
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}
}

func TestTerminalBellEndpointMissingSession(t *testing.T) {
	manager := newTestManager(terminal.ManagerOptions{Shell: "/bin/sh"})
	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/unknown/bell", nil)
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", res.Code)
	}
}

func waitForOutput(session *terminal.Session) bool {
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(session.OutputLines()) > 0 {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func waitForHistoryCursor(manager *terminal.Manager, id string, min int64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cursor, err := manager.HistoryCursor(id)
		if err != nil {
			return false
		}
		if cursor != nil && *cursor >= min {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func containsLine(lines []string, target string) bool {
	for _, line := range lines {
		if line == target {
			return true
		}
	}
	return false
}

func TestParseTerminalPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		id      string
		action  terminalPathAction
		wantErr bool
		status  int
	}{
		{name: "terminal", path: "/api/sessions/123", id: "123", action: terminalPathTerminal},
		{name: "terminal-trailing-slash", path: "/api/sessions/123/", id: "123", action: terminalPathTerminal},
		{name: "output", path: "/api/sessions/123/output", id: "123", action: terminalPathOutput},
		{name: "output-trailing-slash", path: "/api/sessions/123/output/", id: "123", action: terminalPathOutput},
		{name: "history", path: "/api/sessions/123/history", id: "123", action: terminalPathHistory},
		{name: "history-trailing-slash", path: "/api/sessions/123/history/", id: "123", action: terminalPathHistory},
		{name: "input-history", path: "/api/sessions/123/input-history", id: "123", action: terminalPathInputHistory},
		{name: "input-history-trailing-slash", path: "/api/sessions/123/input-history/", id: "123", action: terminalPathInputHistory},
		{name: "workflow-resume", path: "/api/sessions/123/workflow/resume", id: "123", action: terminalPathWorkflowResume},
		{name: "workflow-resume-trailing-slash", path: "/api/sessions/123/workflow/resume/", id: "123", action: terminalPathWorkflowResume},
		{name: "workflow-history", path: "/api/sessions/123/workflow/history", id: "123", action: terminalPathWorkflowHistory},
		{name: "workflow-history-trailing-slash", path: "/api/sessions/123/workflow/history/", id: "123", action: terminalPathWorkflowHistory},
		{name: "missing-prefix", path: "/api/terminal/123", wantErr: true, status: http.StatusNotFound},
		{name: "empty-id", path: "/api/sessions/", wantErr: true, status: http.StatusBadRequest},
		{name: "empty-id-output", path: "/api/sessions//output", wantErr: true, status: http.StatusBadRequest},
		{name: "unknown-action", path: "/api/sessions/123/extra", wantErr: true, status: http.StatusNotFound},
		{name: "workflow-missing-action", path: "/api/sessions/123/workflow", wantErr: true, status: http.StatusNotFound},
		{name: "workflow-unknown-action", path: "/api/sessions/123/workflow/extra", wantErr: true, status: http.StatusNotFound},
		{name: "extra-segments", path: "/api/sessions/123/output/extra", wantErr: true, status: http.StatusNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id, action, err := parseTerminalPath(test.path)
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if err.Status != test.status {
					t.Fatalf("expected status %d, got %d", test.status, err.Status)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != test.id {
				t.Fatalf("expected id %q, got %q", test.id, id)
			}
			if action != test.action {
				t.Fatalf("expected action %v, got %v", test.action, action)
			}
		})
	}
}
