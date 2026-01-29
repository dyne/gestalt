package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/flow"
	"gestalt/internal/logging"
	"gestalt/internal/terminal"
)

type recordingPty struct {
	buffer bytes.Buffer
}

func (pty *recordingPty) Read(data []byte) (int, error) {
	return 0, io.EOF
}

func (pty *recordingPty) Write(data []byte) (int, error) {
	return pty.buffer.Write(data)
}

func (pty *recordingPty) Close() error {
	return nil
}

func (pty *recordingPty) Resize(cols, rows uint16) error {
	return nil
}

type recordingFactory struct {
	last *recordingPty
}

func (factory *recordingFactory) Start(command string, args ...string) (terminal.Pty, *exec.Cmd, error) {
	pty := &recordingPty{}
	factory.last = pty
	return pty, &exec.Cmd{}, nil
}

func newFlowActivities() (*FlowActivities, *recordingFactory, *terminal.Manager) {
	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(logging.DefaultBufferSize), logging.LevelDebug, nil)
	factory := &recordingFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:       "/bin/sh",
		PtyFactory:  factory,
		BufferLines: 10,
		Agents: map[string]agent.Agent{
			"target": {Name: "Target"},
		},
		Logger: logger,
	})
	return NewFlowActivities(manager, logger), factory, manager
}

func waitForWrite(factory *recordingFactory, minBytes int) bool {
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if factory.last != nil && factory.last.buffer.Len() >= minBytes {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func newIPv4Server(testingContext *testing.T, handler http.Handler) *httptest.Server {
	testingContext.Helper()
	server := httptest.NewUnstartedServer(handler)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		testingContext.Fatalf("listen: %v", err)
	}
	server.Listener = listener
	server.Start()
	return server
}

func TestFlowActivitiesSendToTerminal(testingContext *testing.T) {
	activities, factory, manager := newFlowActivities()
	_, err := manager.Create("target", "", "")
	if err != nil {
		testingContext.Fatalf("create session: %v", err)
	}

	request := flow.ActivityRequest{
		EventID:    "event",
		TriggerID:  "trigger",
		ActivityID: "send_to_terminal",
		Config: map[string]any{
			"target_agent_name": "Target",
			"message_template":  "Hello",
		},
		OutputTail: "tail output",
	}
	if err := activities.SendToTerminalActivity(context.Background(), request); err != nil {
		testingContext.Fatalf("send activity error: %v", err)
	}
	if !waitForWrite(factory, 1) {
		testingContext.Fatal("expected pty write")
	}
	written := factory.last.buffer.String()
	expected := "Hello\n\ntail output\n"
	if written != expected {
		testingContext.Fatalf("unexpected write: %q", written)
	}
}

func TestFlowActivitiesPostWebhook(testingContext *testing.T) {
	received := make(chan *http.Request, 1)
	bodyCh := make(chan []byte, 1)
	server := newIPv4Server(testingContext, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- r
		bodyCh <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	activities := NewFlowActivities(nil, nil)
	request := flow.ActivityRequest{
		EventID:    "event",
		TriggerID:  "trigger",
		ActivityID: "post_webhook",
		Event: map[string]string{
			"type": "workflow_paused",
		},
		Config: map[string]any{
			"url":          server.URL,
			"headers_json": `{"X-Custom":"yes"}`,
		},
	}
	if err := activities.PostWebhookActivity(context.Background(), request); err != nil {
		testingContext.Fatalf("webhook error: %v", err)
	}

	select {
	case req := <-received:
		if req.Header.Get("X-Custom") != "yes" {
			testingContext.Fatalf("expected custom header, got %q", req.Header.Get("X-Custom"))
		}
		expectedKey := flow.BuildIdempotencyKey(request.EventID, request.TriggerID, request.ActivityID)
		if req.Header.Get("Idempotency-Key") != expectedKey {
			testingContext.Fatalf("expected idempotency key %q, got %q", expectedKey, req.Header.Get("Idempotency-Key"))
		}
	case <-time.After(200 * time.Millisecond):
		testingContext.Fatal("timed out waiting for webhook")
	}

	select {
	case body := <-bodyCh:
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			testingContext.Fatalf("invalid payload: %v", err)
		}
		if payload["event_id"] != "event" {
			testingContext.Fatalf("expected event_id, got %v", payload["event_id"])
		}
	case <-time.After(200 * time.Millisecond):
		testingContext.Fatal("timed out waiting for payload")
	}
}

func TestFlowActivitiesPostWebhookStatus(testingContext *testing.T) {
	server := newIPv4Server(testingContext, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	activities := NewFlowActivities(nil, nil)
	request := flow.ActivityRequest{
		EventID:    "event",
		TriggerID:  "trigger",
		ActivityID: "post_webhook",
		Config: map[string]any{
			"url": server.URL,
		},
	}
	if err := activities.PostWebhookActivity(context.Background(), request); err == nil {
		testingContext.Fatal("expected webhook error")
	}
}
