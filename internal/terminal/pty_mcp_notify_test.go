package terminal

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/notify"
)

type staticPtyFactory struct {
	pty Pty
}

func (f *staticPtyFactory) Start(command string, args ...string) (Pty, *exec.Cmd, error) {
	return f.pty, &exec.Cmd{}, nil
}

func sendMCPNotification(t *testing.T, out io.Writer, payload interface{}) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	_, _ = out.Write(append(data, '\n'))
}

func TestMCPPtyNotificationLogsToFile(testingContext *testing.T) {
	basePty, _, serverOut := newPipePty()
	mcp := newMCPPty(basePty, false)
	logger, err := newMCPEventLogger(testingContext.TempDir(), "session-1", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		testingContext.Fatalf("create logger: %v", err)
	}
	mcp.SetEventLogger(logger)

	sendMCPNotification(testingContext, serverOut, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "codex/event",
		"params": map[string]interface{}{
			"msg": map[string]interface{}{
				"type":    "task_started",
				"message": "generating response",
			},
		},
	})

	waitForEventLog(testingContext, logger.Path(), `"method":"codex/event"`)
	waitForEventLog(testingContext, logger.Path(), `"type":"task_started"`)

	_ = mcp.Close()
}

func TestMCPPtyNotificationLogsNonCodex(testingContext *testing.T) {
	basePty, _, serverOut := newPipePty()
	mcp := newMCPPty(basePty, false)
	logger, err := newMCPEventLogger(testingContext.TempDir(), "session-1", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		testingContext.Fatalf("create logger: %v", err)
	}
	mcp.SetEventLogger(logger)

	sendMCPNotification(testingContext, serverOut, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialized",
		"params": map[string]interface{}{
			"data": "ready",
		},
	})

	waitForEventLog(testingContext, logger.Path(), `"method":"initialized"`)
	waitForEventLog(testingContext, logger.Path(), `"data":"ready"`)

	_ = mcp.Close()
}

func TestMCPTurnEmitsNotificationEvent(testingContext *testing.T) {
	basePty, serverIn, serverOut := newPipePty()
	server := newFakeMCPServer(testingContext, serverIn, serverOut, nil)
	server.onCall = func(id int64, name string, args map[string]interface{}) {
		server.send(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"result": map[string]interface{}{
				"threadId": "thread-1",
				"content":  "done",
			},
		})
	}
	go server.run()

	mcp := newMCPPty(basePty, false)
	factory := &staticPtyFactory{pty: mcp}
	sink := notify.NewMemorySink()
	sessionFactory := NewSessionFactory(SessionFactoryOptions{
		PtyFactory:       factory,
		NotificationSink: sink,
	})

	profile := &agent.Agent{
		Name:    "Codex",
		CLIType: "codex",
	}
	session, _, err := sessionFactory.Start(sessionCreateRequest{
		AgentID: "codex",
		Title:   "Codex",
	}, profile, "codex mcp-server", "session-1")
	if err != nil {
		testingContext.Fatalf("start session: %v", err)
	}

	if err := session.Write([]byte("hello\r")); err != nil {
		testingContext.Fatalf("write: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(sink.Events()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	events := sink.Events()
	if len(events) != 1 {
		testingContext.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Fields["notify.type"] != "agent-turn-complete" {
		testingContext.Fatalf("expected notify.type agent-turn-complete, got %q", events[0].Fields["notify.type"])
	}
	if events[0].Fields["notify.event_id"] == "" {
		testingContext.Fatalf("expected notify.event_id to be set")
	}
}

func waitForEventLog(t *testing.T, path string, contains string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		payload, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(payload), contains) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for event log %s to contain %q", path, contains)
}
