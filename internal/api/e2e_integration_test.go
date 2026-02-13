package api

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/logging"
	"gestalt/internal/terminal"

	"github.com/gorilla/websocket"
)

func TestEndToEndTerminalFlow(t *testing.T) {
	factory := &testFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		AgentsDir:  ensureTestAgentsDir(),
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex"},
		},
	})
	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(10), logging.LevelInfo, nil)

	mux := http.NewServeMux()
	RegisterRoutes(mux, manager, "secret", StatusConfig{}, "", nil, logger, nil)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping e2e test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: mux},
	}
	server.Start()
	defer server.Close()

	createReq, err := http.NewRequest(http.MethodPost, server.URL+"/api/sessions", strings.NewReader(`{"title":"e2e","agent":"codex","runner":"server"}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	createReq.Header.Set("Authorization", "Bearer secret")
	createReq.Header.Set("Content-Type", "application/json")
	createRes, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	defer createRes.Body.Close()
	if createRes.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRes.StatusCode)
	}

	var summary terminalSummary
	if err := json.NewDecoder(createRes.Body).Decode(&summary); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if summary.ID == "" {
		t.Fatalf("expected terminal id")
	}
	if summary.Interface == "" {
		t.Fatalf("expected interface in terminal summary")
	}
	defer func() {
		_ = manager.Delete(summary.ID)
	}()

	factory.mu.Lock()
	pty := factory.ptys[0]
	factory.mu.Unlock()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/session/" + url.PathEscape(summary.ID) + "?token=secret"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	session, ok := manager.Get(summary.ID)
	if !ok {
		t.Fatalf("expected session to be available")
	}
	waitForSubscribers(t, session, 1, 2*time.Second)

	interactionTimeout := time.Second
	readyPayload := []byte("ready\n")
	if err := conn.WriteMessage(websocket.BinaryMessage, readyPayload); err != nil {
		t.Fatalf("write readiness payload: %v", err)
	}
	if !pty.waitForWrite(readyPayload, interactionTimeout) {
		t.Fatalf("expected PTY to receive readiness payload")
	}

	if err := pty.emitOutput([]byte("hello\n")); err != nil {
		t.Fatalf("emit output: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(interactionTimeout))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if !bytes.Contains(msg, []byte("hello")) {
		t.Fatalf("expected output to contain hello, got %q", string(msg))
	}

	payload := []byte("ls\n")
	if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatalf("write websocket: %v", err)
	}
	if !pty.waitForWrite(payload, interactionTimeout) {
		t.Fatalf("expected PTY to receive %q", string(payload))
	}
}

func TestEndToEndAuthRequired(t *testing.T) {
	manager := terminal.NewManager(terminal.ManagerOptions{Shell: "/bin/sh"})
	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(10), logging.LevelInfo, nil)
	mux := http.NewServeMux()
	RegisterRoutes(mux, manager, "secret", StatusConfig{}, "", nil, logger, nil)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping e2e test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: mux},
	}
	server.Start()
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/status", nil)
	if err != nil {
		t.Fatalf("status request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("status response: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.StatusCode)
	}
	var errPayload errorResponse
	if err := json.NewDecoder(res.Body).Decode(&errPayload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if errPayload.Code != "unauthorized" {
		t.Fatalf("expected error code unauthorized, got %q", errPayload.Code)
	}
	if errPayload.Message == "" {
		t.Fatalf("expected error message to be set")
	}

	req, err = http.NewRequest(http.MethodGet, server.URL+"/api/status", nil)
	if err != nil {
		t.Fatalf("status request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer secret")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("status response: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
}
