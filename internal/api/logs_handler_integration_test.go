package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gestalt/internal/logging"

	"github.com/gorilla/websocket"
)

func TestLogsWebSocketStream(t *testing.T) {
	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(10), logging.LevelInfo, nil)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &LogsHandler{Logger: logger}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/logs"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	logger.Info("hello", map[string]string{"component": "test"})

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var entry logging.LogEntry
	if err := conn.ReadJSON(&entry); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if entry.Message != "hello" {
		t.Fatalf("expected message hello, got %q", entry.Message)
	}
	if entry.Level != logging.LevelInfo {
		t.Fatalf("expected level info, got %q", entry.Level)
	}
	if entry.Context["component"] != "test" {
		t.Fatalf("expected context component=test, got %v", entry.Context)
	}
}

func TestLogsWebSocketAuth(t *testing.T) {
	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(10), logging.LevelInfo, nil)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config: &http.Server{Handler: &LogsHandler{
			Logger:    logger,
			AuthToken: "secret",
		}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/logs"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatalf("expected unauthorized websocket dial to fail")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %v", resp)
	}

	wsURL = wsURL + "?token=secret"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket with token: %v", err)
	}
	conn.Close()
}

func TestLogsWebSocketQueryFilter(t *testing.T) {
	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(10), logging.LevelInfo, nil)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &LogsHandler{Logger: logger}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/logs?level=warning"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	logger.Info("info msg", nil)
	logger.Warn("warn msg", nil)

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var entry logging.LogEntry
	if err := conn.ReadJSON(&entry); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if entry.Level != logging.LevelWarning {
		t.Fatalf("expected warning, got %q", entry.Level)
	}
	if entry.Message != "warn msg" {
		t.Fatalf("expected warn msg, got %q", entry.Message)
	}
}

func TestLogsWebSocketMessageFilter(t *testing.T) {
	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(10), logging.LevelInfo, nil)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &LogsHandler{Logger: logger}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/logs"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"level":"error"}`)); err != nil {
		t.Fatalf("write filter message: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	logger.Warn("warn msg", nil)
	logger.Error("error msg", nil)

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var entry logging.LogEntry
	if err := conn.ReadJSON(&entry); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if entry.Level != logging.LevelError {
		t.Fatalf("expected error, got %q", entry.Level)
	}
	if entry.Message != "error msg" {
		t.Fatalf("expected error msg, got %q", entry.Message)
	}
}
