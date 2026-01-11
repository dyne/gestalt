package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/terminal"

	"github.com/gorilla/websocket"
)

func TestWorkflowEventsWebSocketStream(t *testing.T) {
	factory := &fakeFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &WorkflowEventsHandler{Manager: manager}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/workflows/events"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	workflowEvent := event.WorkflowEvent{
		EventType:  "workflow_started",
		WorkflowID: "workflow-123",
		SessionID:  "session-123",
		OccurredAt: time.Now().UTC(),
	}
	manager.WorkflowBus().Publish(workflowEvent)

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var payload workflowEventPayload
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if payload.Type != workflowEvent.EventType {
		t.Fatalf("expected event type %q, got %q", workflowEvent.EventType, payload.Type)
	}
	if payload.WorkflowID != workflowEvent.WorkflowID {
		t.Fatalf("expected workflow ID %q, got %q", workflowEvent.WorkflowID, payload.WorkflowID)
	}
	if payload.SessionID != workflowEvent.SessionID {
		t.Fatalf("expected session ID %q, got %q", workflowEvent.SessionID, payload.SessionID)
	}
	if payload.Timestamp.IsZero() {
		t.Fatal("expected timestamp to be set")
	}
}
