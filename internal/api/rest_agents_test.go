package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

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

func TestAgentsEndpointIncludesInterface(t *testing.T) {
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &recordFactory{},
		Agents: map[string]agent.Agent{
			"coder": {
				Name:  "Coder",
				Shell: "/bin/bash",
			},
		},
	})
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
	if len(payload) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(payload))
	}
	if payload[0].Interface != agent.AgentInterfaceCLI {
		t.Fatalf("expected interface %q, got %q", agent.AgentInterfaceCLI, payload[0].Interface)
	}
}
