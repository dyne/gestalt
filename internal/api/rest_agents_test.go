package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/terminal"
)

type recordPty struct {
	writes chan []byte
	closed chan struct{}
}

func newRecordPty() *recordPty {
	return &recordPty{
		writes: make(chan []byte, 4),
		closed: make(chan struct{}),
	}
}

func (p *recordPty) Read(_ []byte) (int, error) {
	<-p.closed
	return 0, io.EOF
}

func (p *recordPty) Write(data []byte) (int, error) {
	copyData := append([]byte(nil), data...)
	p.writes <- copyData
	return len(data), nil
}

func (p *recordPty) Close() error {
	select {
	case <-p.closed:
	default:
		close(p.closed)
	}
	return nil
}

func (p *recordPty) Resize(cols, rows uint16) error {
	return nil
}

type recordFactory struct {
	pty *recordPty
}

func (f *recordFactory) Start(command string, args ...string) (terminal.Pty, *exec.Cmd, error) {
	f.pty = newRecordPty()
	return f.pty, nil, nil
}

func TestAgentSendInputEndpoint(t *testing.T) {
	factory := &recordFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"coder": {
				Name:  "Coder",
				Shell: "/bin/bash",
			},
		},
	})
	created, err := manager.Create("coder", "shell", "Coder")
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer func() {
		_ = manager.Delete(created.ID)
	}()

	handler := &RestHandler{Manager: manager}
	req := httptest.NewRequest(http.MethodPost, "/api/agents/Coder/send-input", strings.NewReader(`{"input":"ping"}`))
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleAgentSendInput)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	var responsePayload map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&responsePayload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	pty := factory.pty
	if pty == nil {
		t.Fatalf("expected pty to be created")
	}

	expected := "ping\n"
	var received []byte
	select {
	case received = <-pty.writes:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for input")
	}

	if string(received) != expected {
		t.Fatalf("expected input %q, got %q", expected, string(received))
	}
}
