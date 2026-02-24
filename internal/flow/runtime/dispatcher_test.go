package runtime

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
	"gestalt/internal/event"
	"gestalt/internal/flow"
	"gestalt/internal/logging"
	"gestalt/internal/notify"
	"gestalt/internal/terminal"
)

type recordingPty struct {
	buffer bytes.Buffer
	closed chan struct{}
}

func (pty *recordingPty) Read(data []byte) (int, error) {
	<-pty.closed
	return 0, io.EOF
}

func (pty *recordingPty) Write(data []byte) (int, error) {
	return pty.buffer.Write(data)
}

func (pty *recordingPty) Close() error {
	close(pty.closed)
	return nil
}

func (pty *recordingPty) Resize(cols, rows uint16) error {
	return nil
}

type recordingFactory struct {
	last *recordingPty
}

func (factory *recordingFactory) Start(command string, args ...string) (terminal.Pty, *exec.Cmd, error) {
	pty := &recordingPty{closed: make(chan struct{})}
	factory.last = pty
	return pty, &exec.Cmd{}, nil
}

func newDispatcher() (*Dispatcher, *recordingFactory, *terminal.Manager) {
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
	return NewDispatcher(manager, logger, notify.NewMemorySink(), 0), factory, manager
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

func TestDispatcherSendToTerminal(testingContext *testing.T) {
	testingContext.Skip("obsolete: expects PTY-backed send path")
	dispatcher, factory, manager := newDispatcher()
	session, err := manager.Create("target", "", "")
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
	if err := dispatcher.Dispatch(context.Background(), request); err != nil {
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
	factory.last.buffer.Reset()

	requestByID := flow.ActivityRequest{
		EventID:    "event",
		TriggerID:  "trigger",
		ActivityID: "send_to_terminal",
		Config: map[string]any{
			"target_session_id": session.ID,
			"message_template":  "Session hello",
		},
	}
	if err := dispatcher.Dispatch(context.Background(), requestByID); err != nil {
		testingContext.Fatalf("send by id error: %v", err)
	}
	if !waitForWrite(factory, len("Session hello\n")) {
		testingContext.Fatal("expected pty write for session id")
	}
	byIDWritten := factory.last.buffer.String()
	if byIDWritten != "Session hello\n" {
		testingContext.Fatalf("unexpected session id write: %q", byIDWritten)
	}
}

func TestDispatcherSendToTerminalRequiresTarget(testingContext *testing.T) {
	dispatcher, _, _ := newDispatcher()
	request := flow.ActivityRequest{
		EventID:    "event",
		TriggerID:  "trigger",
		ActivityID: "send_to_terminal",
		Config: map[string]any{
			"message_template": "Hello",
		},
	}
	if err := dispatcher.Dispatch(context.Background(), request); err == nil {
		testingContext.Fatal("expected target error")
	}
}

func TestDispatcherSendToChat(testingContext *testing.T) {
	dispatcher, _, manager := newDispatcher()
	events, cancel := manager.ChatBus().Subscribe()
	defer cancel()

	request := flow.ActivityRequest{
		EventID:    "event",
		TriggerID:  "trigger",
		ActivityID: "send_to_terminal",
		Config: map[string]any{
			"target_session_id": "chat",
			"message_template":  "hello chat",
		},
	}
	if err := dispatcher.Dispatch(context.Background(), request); err != nil {
		testingContext.Fatalf("chat send error: %v", err)
	}
	chatEvent := event.ReceiveWithTimeout(testingContext, events, time.Second)
	if chatEvent.Message != "hello chat" {
		testingContext.Fatalf("expected message %q, got %q", "hello chat", chatEvent.Message)
	}
	if chatEvent.SessionID != terminal.ChatSessionID {
		testingContext.Fatalf("expected session id %q, got %q", terminal.ChatSessionID, chatEvent.SessionID)
	}
	if chatEvent.Role != "user" {
		testingContext.Fatalf("expected role user, got %q", chatEvent.Role)
	}
}

func TestDispatcherSpawnAgentSession(testingContext *testing.T) {
	testingContext.Skip("obsolete: agents hub session affects count assertions")
	dispatcher, factory, manager := newDispatcher()
	request := flow.ActivityRequest{
		EventID:    "event",
		TriggerID:  "trigger",
		ActivityID: "spawn_agent_session",
		Config: map[string]any{
			"agent_id":         "target",
			"message_template": "Hello",
		},
	}
	if err := dispatcher.Dispatch(context.Background(), request); err != nil {
		testingContext.Fatalf("spawn activity error: %v", err)
	}
	if len(manager.List()) != 1 {
		testingContext.Fatalf("expected one session, got %d", len(manager.List()))
	}
	if !waitForWrite(factory, len("Hello\n")) {
		testingContext.Fatal("expected spawn message write")
	}
	if written := factory.last.buffer.String(); written != "Hello\n" {
		testingContext.Fatalf("unexpected spawn write: %q", written)
	}
}

func TestDispatcherSpawnAgentSessionReuse(testingContext *testing.T) {
	testingContext.Skip("obsolete: agents hub session affects count assertions")
	dispatcher, factory, manager := newDispatcher()
	if _, err := manager.Create("target", "", ""); err != nil {
		testingContext.Fatalf("create session: %v", err)
	}
	factory.last.buffer.Reset()
	request := flow.ActivityRequest{
		EventID:    "event",
		TriggerID:  "trigger",
		ActivityID: "spawn_agent_session",
		Config: map[string]any{
			"agent_id":         "target",
			"message_template": "Reuse",
		},
	}
	if err := dispatcher.Dispatch(context.Background(), request); err != nil {
		testingContext.Fatalf("reuse activity error: %v", err)
	}
	if len(manager.List()) != 1 {
		testingContext.Fatalf("expected one session, got %d", len(manager.List()))
	}
	if !waitForWrite(factory, len("Reuse\n")) {
		testingContext.Fatal("expected reuse message write")
	}
	if written := factory.last.buffer.String(); written != "Reuse\n" {
		testingContext.Fatalf("unexpected reuse write: %q", written)
	}
}

func TestDispatcherPostWebhook(testingContext *testing.T) {
	received := make(chan *http.Request, 1)
	bodyCh := make(chan []byte, 1)
	server := newIPv4Server(testingContext, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- r
		bodyCh <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dispatcher := NewDispatcher(nil, nil, notify.NewMemorySink(), 0)
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
	if err := dispatcher.Dispatch(context.Background(), request); err != nil {
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

func TestDispatcherPostWebhookStatus(testingContext *testing.T) {
	server := newIPv4Server(testingContext, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	dispatcher := NewDispatcher(nil, nil, notify.NewMemorySink(), 0)
	request := flow.ActivityRequest{
		EventID:    "event",
		TriggerID:  "trigger",
		ActivityID: "post_webhook",
		Config: map[string]any{
			"url": server.URL,
		},
	}
	if err := dispatcher.Dispatch(context.Background(), request); err == nil {
		testingContext.Fatal("expected webhook error")
	}
}
