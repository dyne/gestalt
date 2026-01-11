package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/terminal"

	"github.com/gorilla/websocket"
)

func TestAgentEventsWebSocketStream(t *testing.T) {
	factory := &fakeFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex"},
		},
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &AgentEventsHandler{Manager: manager}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/agents/events"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	session, err := manager.Create("codex", "role", "title")
	if err != nil {
		t.Fatalf("create agent session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var payload agentEventPayload
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if payload.Type != "agent_started" {
		t.Fatalf("expected agent_started, got %q", payload.Type)
	}
	if payload.AgentID != "codex" {
		t.Fatalf("expected agent_id codex, got %q", payload.AgentID)
	}
	if payload.AgentName != "Codex" {
		t.Fatalf("expected agent_name Codex, got %q", payload.AgentName)
	}
}
