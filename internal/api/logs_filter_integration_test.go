package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/temporal/activities"
	"gestalt/internal/terminal"

	"github.com/gorilla/websocket"
)

type ansiPty struct {
	reader   *strings.Reader
	closeErr error
}

func (p *ansiPty) Read(data []byte) (int, error) {
	return p.reader.Read(data)
}

func (p *ansiPty) Write(data []byte) (int, error) {
	return len(data), nil
}

func (p *ansiPty) Close() error {
	return p.closeErr
}

func (p *ansiPty) Resize(cols, rows uint16) error {
	return nil
}

type ansiPtyFactory struct {
	output   string
	closeErr error
}

func (f *ansiPtyFactory) Start(command string, args ...string) (terminal.Pty, *exec.Cmd, error) {
	return &ansiPty{reader: strings.NewReader(f.output), closeErr: f.closeErr}, nil, nil
}

func waitForSessionOutput(session *terminal.Session, minimumLines int) bool {
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if session != nil && len(session.OutputLines()) >= minimumLines {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func findLogEntry(entries []logging.LogEntry, message string) (logging.LogEntry, bool) {
	for _, entry := range entries {
		if entry.Message == message {
			return entry, true
		}
	}
	return logging.LogEntry{}, false
}

func readLogEntryWithMessage(conn *websocket.Conn, message string, timeout time.Duration) (logging.LogEntry, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(deadline)
		var entry logging.LogEntry
		if err := conn.ReadJSON(&entry); err != nil {
			return logging.LogEntry{}, err
		}
		if entry.Message == message {
			return entry, nil
		}
	}
	return logging.LogEntry{}, errors.New("log entry not found")
}

func assertFilteredContext(t *testing.T, value string) {
	t.Helper()
	if strings.Contains(value, "\x1b") {
		t.Fatalf("expected ANSI stripped, got %q", value)
	}
	if strings.Contains(value, "-----") {
		t.Fatalf("expected repeated chars collapsed, got %q", value)
	}
}

func TestLogsFilteringWebSocketAndREST(t *testing.T) {
	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(50), logging.LevelDebug, nil)
	factory := &ansiPtyFactory{
		output:   "ok\x1b[31mred\x1b[0m\n-----\n",
		closeErr: errors.New("pty close failed"),
	}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:       "/bin/sh",
		PtyFactory:  factory,
		BufferLines: 20,
		Logger:      logger,
	})

	session, err := manager.Create("", "role", "title")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if !waitForSessionOutput(session, 2) {
		t.Fatalf("timed out waiting for session output")
	}

	activity := activities.NewSessionActivities(manager, logger)
	bellErr := activity.RecordBellActivity(context.Background(), session.ID, time.Now().UTC(), "bell\x1b[31m-alert\x1b[0m-----")
	if bellErr != nil {
		t.Fatalf("record bell: %v", bellErr)
	}

	if err := manager.Delete(session.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}

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

	bellEntry, err := readLogEntryWithMessage(conn, "temporal bell recorded", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("read bell entry: %v", err)
	}
	assertFilteredContext(t, bellEntry.Context["context"])

	closeEntry, err := readLogEntryWithMessage(conn, "terminal close error", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("read close entry: %v", err)
	}
	assertFilteredContext(t, closeEntry.Context["output_tail"])

	handler := &RestHandler{Logger: logger}
	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()

	restHandler("secret", handler.handleLogs)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload []logging.LogEntry
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode logs: %v", err)
	}
	bellEntry, ok := findLogEntry(payload, "temporal bell recorded")
	if !ok {
		t.Fatalf("expected bell entry in REST logs")
	}
	assertFilteredContext(t, bellEntry.Context["context"])

	closeEntry, ok = findLogEntry(payload, "terminal close error")
	if !ok {
		t.Fatalf("expected close error entry in REST logs")
	}
	assertFilteredContext(t, closeEntry.Context["output_tail"])
}
