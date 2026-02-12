package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gestalt/internal/terminal"

	"github.com/gorilla/websocket"
)

func TestRunnerHandlerBridgesIO(t *testing.T) {
	manager := newTerminalTestManager(terminal.ManagerOptions{})
	session := terminal.NewExternalSession("runner-1", "title", "role", time.Now(), 10, 0, terminal.OutputBackpressureBlock, 0, nil, nil, nil)
	manager.RegisterSession(session)

	mux := http.NewServeMux()
	mux.Handle("/ws/runner/session/", &RunnerHandler{Manager: manager})
	mux.Handle("/ws/session/", &TerminalHandler{Manager: manager})
	rest := &RestHandler{Manager: manager}
	mux.Handle("/api/sessions/", restHandler("", nil, rest.handleTerminal))

	server := httptest.NewServer(mux)
	defer server.Close()

	runnerURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/runner/session/" + escapeTerminalID(session.ID)
	runnerConn, _, err := websocket.DefaultDialer.Dial(runnerURL, nil)
	if err != nil {
		t.Fatalf("runner websocket dial: %v", err)
	}
	defer runnerConn.Close()

	terminalURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/session/" + escapeTerminalID(session.ID)
	terminalConn, _, err := websocket.DefaultDialer.Dial(terminalURL, nil)
	if err != nil {
		t.Fatalf("terminal websocket dial: %v", err)
	}
	defer terminalConn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if session.SubscriberCount() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if session.SubscriberCount() == 0 {
		t.Fatalf("expected terminal websocket to subscribe before publishing output")
	}

	outputPayload := []byte("hello\n")
	if err := runnerConn.WriteMessage(websocket.BinaryMessage, outputPayload); err != nil {
		t.Fatalf("runner output write: %v", err)
	}
	_ = terminalConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, outputMsg, err := terminalConn.ReadMessage()
	if err != nil {
		t.Fatalf("terminal output read: %v", err)
	}
	if !bytes.Equal(outputMsg, outputPayload) {
		t.Fatalf("unexpected output payload: %q", string(outputMsg))
	}

	outputResp, err := http.Get(server.URL + "/api/sessions/" + escapeTerminalID(session.ID) + "/output")
	if err != nil {
		t.Fatalf("output api request: %v", err)
	}
	defer outputResp.Body.Close()

	var output terminalOutputResponse
	if err := json.NewDecoder(outputResp.Body).Decode(&output); err != nil {
		t.Fatalf("decode output api response: %v", err)
	}
	joined := strings.Join(output.Lines, "\n")
	if !strings.Contains(joined, "hello") {
		t.Fatalf("expected output lines to include hello, got %q", joined)
	}

	inputPayload := []byte("ls\n")
	if err := terminalConn.WriteMessage(websocket.BinaryMessage, inputPayload); err != nil {
		t.Fatalf("terminal input write: %v", err)
	}
	_ = runnerConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, inputMsg, err := runnerConn.ReadMessage()
	if err != nil {
		t.Fatalf("runner input read: %v", err)
	}
	if !bytes.Equal(inputMsg, inputPayload) {
		t.Fatalf("unexpected input payload: %q", string(inputMsg))
	}
}

func TestRunnerHandlerDeletesSessionOnDisconnect(t *testing.T) {
	manager := newTerminalTestManager(terminal.ManagerOptions{})
	session := terminal.NewExternalSession("runner-2", "title", "role", time.Now(), 10, 0, terminal.OutputBackpressureBlock, 0, nil, nil, nil)
	manager.RegisterSession(session)

	mux := http.NewServeMux()
	mux.Handle("/ws/runner/session/", &RunnerHandler{Manager: manager})
	server := httptest.NewServer(mux)
	defer server.Close()

	runnerURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/runner/session/" + escapeTerminalID(session.ID)
	runnerConn, _, err := websocket.DefaultDialer.Dial(runnerURL, nil)
	if err != nil {
		t.Fatalf("runner websocket dial: %v", err)
	}
	_ = runnerConn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := manager.Get(session.ID); !ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected session to be deleted after runner disconnect")
}
