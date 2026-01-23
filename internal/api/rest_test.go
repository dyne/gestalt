package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/plan"
	"gestalt/internal/skill"
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

type fakeWorkflowSignalClient struct {
	runID   string
	signals []workflowSignalRecord
}

func (client *fakeWorkflowSignalClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	return &fakeWorkflowRun{workflowID: options.ID, runID: client.runID}, nil
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

	restHandler("secret", handler.handleStatus)(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.Code)
	}
}

func TestStatusHandlerReturnsCount(t *testing.T) {
	factory := &fakeFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	created, err := manager.Create("", "", "")
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

	restHandler("secret", handler.handleStatus)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload statusResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.TerminalCount != 1 {
		t.Fatalf("expected 1 terminal, got %d", payload.TerminalCount)
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

func TestStatusHandlerIncludesTemporalURL(t *testing.T) {
	factory := &fakeFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
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

	restHandler("secret", handler.handleStatus)(res, req)
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

func TestMetricsEndpointReturnsText(t *testing.T) {
	handler := &RestHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	res := httptest.NewRecorder()

	restHandler("", handler.handleMetrics)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	if contentType := res.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("unexpected content type: %s", contentType)
	}
	body := res.Body.String()
	if !strings.Contains(body, "gestalt_workflows_started_total") {
		t.Fatalf("expected workflow metrics, got %q", body)
	}
}

func TestEventDebugEndpointReturnsBuses(t *testing.T) {
	bus := event.NewBus[string](context.Background(), event.BusOptions{
		Name: "debug_bus",
	})
	events, cancel := bus.Subscribe()
	if events == nil {
		t.Fatal("expected subscription channel")
	}
	t.Cleanup(cancel)
	t.Cleanup(bus.Close)

	handler := &RestHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/events/debug", nil)
	res := httptest.NewRecorder()

	restHandler("", handler.handleEventDebug)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	var payload []eventBusDebug
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var found *eventBusDebug
	for index := range payload {
		if payload[index].Name == "debug_bus" {
			found = &payload[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected debug_bus in response: %#v", payload)
	}
	if found.UnfilteredSubscribers != 1 {
		t.Fatalf("expected 1 unfiltered subscriber, got %d", found.UnfilteredSubscribers)
	}
	if found.FilteredSubscribers != 0 {
		t.Fatalf("expected 0 filtered subscribers, got %d", found.FilteredSubscribers)
	}
}

func TestWorkflowsEndpointReturnsSummary(t *testing.T) {
	factory := &fakeFactory{}
	temporalClient := &fakeWorkflowQueryClient{
		runID:        "run-42",
		queryResults: make(map[string]workflows.SessionWorkflowState),
	}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:           "/bin/sh",
		PtyFactory:      factory,
		TemporalClient:  temporalClient,
		TemporalEnabled: true,
	})
	useWorkflow := true
	created, err := manager.CreateWithOptions(terminal.CreateOptions{
		UseWorkflow: &useWorkflow,
	})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	workflowID := "session-" + created.ID
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

	restHandler("", handler.handleWorkflows)(res, req)
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
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:           "/bin/sh",
		PtyFactory:      factory,
		TemporalClient:  temporalClient,
		TemporalEnabled: true,
	})
	useWorkflow := true
	created, err := manager.CreateWithOptions(terminal.CreateOptions{
		UseWorkflow: &useWorkflow,
	})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, "/api/terminals/"+created.ID+"/workflow/resume", strings.NewReader(`{"action":"continue"}`))
	res := httptest.NewRecorder()

	restHandler("", handler.handleTerminal)(res, req)
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
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	created, err := manager.Create("", "", "")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, "/api/terminals/"+created.ID+"/workflow/resume", strings.NewReader(`{"action":"continue"}`))
	res := httptest.NewRecorder()

	restHandler("", handler.handleTerminal)(res, req)
	if res.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", res.Code)
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
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:           "/bin/sh",
		PtyFactory:      factory,
		TemporalClient:  temporalClient,
		TemporalEnabled: true,
	})
	useWorkflow := true
	created, err := manager.CreateWithOptions(terminal.CreateOptions{
		UseWorkflow: &useWorkflow,
	})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, "/api/terminals/"+created.ID+"/workflow/history", nil)
	res := httptest.NewRecorder()

	restHandler("", handler.handleTerminal)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload []workflowHistoryEntry
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 3 {
		t.Fatalf("expected 3 history events, got %d", len(payload))
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
	if payload[2].Type != "resume" || payload[2].Action != workflows.ResumeActionContinue {
		t.Fatalf("unexpected resume event: %#v", payload[2])
	}
}

func TestStatusHandlerIncludesGitInfo(t *testing.T) {
	manager := terminal.NewManager(terminal.ManagerOptions{Shell: "/bin/sh"})
	handler := &RestHandler{
		Manager:   manager,
		GitOrigin: "origin",
		GitBranch: "main",
	}
	handler.setGitBranch("feature")

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleStatus)(res, req)
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
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	created, err := manager.Create("", "", "")
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
	req := httptest.NewRequest(http.MethodGet, "/api/terminals/"+created.ID+"/output", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleTerminal)(res, req)
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
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	created, err := manager.Create("", "", "")
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
	req := httptest.NewRequest(http.MethodGet, "/api/terminals/"+created.ID+"/history?lines=5", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleTerminal)(res, req)
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
}

func TestTerminalInputHistoryEndpoint(t *testing.T) {
	factory := &fakeFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	created, err := manager.Create("", "", "")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	created.RecordInput("one")
	created.RecordInput("two")

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, "/api/terminals/"+created.ID+"/input-history?limit=1", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleTerminal)(res, req)
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
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	created, err := manager.Create("", "", "")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/terminals/"+created.ID+"/input-history",
		strings.NewReader(`{"command":"echo hi"}`),
	)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleTerminal)(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}

	getReq := httptest.NewRequest(
		http.MethodGet,
		"/api/terminals/"+created.ID+"/input-history?limit=1",
		nil,
	)
	getReq.Header.Set("Authorization", "Bearer secret")
	getRes := httptest.NewRecorder()

	restHandler("secret", handler.handleTerminal)(getRes, getReq)
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
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	handler := &RestHandler{Manager: manager}

	req := httptest.NewRequest(http.MethodPost, "/api/terminals", strings.NewReader(`{"title":"plain","role":"shell"}`))
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleTerminals)(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.Code)
	}

	var payload terminalSummary
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Title != "plain" {
		t.Fatalf("title mismatch: %q", payload.Title)
	}
	if payload.Role != "shell" {
		t.Fatalf("role mismatch: %q", payload.Role)
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
	if factory.commands[0] != "/bin/sh" {
		t.Fatalf("expected /bin/sh, got %q", factory.commands[0])
	}
}

func TestCreateTerminalWorkflowFlag(t *testing.T) {
	factory := &fakeFactory{}
	temporalClient := &fakeWorkflowSignalClient{runID: "run-1"}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:           "/bin/sh",
		PtyFactory:      factory,
		TemporalClient:  temporalClient,
		TemporalEnabled: true,
	})
	handler := &RestHandler{Manager: manager}

	req := httptest.NewRequest(http.MethodPost, "/api/terminals", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleTerminals)(res, req)
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
	workflowID, runID, ok := session.WorkflowIdentifiers()
	if !ok {
		t.Fatalf("expected workflow identifiers")
	}
	if workflowID != "session-"+payload.ID {
		t.Fatalf("expected workflow id %q, got %q", "session-"+payload.ID, workflowID)
	}
	if runID != temporalClient.runID {
		t.Fatalf("expected run id %q, got %q", temporalClient.runID, runID)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/terminals", strings.NewReader(`{"workflow":true}`))
	req.Header.Set("Authorization", "Bearer secret")
	res = httptest.NewRecorder()

	restHandler("secret", handler.handleTerminals)(res, req)
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
	defer func() {
		_ = manager.Delete(payloadExplicit.ID)
	}()
	if _, _, ok := explicitSession.WorkflowIdentifiers(); !ok {
		t.Fatalf("expected workflow identifiers for explicit enable")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/terminals", strings.NewReader(`{"workflow":false}`))
	req.Header.Set("Authorization", "Bearer secret")
	res = httptest.NewRecorder()

	restHandler("secret", handler.handleTerminals)(res, req)
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
	defer func() {
		_ = manager.Delete(payloadDisabled.ID)
	}()
	if _, _, ok := disabledSession.WorkflowIdentifiers(); ok {
		t.Fatalf("expected no workflow identifiers")
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
	manager := terminal.NewManager(terminal.ManagerOptions{
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

	req := httptest.NewRequest(http.MethodPost, "/api/terminals", strings.NewReader(`{"title":"ignored","role":"shell","agent":"codex"}`))
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleTerminals)(res, req)
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
	if factory.commands[0] != "/bin/zsh" {
		t.Fatalf("expected /bin/zsh, got %q", factory.commands[0])
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
	manager := terminal.NewManager(terminal.ManagerOptions{
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

	req := httptest.NewRequest(http.MethodPost, "/api/terminals", strings.NewReader(`{"agent":"codex"}`))
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleTerminals)(res, req)
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
	manager := terminal.NewManager(terminal.ManagerOptions{
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

	req := httptest.NewRequest(http.MethodPost, "/api/terminals", strings.NewReader(`{"agent":"codex"}`))
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleTerminals)(res, req)
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

	req = httptest.NewRequest(http.MethodPost, "/api/terminals", strings.NewReader(`{"agent":"codex"}`))
	req.Header.Set("Authorization", "Bearer secret")
	res = httptest.NewRecorder()

	restHandler("secret", handler.handleTerminals)(res, req)
	if res.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", res.Code)
	}

	var payload errorResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.TerminalID != created.ID {
		t.Fatalf("expected terminal_id %q, got %q", created.ID, payload.TerminalID)
	}
	if payload.Error == "" {
		t.Fatalf("expected error message")
	}
}

func TestListTerminalsIncludesLLMMetadata(t *testing.T) {
	factory := &fakeFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:     "Codex",
				Shell:    "/bin/zsh",
				CLIType:  "codex",
				LLMModel: "default",
			},
		},
	})

	defaultSession, err := manager.Create("", "build", "plain")
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
	req := httptest.NewRequest(http.MethodGet, "/api/terminals", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleTerminals)(res, req)
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
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})

	session, err := manager.Create("", "build", "plain")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	session.PromptFiles = []string{"main.tmpl", "fragment.txt"}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, "/api/terminals", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleTerminals)(res, req)
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
	manager := terminal.NewManager(terminal.ManagerOptions{
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

	restHandler("secret", handler.handleAgents)(res, req)
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
	if codex.TerminalID != created.ID {
		t.Fatalf("expected codex terminal id %q, got %q", created.ID, codex.TerminalID)
	}
	if !codex.UseWorkflow {
		t.Fatalf("expected codex use_workflow to be true")
	}
	if copilot.Running {
		t.Fatalf("expected copilot to be stopped")
	}
	if copilot.TerminalID != "" {
		t.Fatalf("expected copilot terminal id to be empty, got %q", copilot.TerminalID)
	}
	if copilot.UseWorkflow {
		t.Fatalf("expected copilot use_workflow to be false")
	}
}

func TestAgentInputEndpoint(t *testing.T) {
	factory := &fakeFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
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

	restHandler("secret", handler.handleAgentInput)(res, req)
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

	manager := terminal.NewManager(terminal.ManagerOptions{
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

	restHandler("secret", handler.handleSkills)(res, req)
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

	manager := terminal.NewManager(terminal.ManagerOptions{
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

	restHandler("secret", handler.handleSkills)(res, req)
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
	manager := terminal.NewManager(terminal.ManagerOptions{})
	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, "/api/skills?agent=missing", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleSkills)(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", res.Code)
	}
}

func TestSkillEndpoint(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "git-workflows")
	if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0755); err != nil {
		t.Fatalf("mkdir references: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillDir, "assets"), 0755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "scripts", "run.sh"), []byte("echo hi"), 0644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "references", "guide.md"), []byte("guide"), 0644); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "assets", "icon.png"), []byte("png"), 0644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	manager := terminal.NewManager(terminal.ManagerOptions{
		Skills: map[string]*skill.Skill{
			"git-workflows": {
				Name:          "git-workflows",
				Description:   "Helpful git workflows",
				License:       "MIT",
				Compatibility: ">=1.0",
				Metadata:      map[string]any{"owner": "dyne"},
				AllowedTools:  []string{"bash"},
				Path:          skillDir,
				Content:       "# Git Workflows\n",
			},
		},
	})

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, "/api/skills/git-workflows", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleSkill)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload skillDetail
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Name != "git-workflows" {
		t.Fatalf("unexpected name: %q", payload.Name)
	}
	if payload.Content == "" {
		t.Fatalf("expected content to be set")
	}
	if len(payload.Scripts) != 1 || payload.Scripts[0] != "run.sh" {
		t.Fatalf("unexpected scripts: %v", payload.Scripts)
	}
	if len(payload.References) != 1 || payload.References[0] != "guide.md" {
		t.Fatalf("unexpected references: %v", payload.References)
	}
	if len(payload.Assets) != 1 || payload.Assets[0] != "icon.png" {
		t.Fatalf("unexpected assets: %v", payload.Assets)
	}
}

func TestSkillEndpointMissing(t *testing.T) {
	manager := terminal.NewManager(terminal.ManagerOptions{})
	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, "/api/skills/missing", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleSkill)(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", res.Code)
	}
}

func TestLogsEndpointDefaults(t *testing.T) {
	buffer := logging.NewLogBuffer(10)
	buffer.Add(logging.LogEntry{Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Level: logging.LevelInfo, Message: "one"})
	buffer.Add(logging.LogEntry{Timestamp: time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC), Level: logging.LevelWarning, Message: "two"})
	buffer.Add(logging.LogEntry{Timestamp: time.Date(2024, 1, 1, 2, 0, 0, 0, time.UTC), Level: logging.LevelError, Message: "three"})
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, io.Discard)

	handler := &RestHandler{Logger: logger}
	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleLogs)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload []logging.LogEntry
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(payload))
	}
	if payload[0].Message != "one" {
		t.Fatalf("expected first entry 'one', got %q", payload[0].Message)
	}
}

func TestLogsEndpointFilters(t *testing.T) {
	buffer := logging.NewLogBuffer(10)
	buffer.Add(logging.LogEntry{Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Level: logging.LevelInfo, Message: "one"})
	buffer.Add(logging.LogEntry{Timestamp: time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC), Level: logging.LevelWarning, Message: "two"})
	buffer.Add(logging.LogEntry{Timestamp: time.Date(2024, 1, 1, 2, 0, 0, 0, time.UTC), Level: logging.LevelError, Message: "three"})
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, io.Discard)

	handler := &RestHandler{Logger: logger}
	req := httptest.NewRequest(http.MethodGet, "/api/logs?level=warning&limit=1", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleLogs)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload []logging.LogEntry
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(payload))
	}
	if payload[0].Message != "three" {
		t.Fatalf("expected last entry 'three', got %q", payload[0].Message)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/logs?since=2024-01-01T01:00:00Z", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res = httptest.NewRecorder()

	restHandler("secret", handler.handleLogs)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	payload = nil
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(payload))
	}
	if payload[0].Message != "two" {
		t.Fatalf("expected entry 'two', got %q", payload[0].Message)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/logs?level=unknown", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res = httptest.NewRecorder()

	restHandler("secret", handler.handleLogs)(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.Code)
	}
}

func TestLogsEndpointCreateEntry(t *testing.T) {
	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, io.Discard)
	handler := &RestHandler{Logger: logger}

	body := `{"level":"warning","message":"toast error","context":{"component":"terminal"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleLogs)(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}

	entries := buffer.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry.Level != logging.LevelWarning {
		t.Fatalf("expected warning level, got %q", entry.Level)
	}
	if entry.Message != "toast error" {
		t.Fatalf("expected message %q, got %q", "toast error", entry.Message)
	}
	if entry.Context["component"] != "terminal" {
		t.Fatalf("expected component context, got %q", entry.Context["component"])
	}
	if entry.Context["source"] != "frontend" {
		t.Fatalf("expected source context, got %q", entry.Context["source"])
	}
	if entry.Context["toast"] != "true" {
		t.Fatalf("expected toast context, got %q", entry.Context["toast"])
	}
}

func TestPlanEndpointReturnsContent(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, ".gestalt", "PLAN.org")
	if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	content := "* TODO [#A] Sample\n"
	if err := os.WriteFile(planPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write plan file: %v", err)
	}

	handler := &RestHandler{PlanPath: planPath}
	req := httptest.NewRequest(http.MethodGet, "/api/plan", nil)
	res := httptest.NewRecorder()

	jsonErrorMiddleware(handler.handlePlan)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload planResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Content != content {
		t.Fatalf("expected content %q, got %q", content, payload.Content)
	}
	if res.Header().Get("ETag") == "" {
		t.Fatalf("expected ETag header to be set")
	}
}

func TestPlanEndpointMissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, ".gestalt", "PLAN.org")
	handler := &RestHandler{PlanPath: planPath}
	req := httptest.NewRequest(http.MethodGet, "/api/plan", nil)
	res := httptest.NewRecorder()

	jsonErrorMiddleware(handler.handlePlan)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload planResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Content != "" {
		t.Fatalf("expected empty content, got %q", payload.Content)
	}
}

func TestPlanEndpointETagNotModified(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, ".gestalt", "PLAN.org")
	if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	content := "* TODO [#B] Cached\n"
	if err := os.WriteFile(planPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write plan file: %v", err)
	}

	handler := &RestHandler{PlanPath: planPath}
	req := httptest.NewRequest(http.MethodGet, "/api/plan", nil)
	res := httptest.NewRecorder()
	jsonErrorMiddleware(handler.handlePlan)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	etag := res.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("expected ETag header")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/plan", nil)
	req.Header.Set("If-None-Match", etag)
	res = httptest.NewRecorder()
	jsonErrorMiddleware(handler.handlePlan)(res, req)
	if res.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", res.Code)
	}
	if res.Body.Len() != 0 {
		t.Fatalf("expected empty body on 304 response")
	}
}

func TestPlanCurrentEndpointReturnsWip(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, ".gestalt", "PLAN.org")
	if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	content := "* WIP [#A] Feature One\n** WIP [#B] Step One\n"
	if err := os.WriteFile(planPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write plan file: %v", err)
	}

	planCache := plan.NewCache(planPath, nil)
	handler := &RestHandler{PlanPath: planPath, PlanCache: planCache}
	req := httptest.NewRequest(http.MethodGet, "/api/plan/current", nil)
	res := httptest.NewRecorder()

	jsonErrorMiddleware(handler.handlePlanCurrent)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload planCurrentResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.L1 != "Feature One" || payload.L2 != "Step One" {
		t.Fatalf("unexpected plan current payload: %#v", payload)
	}
}

func TestPlanCurrentEndpointMissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "missing.org")

	planCache := plan.NewCache(planPath, nil)
	handler := &RestHandler{PlanPath: planPath, PlanCache: planCache}
	req := httptest.NewRequest(http.MethodGet, "/api/plan/current", nil)
	res := httptest.NewRecorder()

	jsonErrorMiddleware(handler.handlePlanCurrent)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload planCurrentResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.L1 != "" || payload.L2 != "" {
		t.Fatalf("expected empty payload, got %#v", payload)
	}
}

func TestTerminalBellEndpointReturnsNoContent(t *testing.T) {
	factory := &fakeFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})
	created, err := manager.Create("", "", "")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, "/api/terminals/"+created.ID+"/bell", nil)
	res := httptest.NewRecorder()

	restHandler("", handler.handleTerminal)(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}
}

func TestTerminalBellEndpointMissingSession(t *testing.T) {
	manager := terminal.NewManager(terminal.ManagerOptions{Shell: "/bin/sh"})
	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, "/api/terminals/unknown/bell", nil)
	res := httptest.NewRecorder()

	restHandler("", handler.handleTerminal)(res, req)
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
		name   string
		path   string
		id     string
		action terminalPathAction
	}{
		{name: "terminal", path: "/api/terminals/123", id: "123", action: terminalPathTerminal},
		{name: "terminal-trailing-slash", path: "/api/terminals/123/", id: "123", action: terminalPathTerminal},
		{name: "output", path: "/api/terminals/123/output", id: "123", action: terminalPathOutput},
		{name: "output-trailing-slash", path: "/api/terminals/123/output/", id: "123", action: terminalPathOutput},
		{name: "history", path: "/api/terminals/123/history", id: "123", action: terminalPathHistory},
		{name: "history-trailing-slash", path: "/api/terminals/123/history/", id: "123", action: terminalPathHistory},
		{name: "input-history", path: "/api/terminals/123/input-history", id: "123", action: terminalPathInputHistory},
		{name: "input-history-trailing-slash", path: "/api/terminals/123/input-history/", id: "123", action: terminalPathInputHistory},
		{name: "workflow-resume", path: "/api/terminals/123/workflow/resume", id: "123", action: terminalPathWorkflowResume},
		{name: "workflow-resume-trailing-slash", path: "/api/terminals/123/workflow/resume/", id: "123", action: terminalPathWorkflowResume},
		{name: "workflow-history", path: "/api/terminals/123/workflow/history", id: "123", action: terminalPathWorkflowHistory},
		{name: "workflow-history-trailing-slash", path: "/api/terminals/123/workflow/history/", id: "123", action: terminalPathWorkflowHistory},
		{name: "missing-prefix", path: "/api/terminal/123", id: "", action: terminalPathTerminal},
		{name: "empty-id", path: "/api/terminals/", id: "", action: terminalPathTerminal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id, action := parseTerminalPath(test.path)
			if id != test.id {
				t.Fatalf("expected id %q, got %q", test.id, id)
			}
			if action != test.action {
				t.Fatalf("expected action %v, got %v", test.action, action)
			}
		})
	}
}
