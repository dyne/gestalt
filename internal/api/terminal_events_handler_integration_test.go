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

func TestTerminalEventsWebSocketStream(t *testing.T) {
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
		Config:   &http.Server{Handler: &TerminalEventsHandler{Manager: manager}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/sessions/events"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	session, err := manager.Create("codex", "role", "title")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var created terminalEventPayload
	if err := conn.ReadJSON(&created); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if created.Type != "terminal_created" {
		t.Fatalf("expected terminal_created, got %q", created.Type)
	}
	if created.TerminalID != session.ID {
		t.Fatalf("expected terminal ID %q, got %q", session.ID, created.TerminalID)
	}

	if err := manager.Delete(session.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var closed terminalEventPayload
	if err := conn.ReadJSON(&closed); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if closed.Type != "terminal_closed" {
		t.Fatalf("expected terminal_closed, got %q", closed.Type)
	}
	if closed.TerminalID != session.ID {
		t.Fatalf("expected terminal ID %q, got %q", session.ID, closed.TerminalID)
	}
}
