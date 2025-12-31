package api

import (
	"encoding/json"
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
	"gestalt/internal/logging"
	"gestalt/internal/skill"
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
	planPath := filepath.Join(dir, "PLAN.org")
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
}

func TestPlanEndpointMissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "missing.org")
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
