package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync"
	"testing"
	"time"

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
	mu   sync.Mutex
	ptys []*fakePty
}

func (f *fakeFactory) Start(command string, args ...string) (terminal.Pty, *exec.Cmd, error) {
	pty := newFakePty()

	f.mu.Lock()
	f.ptys = append(f.ptys, pty)
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
	created, err := manager.Create("", "")
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
	created, err := manager.Create("", "")
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
