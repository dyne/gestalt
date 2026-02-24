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
	"strings"
	"sync"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/event"
	"gestalt/internal/flow"
	"gestalt/internal/logging"
	"gestalt/internal/notify"
	"gestalt/internal/otel"
	"gestalt/internal/runner/launchspec"
	"gestalt/internal/skill"
	"gestalt/internal/terminal"
	"gestalt/internal/version"
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

func writeFlowConfig(t *testing.T, repo flow.Repository, eventType string) {
	t.Helper()
	if repo == nil {
		t.Fatalf("repo is required")
	}
	cfg := flow.Config{
		Version: flow.ConfigVersion,
		Triggers: []flow.EventTrigger{
			{ID: "t1", EventType: eventType},
		},
		BindingsByTriggerID: map[string][]flow.ActivityBinding{
			"t1": {
				{ActivityID: "toast_notification", Config: map[string]any{"level": "info", "message_template": "hi"}},
			},
		},
	}
	if err := repo.Save(cfg); err != nil {
		t.Fatalf("save flow config: %v", err)
	}
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

type fakeTmuxClient struct {
	hasSession bool
	targets    []string
	windows    map[string]bool
	loads      [][]byte
	pastes     []string
	loadErr    error
	pasteErr   error
	resizeErr  error
}

func (f *fakeTmuxClient) HasSession(name string) (bool, error) {
	return f.hasSession, nil
}

func (f *fakeTmuxClient) HasWindow(sessionName, windowName string) (bool, error) {
	if f.windows == nil {
		return true, nil
	}
	return f.windows[windowName], nil
}

func (f *fakeTmuxClient) SelectWindow(target string) error {
	f.targets = append(f.targets, target)
	return nil
}

func (f *fakeTmuxClient) LoadBuffer(data []byte) error {
	if f.loadErr != nil {
		return f.loadErr
	}
	f.loads = append(f.loads, append([]byte(nil), data...))
	return nil
}

func (f *fakeTmuxClient) PasteBuffer(target string) error {
	if f.pasteErr != nil {
		return f.pasteErr
	}
	f.pastes = append(f.pastes, target)
	return nil
}

func (f *fakeTmuxClient) ResizePane(target string, cols, rows uint16) error {
	if f.resizeErr != nil {
		return f.resizeErr
	}
	return nil
}

type fakeDispatcher struct {
	mu       sync.Mutex
	requests []flow.ActivityRequest
	err      error
}

func (dispatcher *fakeDispatcher) Dispatch(ctx context.Context, request flow.ActivityRequest) error {
	dispatcher.mu.Lock()
	dispatcher.requests = append(dispatcher.requests, request)
	dispatcher.mu.Unlock()
	if dispatcher.err != nil {
		return dispatcher.err
	}
	return nil
}

func (dispatcher *fakeDispatcher) Requests() []flow.ActivityRequest {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	copied := make([]flow.ActivityRequest, len(dispatcher.requests))
	copy(copied, dispatcher.requests)
	return copied
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
	t.Skip("obsolete: agents hub session adds extra count")
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

	handler := &RestHandler{
		Manager:                manager,
		SessionScrollbackLines: 4242,
		SessionFontFamily:      "Courier New, monospace",
		SessionFontSize:        "14px",
		SessionInputFontFamily: "Input Mono",
		SessionInputFontSize:   "12px",
	}
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
	if payload.SessionScrollbackLines != 4242 {
		t.Fatalf("expected scrollback lines 4242, got %d", payload.SessionScrollbackLines)
	}
	if payload.SessionFontFamily != "Courier New, monospace" {
		t.Fatalf("expected session font family, got %q", payload.SessionFontFamily)
	}
	if payload.SessionFontSize != "14px" {
		t.Fatalf("expected session font size, got %q", payload.SessionFontSize)
	}
	if payload.SessionInputFontFamily != "Input Mono" {
		t.Fatalf("expected session input font family, got %q", payload.SessionInputFontFamily)
	}
	if payload.SessionInputFontSize != "12px" {
		t.Fatalf("expected session input font size, got %q", payload.SessionInputFontSize)
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

func TestStatusHandlerIncludesAgentsHubFields(t *testing.T) {
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &fakeFactory{},
		Agents: map[string]agent.Agent{
			"codex": {
				Name:      "Codex",
				Shell:     "codex -c model=o3",
				CLIType:   "codex",
				Interface: agent.AgentInterfaceCLI,
			},
		},
		StartExternalTmuxWindow: func(_ *launchspec.LaunchSpec) error {
			return nil
		},
	})
	created, err := manager.CreateWithOptions(terminal.CreateOptions{
		AgentID: "codex",
		Runner:  "external",
	})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
		hubID, _ := manager.AgentsHubStatus()
		if hubID != "" {
			_ = manager.Delete(hubID)
		}
	}()

	expectedHubID, expectedTmux := manager.AgentsHubStatus()
	if expectedHubID == "" || expectedTmux == "" {
		t.Fatalf("expected hub status, got id=%q tmux=%q", expectedHubID, expectedTmux)
	}

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
	if payload.AgentsSessionID != expectedHubID {
		t.Fatalf("expected agents_session_id %q, got %q", expectedHubID, payload.AgentsSessionID)
	}
	if payload.AgentsTmuxSession != expectedTmux {
		t.Fatalf("expected agents_tmux_session %q, got %q", expectedTmux, payload.AgentsTmuxSession)
	}
}

func TestTerminalNotifyEndpoint(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex", Shell: "/bin/bash", CLIType: "codex"},
		},
	})
	created, err := manager.CreateWithOptions(terminal.CreateOptions{
		AgentID: "codex",
	})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	tempDir := t.TempDir()
	repo := flow.NewFileRepository(filepath.Join(tempDir, "automations.json"), nil)
	writeFlowConfig(t, repo, flow.CanonicalNotifyEventType("plan-L1-wip"))
	dispatcher := &fakeDispatcher{}
	service := flow.NewService(repo, dispatcher, nil)
	sink := notify.NewMemorySink()
	handler := &RestHandler{Manager: manager, FlowService: service, NotificationSink: sink}
	body := `{"session_id":"` + created.ID + `","occurred_at":"2025-04-01T10:00:00Z","payload":{"type":"plan-L1-wip","plan_file":"plan.org"},"raw":"{}","event_id":"manual:1"}`
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/notify", strings.NewReader(body))
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}
	requests := dispatcher.Requests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(requests))
	}
	request := requests[0]
	if request.EventID != "manual:1" {
		t.Fatalf("expected event id manual:1, got %q", request.EventID)
	}
	if request.Event["type"] != "plan-update" {
		t.Fatalf("expected canonical type plan-update, got %q", request.Event["type"])
	}
	if request.Event["notify.type"] != "plan-L1-wip" {
		t.Fatalf("expected notify.type plan-L1-wip, got %q", request.Event["notify.type"])
	}
	if request.Event["notify.event_id"] != "manual:1" {
		t.Fatalf("expected notify.event_id manual:1, got %q", request.Event["notify.event_id"])
	}
	if request.Event["session_id"] != created.ID {
		t.Fatalf("expected session_id %q, got %q", created.ID, request.Event["session_id"])
	}
	if request.Event["plan_file"] != "plan.org" || request.Event["notify.plan_file"] != "plan.org" {
		t.Fatalf("expected plan_file aliases, got %q/%q", request.Event["plan_file"], request.Event["notify.plan_file"])
	}
	if request.Event["timestamp"] != time.Date(2025, 4, 1, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano) {
		t.Fatalf("unexpected notify timestamp: %v", request.Event["timestamp"])
	}
	events := sink.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 notification event, got %d", len(events))
	}
	if events[0].Fields["notify.type"] != "plan-L1-wip" {
		t.Fatalf("expected notify.type plan-L1-wip, got %q", events[0].Fields["notify.type"])
	}
}

func TestTerminalNotifyProgressMissingPlanFile(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex", Shell: "/bin/bash", CLIType: "codex"},
		},
	})
	created, err := manager.CreateWithOptions(terminal.CreateOptions{
		AgentID: "codex",
	})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager, NotificationSink: notify.NewMemorySink()}
	body := `{"session_id":"` + created.ID + `","payload":{"type":"progress"}}`
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/notify", strings.NewReader(body))
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", res.Code)
	}
}

func TestTerminalNotifyProgressNormalization(t *testing.T) {
	cases := []struct {
		name     string
		planFile string
	}{
		{name: "plans-path", planFile: "plans/plan.org"},
		{name: "gestalt-plans-path", planFile: ".gestalt/plans/plan.org"},
		{name: "basename", planFile: "plan.org"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			factory := &fakeFactory{}
			manager := newTestManager(terminal.ManagerOptions{
				Shell:      "/bin/sh",
				PtyFactory: factory,
				Agents: map[string]agent.Agent{
					"codex": {Name: "Codex", Shell: "/bin/bash", CLIType: "codex"},
				},
			})
			created, err := manager.CreateWithOptions(terminal.CreateOptions{
				AgentID: "codex",
			})
			if err != nil {
				t.Fatalf("create terminal: %v", err)
			}
			defer func() {
				_ = manager.Delete(created.ID)
			}()

			tempDir := t.TempDir()
			repo := flow.NewFileRepository(filepath.Join(tempDir, "automations.json"), nil)
			writeFlowConfig(t, repo, flow.CanonicalNotifyEventType("progress"))
			dispatcher := &fakeDispatcher{}
			service := flow.NewService(repo, dispatcher, nil)
			handler := &RestHandler{Manager: manager, FlowService: service, NotificationSink: notify.NewMemorySink()}
			body := `{"session_id":"` + created.ID + `","payload":{"type":"progress","plan_file":"` + testCase.planFile + `","l1":"* TODO [#A] First L1","l2":"WIP [#B] L2 Two","task_level":"2","task_state":"WIP"}}`
			req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/notify", strings.NewReader(body))
			res := httptest.NewRecorder()

			restHandler("", nil, handler.handleTerminal)(res, req)
			if res.Code != http.StatusNoContent {
				t.Fatalf("expected 204, got %d", res.Code)
			}
			requests := dispatcher.Requests()
			if len(requests) != 1 {
				t.Fatalf("expected 1 dispatch, got %d", len(requests))
			}
			request := requests[0]
			if request.Event["plan_file"] != "plan.org" || request.Event["notify.plan_file"] != "plan.org" {
				t.Fatalf("expected normalized plan_file, got %q/%q", request.Event["plan_file"], request.Event["notify.plan_file"])
			}
			if request.Event["l1"] != "First L1" {
				t.Fatalf("expected normalized l1, got %q", request.Event["l1"])
			}
			if request.Event["l2"] != "L2 Two" {
				t.Fatalf("expected normalized l2, got %q", request.Event["l2"])
			}
			if request.Event["task_level"] != "2" {
				t.Fatalf("expected task_level 2, got %q", request.Event["task_level"])
			}
		})
	}
}

func TestTerminalNotifyEndpointMissingTerminal(t *testing.T) {
	manager := newTestManager(terminal.ManagerOptions{Shell: "/bin/sh"})
	handler := &RestHandler{Manager: manager}

	req := httptest.NewRequest(http.MethodPost, "/api/sessions/missing/notify", strings.NewReader(`{"session_id":"missing","payload":{"type":"plan-L1-wip"}}`))
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", res.Code)
	}
}

func TestTerminalNotifyEndpointWithoutBindings(t *testing.T) {
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

	tempDir := t.TempDir()
	repo := flow.NewFileRepository(filepath.Join(tempDir, "automations.json"), nil)
	dispatcher := &fakeDispatcher{}
	service := flow.NewService(repo, dispatcher, nil)
	handler := &RestHandler{Manager: manager, FlowService: service, NotificationSink: notify.NewMemorySink()}
	body := `{"session_id":"` + created.ID + `","payload":{"type":"plan-L1-wip"}}`
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/notify", strings.NewReader(body))
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}
	if len(dispatcher.Requests()) != 0 {
		t.Fatalf("expected no dispatch when config has no bindings")
	}
}

func TestTerminalNotifyEndpointDispatcherUnavailable(t *testing.T) {
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

	tempDir := t.TempDir()
	repo := flow.NewFileRepository(filepath.Join(tempDir, "automations.json"), nil)
	service := flow.NewService(repo, nil, nil)
	handler := &RestHandler{Manager: manager, FlowService: service, NotificationSink: notify.NewMemorySink()}
	body := `{"session_id":"` + created.ID + `","payload":{"type":"plan-L1-wip"}}`
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/notify", strings.NewReader(body))
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", res.Code)
	}
}

func TestTerminalNotifyLoggingDoesNotChangeStatusMapping(t *testing.T) {
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

	tempDir := t.TempDir()
	repo := flow.NewFileRepository(filepath.Join(tempDir, "automations.json"), nil)
	service := flow.NewService(repo, nil, nil)
	body := `{"session_id":"` + created.ID + `","payload":{"type":"plan-L1-wip"}}`

	for _, withLogger := range []bool{false, true} {
		handler := &RestHandler{Manager: manager, FlowService: service}
		if withLogger {
			logBuffer := logging.NewLogBuffer(20)
			handler.Logger = logging.NewLoggerWithOutput(logBuffer, logging.LevelDebug, nil)
		}
		req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/notify", strings.NewReader(body))
		res := httptest.NewRecorder()

		restHandler("", nil, handler.handleTerminal)(res, req)
		if res.Code != http.StatusServiceUnavailable {
			t.Fatalf("withLogger=%t expected 503, got %d", withLogger, res.Code)
		}
		var payload errorResponse
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			t.Fatalf("withLogger=%t decode response: %v", withLogger, err)
		}
		if payload.Message != "notification sink unavailable" {
			t.Fatalf("withLogger=%t expected notification sink unavailable message, got %q", withLogger, payload.Message)
		}
	}
}

func TestTerminalProgressEndpointMissingSession(t *testing.T) {
	manager := newTestManager(terminal.ManagerOptions{Shell: "/bin/sh"})
	handler := &RestHandler{Manager: manager}

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/missing/progress", nil)
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", res.Code)
	}
}

func TestTerminalProgressEndpointEmpty(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex", Shell: "/bin/bash", CLIType: "codex"},
		},
	})
	created, err := manager.CreateWithOptions(terminal.CreateOptions{
		AgentID: "codex",
	})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, terminalPath(created.ID)+"/progress", nil)
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload terminalProgressResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.HasProgress {
		t.Fatalf("expected has_progress false")
	}
}

func TestTerminalProgressEndpointAfterNotify(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex", Shell: "/bin/bash", CLIType: "codex"},
		},
	})
	created, err := manager.CreateWithOptions(terminal.CreateOptions{
		AgentID: "codex",
	})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	tempDir := t.TempDir()
	repo := flow.NewFileRepository(filepath.Join(tempDir, "automations.json"), nil)
	service := flow.NewService(repo, nil, nil)
	handler := &RestHandler{Manager: manager, FlowService: service, NotificationSink: notify.NewMemorySink()}

	body := `{"session_id":"` + created.ID + `","payload":{"type":"progress","plan_file":"plans/plan.org","l1":"TODO First L1","l2":"WIP Second L2","task_level":2,"task_state":"WIP"}}`
	notifyReq := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/notify", strings.NewReader(body))
	notifyRes := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(notifyRes, notifyReq)
	if notifyRes.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", notifyRes.Code)
	}

	progressReq := httptest.NewRequest(http.MethodGet, terminalPath(created.ID)+"/progress", nil)
	progressRes := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(progressRes, progressReq)
	if progressRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", progressRes.Code)
	}

	var payload terminalProgressResponse
	if err := json.NewDecoder(progressRes.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.HasProgress {
		t.Fatalf("expected has_progress true")
	}
	if payload.PlanFile != "plan.org" {
		t.Fatalf("expected plan_file plan.org, got %q", payload.PlanFile)
	}
	if payload.L1 != "First L1" {
		t.Fatalf("expected l1 First L1, got %q", payload.L1)
	}
	if payload.L2 != "Second L2" {
		t.Fatalf("expected l2 Second L2, got %q", payload.L2)
	}
	if payload.TaskLevel != 2 {
		t.Fatalf("expected task_level 2, got %d", payload.TaskLevel)
	}
	if payload.TaskState != "WIP" {
		t.Fatalf("expected task_state WIP, got %q", payload.TaskState)
	}
	if payload.UpdatedAt == nil || payload.UpdatedAt.IsZero() {
		t.Fatalf("expected updated_at set")
	}
}

func TestTerminalNotifyProgressPublishesEvent(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex", Shell: "/bin/bash", CLIType: "codex"},
		},
	})
	created, err := manager.CreateWithOptions(terminal.CreateOptions{
		AgentID: "codex",
	})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	events, cancel := manager.TerminalBus().Subscribe()
	defer cancel()

	handler := &RestHandler{Manager: manager, NotificationSink: notify.NewMemorySink()}
	body := `{"session_id":"` + created.ID + `","payload":{"type":"progress","plan_file":"plan.org","l1":"First L1","l2":"Second L2","task_level":2,"task_state":"WIP"}}`
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/notify", strings.NewReader(body))
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}

	terminalEvent := event.ReceiveWithTimeout(t, events, time.Second)
	if terminalEvent.Type() != "plan-update" {
		t.Fatalf("expected plan-update event, got %q", terminalEvent.Type())
	}
	if terminalEvent.TerminalID != created.ID {
		t.Fatalf("expected terminal id %q, got %q", created.ID, terminalEvent.TerminalID)
	}
	if terminalEvent.Data["plan_file"] != "plan.org" {
		t.Fatalf("expected plan_file plan.org, got %v", terminalEvent.Data["plan_file"])
	}
	if terminalEvent.Data["l1"] != "First L1" {
		t.Fatalf("expected l1 First L1, got %v", terminalEvent.Data["l1"])
	}
	if terminalEvent.Data["l2"] != "Second L2" {
		t.Fatalf("expected l2 Second L2, got %v", terminalEvent.Data["l2"])
	}
	if terminalEvent.Data["task_level"] != 2 {
		t.Fatalf("expected task_level 2, got %v", terminalEvent.Data["task_level"])
	}
	if terminalEvent.Data["task_state"] != "WIP" {
		t.Fatalf("expected task_state WIP, got %v", terminalEvent.Data["task_state"])
	}
	if terminalEvent.Data["timestamp"] == nil || terminalEvent.OccurredAt.IsZero() {
		t.Fatalf("expected timestamp set")
	}
}

func TestTerminalNotifyEndpointBadJSON(t *testing.T) {
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex", Shell: "/bin/bash", CLIType: "codex"},
		},
	})
	created, err := manager.CreateWithOptions(terminal.CreateOptions{
		AgentID: "codex",
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
	t.Skip("obsolete: expects PTY-backed agent output")
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
	t.Skip("obsolete: expects PTY-backed agent output")
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

	if !waitForOutputLines(created, 1, 500*time.Millisecond) {
		t.Fatalf("expected output buffer to receive data")
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
	if payload.Cursor != nil {
		t.Fatalf("expected cursor to be nil when session logs are disabled for agents")
	}
}

func TestTerminalHistoryPagination(t *testing.T) {
	t.Skip("obsolete: expects PTY-backed agent output")
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

	if !waitForOutputLines(created, len(payloads), 500*time.Millisecond) {
		t.Fatalf("expected output buffer to receive data")
	}

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodGet, terminalPath(created.ID)+"/history?lines=4", nil)
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
	nonEmpty := make([]string, 0, len(page.Lines))
	for _, line := range page.Lines {
		if line != "" {
			nonEmpty = append(nonEmpty, line)
		}
	}
	if len(nonEmpty) < 2 || nonEmpty[len(nonEmpty)-2] != "four" || nonEmpty[len(nonEmpty)-1] != "five" {
		t.Fatalf("unexpected history page: %v", page.Lines)
	}
	if page.Cursor != nil {
		t.Fatalf("expected cursor to be nil when session logs are disabled for agents")
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

func TestTerminalInputEndpoint(t *testing.T) {
	t.Skip("obsolete: expects PTY-backed input writes")
	factory := &recordFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			testAgentID: {
				Name:  "Codex",
				Shell: "/bin/sh",
			},
		},
	})
	created, err := manager.Create(testAgentID, "", "")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/input", strings.NewReader("hello"))
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminal)(res, req)
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
	select {
	case got := <-factory.pty.writes:
		if string(got) != "hello" {
			t.Fatalf("expected raw bytes %q, got %q", "hello", string(got))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for input write")
	}
}

func TestTerminalInputEndpointTmuxManagedAgentSession(t *testing.T) {
	tmuxClient := &fakeTmuxClient{hasSession: true}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &fakeFactory{},
		Agents: map[string]agent.Agent{
			"codex": {
				Name:      "Codex",
				Shell:     "codex -c model=o3",
				CLIType:   "codex",
				Interface: agent.AgentInterfaceCLI,
			},
		},
		StartExternalTmuxWindow: func(_ *launchspec.LaunchSpec) error { return nil },
		TmuxClientFactory:       func() terminal.TmuxClient { return tmuxClient },
	})
	created, err := manager.CreateWithOptions(terminal.CreateOptions{AgentID: "codex"})
	if err != nil {
		t.Fatalf("create tmux-managed terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
		if hubID, _ := manager.AgentsHubStatus(); hubID != "" {
			_ = manager.Delete(hubID)
		}
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/input", strings.NewReader("hello\nworld\n"))
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if len(tmuxClient.loads) != 1 {
		t.Fatalf("expected one LoadBuffer call, got %d", len(tmuxClient.loads))
	}
	if string(tmuxClient.loads[0]) != "hello\nworld\n" {
		t.Fatalf("expected raw payload preserved, got %q", string(tmuxClient.loads[0]))
	}
	if len(tmuxClient.pastes) != 1 {
		t.Fatalf("expected one PasteBuffer call, got %d", len(tmuxClient.pastes))
	}
	if !strings.HasSuffix(tmuxClient.pastes[0], ":"+created.ID) {
		t.Fatalf("expected paste target to end with %q, got %q", ":"+created.ID, tmuxClient.pastes[0])
	}
}

func TestTerminalInputEndpointTmuxWindowMissing(t *testing.T) {
	tmuxClient := &fakeTmuxClient{hasSession: true, pasteErr: errors.New("can't find window")}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &fakeFactory{},
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex", Shell: "codex", CLIType: "codex", Interface: agent.AgentInterfaceCLI},
		},
		StartExternalTmuxWindow: func(_ *launchspec.LaunchSpec) error { return nil },
		TmuxClientFactory:       func() terminal.TmuxClient { return tmuxClient },
	})
	created, err := manager.CreateWithOptions(terminal.CreateOptions{AgentID: "codex"})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/input", strings.NewReader("hello"))
	res := httptest.NewRecorder()
	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", res.Code)
	}
	var payload errorResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(payload.Message, "session window not found; run gestalt-agent codex") {
		t.Fatalf("unexpected error message: %q", payload.Message)
	}
}

func TestTerminalInputEndpointTmuxUnavailable(t *testing.T) {
	tmuxClient := &fakeTmuxClient{hasSession: true, loadErr: errors.New("exec: \"tmux\": executable file not found in $PATH")}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &fakeFactory{},
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex", Shell: "codex", CLIType: "codex", Interface: agent.AgentInterfaceCLI},
		},
		StartExternalTmuxWindow: func(_ *launchspec.LaunchSpec) error { return nil },
		TmuxClientFactory:       func() terminal.TmuxClient { return tmuxClient },
	})
	created, err := manager.CreateWithOptions(terminal.CreateOptions{AgentID: "codex"})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/input", strings.NewReader("hello"))
	res := httptest.NewRecorder()
	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", res.Code)
	}
	var payload errorResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(payload.Message, "tmux unavailable; run gestalt-agent codex") {
		t.Fatalf("unexpected error message: %q", payload.Message)
	}
}

func TestTerminalInputEndpointTmuxBridgeDetached(t *testing.T) {
	tmuxClient := &fakeTmuxClient{hasSession: true}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &fakeFactory{},
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex", Shell: "codex", CLIType: "codex", Interface: agent.AgentInterfaceCLI},
		},
		StartExternalTmuxWindow: func(_ *launchspec.LaunchSpec) error { return nil },
		TmuxClientFactory:       func() terminal.TmuxClient { return tmuxClient },
	})
	created, err := manager.CreateWithOptions(terminal.CreateOptions{AgentID: "codex"})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	created.DetachExternalRunner()
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/input", strings.NewReader("hello"))
	res := httptest.NewRecorder()
	restHandler("", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", res.Code)
	}
	var payload errorResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(payload.Message, "session input bridge unavailable; run gestalt-agent codex") {
		t.Fatalf("unexpected error message: %q", payload.Message)
	}
}

func TestTerminalInputEndpointMissingSession(t *testing.T) {
	manager := newTestManager(terminal.ManagerOptions{Shell: "/bin/sh", PtyFactory: &fakeFactory{}})
	handler := &RestHandler{Manager: manager}

	req := httptest.NewRequest(http.MethodPost, "/api/sessions/missing/input", strings.NewReader("hello"))
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", res.Code)
	}
}

func TestTerminalActivateEndpointSelectsTmuxWindow(t *testing.T) {
	tmuxClient := &fakeTmuxClient{hasSession: true}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &fakeFactory{},
		Agents: map[string]agent.Agent{
			"codex": {
				Name:      "Codex",
				Shell:     "codex -c model=o3",
				CLIType:   "codex",
				Interface: agent.AgentInterfaceCLI,
			},
		},
		StartExternalTmuxWindow: func(_ *launchspec.LaunchSpec) error { return nil },
		TmuxClientFactory:       func() terminal.TmuxClient { return tmuxClient },
	})
	created, err := manager.CreateWithOptions(terminal.CreateOptions{AgentID: "codex", Runner: "external"})
	if err != nil {
		t.Fatalf("create external terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
		if hubID, _ := manager.AgentsHubStatus(); hubID != "" {
			_ = manager.Delete(hubID)
		}
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/activate", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}
	if len(tmuxClient.targets) != 1 {
		t.Fatalf("expected one select-window call, got %d", len(tmuxClient.targets))
	}
	if !strings.HasSuffix(tmuxClient.targets[0], ":"+created.ID) {
		t.Fatalf("expected target to end with session id %q, got %q", created.ID, tmuxClient.targets[0])
	}
}

func TestTerminalActivateEndpointNonExternalReturnsConflict(t *testing.T) {
	t.Skip("obsolete: agent sessions are always external tmux-backed")
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &fakeFactory{},
	})
	created, err := manager.Create(testAgentID, "", "")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, terminalPath(created.ID)+"/activate", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", nil, handler.handleTerminal)(res, req)
	if res.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", res.Code)
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

	req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{"title":"ignored","role":"shell","agent":"codex","runner":"server"}`))
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
	if payload.Interface != agent.AgentInterfaceCLI {
		t.Fatalf("expected interface %q, got %q", agent.AgentInterfaceCLI, payload.Interface)
	}
	if payload.ID == "" {
		t.Fatalf("missing id")
	}
	if payload.Runner != "external" {
		t.Fatalf("expected runner external, got %q", payload.Runner)
	}
	defer func() {
		_ = manager.Delete(payload.ID)
	}()
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

func TestListTerminalsIncludesModelMetadata(t *testing.T) {
	t.Skip("obsolete: llm_type no longer coupled to cli_type")
	factory := &fakeFactory{}
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:    "Codex",
				Shell:   "/bin/zsh",
				CLIType: "codex",
				Model:   "default",
			},
			"architect": {
				Name:  "Architect",
				Shell: "/bin/bash",
			},
		},
	})

	defaultSession, err := manager.Create("architect", "build", "plain")
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
	if agentSummary.Model != "default" {
		t.Fatalf("expected model default, got %q", agentSummary.Model)
	}
	if agentSummary.Interface != agent.AgentInterfaceCLI {
		t.Fatalf("expected interface %q, got %q", agent.AgentInterfaceCLI, agentSummary.Interface)
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
				Name:    "Codex",
				Shell:   "/bin/zsh",
				CLIType: "codex",
				Model:   "default",
			},
			"copilot": {
				Name:    "Copilot",
				Shell:   "/bin/bash",
				CLIType: "copilot",
				Model:   "default",
				Hidden:  true,
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
	if codex.Interface != agent.AgentInterfaceCLI {
		t.Fatalf("expected codex interface %q, got %q", agent.AgentInterfaceCLI, codex.Interface)
	}
	if codex.Hidden {
		t.Fatalf("expected codex hidden=false")
	}
	if copilot.Interface != agent.AgentInterfaceCLI {
		t.Fatalf("expected copilot interface %q, got %q", agent.AgentInterfaceCLI, copilot.Interface)
	}
	if !copilot.Hidden {
		t.Fatalf("expected copilot hidden=true")
	}
	if copilot.Running {
		t.Fatalf("expected copilot to be stopped")
	}
	if copilot.SessionID != "" {
		t.Fatalf("expected copilot terminal id to be empty, got %q", copilot.SessionID)
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

func TestCreateTerminalRejectsUnsupportedInterfaceProfile(t *testing.T) {
	t.Skip("obsolete: interface setting removed")
	dir := t.TempDir()
	agentTOML := "name = \"Codex\"\nshell = \"/bin/sh\"\ncli_type = \"codex\"\ninterface = \"mcp\"\n"
	if err := os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(agentTOML), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}
	manager := newTestManager(terminal.ManagerOptions{
		AgentsDir:  dir,
		Shell:      "/bin/sh",
		PtyFactory: &fakeFactory{},
	})
	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{"agent":"codex"}`))
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminals)(res, req)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", res.Code)
	}

	var payload errorResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(payload.Message, "failed to refresh agent config") {
		t.Fatalf("expected config refresh error, got %q", payload.Message)
	}
}

func TestCreateTerminalMapsExternalTmuxFailure(t *testing.T) {
	manager := newTestManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &fakeFactory{},
		Agents: map[string]agent.Agent{
			"codex": {
				Name:      "Codex",
				Shell:     "codex -c model=o3",
				CLIType:   "codex",
				Interface: agent.AgentInterfaceCLI,
			},
		},
		StartExternalTmuxWindow: func(_ *launchspec.LaunchSpec) error {
			return errors.New("exec: \"tmux\": executable file not found in $PATH")
		},
	})
	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{"agent":"codex","runner":"external"}`))
	res := httptest.NewRecorder()

	restHandler("", nil, handler.handleTerminals)(res, req)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", res.Code)
	}

	var payload errorResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Message != "tmux unavailable" {
		t.Fatalf("expected tmux unavailable message, got %q", payload.Message)
	}
}

func TestCreateTerminalProfilesReturnStableInterfaceRunner(t *testing.T) {
	dir := t.TempDir()
	agentIDs := []string{"coder", "fixer", "architect"}
	agentNames := map[string]string{
		"coder":     "Coder",
		"fixer":     "Fixer",
		"architect": "Architect",
	}
	profiles := map[string]agent.Agent{}
	for _, id := range agentIDs {
		name := agentNames[id]
		agentTOML := "name = \"" + name + "\"\nshell = \"/bin/sh\"\ncli_type = \"codex\"\ninterface = \"cli\"\n"
		if err := os.WriteFile(filepath.Join(dir, id+".toml"), []byte(agentTOML), 0o644); err != nil {
			t.Fatalf("write agent file for %s: %v", id, err)
		}
		profiles[id] = agent.Agent{
			Name:      name,
			Shell:     "/bin/sh",
			CLIType:   "codex",
			Interface: agent.AgentInterfaceCLI,
		}
	}

	manager := newTestManager(terminal.ManagerOptions{
		Agents:     profiles,
		AgentsDir:  dir,
		PtyFactory: &fakeFactory{},
	})
	handler := &RestHandler{Manager: manager}

	for _, id := range agentIDs {
		req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{"agent":"`+id+`"}`))
		res := httptest.NewRecorder()
		restHandler("", nil, handler.handleTerminals)(res, req)
		if res.Code != http.StatusCreated {
			t.Fatalf("%s: expected 201, got %d", id, res.Code)
		}
		var payload terminalCreateResponse
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			t.Fatalf("%s: decode response: %v", id, err)
		}
		if payload.Interface != agent.AgentInterfaceCLI {
			t.Fatalf("%s: expected interface %q, got %q", id, agent.AgentInterfaceCLI, payload.Interface)
		}
		if payload.Runner != "external" {
			t.Fatalf("%s: expected runner external, got %q", id, payload.Runner)
		}
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

func waitForOutputLines(session *terminal.Session, min int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(session.OutputLines()) >= min {
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
		{name: "input", path: "/api/sessions/123/input", id: "123", action: terminalPathInput},
		{name: "input-trailing-slash", path: "/api/sessions/123/input/", id: "123", action: terminalPathInput},
		{name: "activate", path: "/api/sessions/123/activate", id: "123", action: terminalPathActivate},
		{name: "activate-trailing-slash", path: "/api/sessions/123/activate/", id: "123", action: terminalPathActivate},
		{name: "input-history", path: "/api/sessions/123/input-history", id: "123", action: terminalPathInputHistory},
		{name: "input-history-trailing-slash", path: "/api/sessions/123/input-history/", id: "123", action: terminalPathInputHistory},
		{name: "workflow-resume", path: "/api/sessions/123/workflow/resume", wantErr: true, status: http.StatusNotFound},
		{name: "workflow-resume-trailing-slash", path: "/api/sessions/123/workflow/resume/", wantErr: true, status: http.StatusNotFound},
		{name: "workflow-history", path: "/api/sessions/123/workflow/history", wantErr: true, status: http.StatusNotFound},
		{name: "workflow-history-trailing-slash", path: "/api/sessions/123/workflow/history/", wantErr: true, status: http.StatusNotFound},
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
