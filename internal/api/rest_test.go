package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/terminal"
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

func TestCreateTerminalWithAgent(t *testing.T) {
	factory := &fakeFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:    "Codex",
				Shell:   "/bin/zsh",
				LLMType: "codex",
			},
		},
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

func TestListTerminalsIncludesLLMMetadata(t *testing.T) {
	factory := &fakeFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:     "Codex",
				Shell:    "/bin/zsh",
				LLMType:  "codex",
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

func TestAgentsEndpoint(t *testing.T) {
	manager := terminal.NewManager(terminal.ManagerOptions{
		Agents: map[string]agent.Agent{
			"codex": {
				Name:     "Codex",
				Shell:    "/bin/zsh",
				LLMType:  "codex",
				LLMModel: "default",
			},
			"copilot": {
				Name:     "Copilot",
				Shell:    "/bin/bash",
				LLMType:  "copilot",
				LLMModel: "default",
			},
		},
	})

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
	if payload[0].ID != "codex" && payload[1].ID != "codex" {
		t.Fatalf("missing codex agent")
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
		name        string
		path        string
		id          string
		wantsOutput bool
	}{
		{name: "terminal", path: "/api/terminals/123", id: "123", wantsOutput: false},
		{name: "terminal-trailing-slash", path: "/api/terminals/123/", id: "123", wantsOutput: false},
		{name: "output", path: "/api/terminals/123/output", id: "123", wantsOutput: true},
		{name: "output-trailing-slash", path: "/api/terminals/123/output/", id: "123", wantsOutput: true},
		{name: "missing-prefix", path: "/api/terminal/123", id: "", wantsOutput: false},
		{name: "empty-id", path: "/api/terminals/", id: "", wantsOutput: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id, wantsOutput := parseTerminalPath(test.path)
			if id != test.id {
				t.Fatalf("expected id %q, got %q", test.id, id)
			}
			if wantsOutput != test.wantsOutput {
				t.Fatalf("expected wantsOutput %v, got %v", test.wantsOutput, wantsOutput)
			}
		})
	}
}
