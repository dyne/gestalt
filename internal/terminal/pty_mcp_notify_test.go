package terminal

import (
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/temporal/workflows"
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

func readOutputLine(t *testing.T, p Pty) string {
	t.Helper()
	deadline := time.After(2 * time.Second)
	var buf bytes.Buffer
	tmp := make([]byte, 64)
	for {
		select {
		case <-deadline:
			_ = p.Close()
			t.Fatalf("timeout waiting for output")
			return ""
		default:
		}
		n, err := p.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
			if strings.Contains(buf.String(), "\n") {
				return buf.String()
			}
		}
		if err != nil {
			return buf.String()
		}
	}
}

func TestMCPPtyNotificationIdleOutput(testingContext *testing.T) {
	basePty, _, serverOut := newPipePty()
	mcp := newMCPPty(basePty, false)

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

	out := readOutputLine(testingContext, mcp)
	if !strings.Contains(out, "[mcp codex/event] task_started: generating response") {
		testingContext.Fatalf("expected notification output, got %q", out)
	}

	_ = mcp.Close()
}

func TestMCPPtyNotificationTruncatesParams(testingContext *testing.T) {
	basePty, _, serverOut := newPipePty()
	mcp := newMCPPty(basePty, false)

	longText := strings.Repeat("a", 600)
	sendMCPNotification(testingContext, serverOut, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialized",
		"params": map[string]interface{}{
			"data": longText,
		},
	})

	out := readOutputLine(testingContext, mcp)
	if !strings.Contains(out, "[mcp initialized]") {
		testingContext.Fatalf("expected notification prefix, got %q", out)
	}
	if strings.Contains(out, longText) {
		testingContext.Fatalf("expected truncation, got %q", out)
	}
	if !strings.Contains(out, "\u2026") {
		testingContext.Fatalf("expected truncation ellipsis, got %q", out)
	}

	_ = mcp.Close()
}

func TestMCPTurnEmitsNotifySignal(testingContext *testing.T) {
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
	sessionFactory := NewSessionFactory(SessionFactoryOptions{
		PtyFactory: factory,
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
	workflowClient := &fakeWorkflowClient{runID: "run-1"}
	if err := session.StartWorkflow(workflowClient, "", ""); err != nil {
		testingContext.Fatalf("start workflow: %v", err)
	}

	if err := session.Write([]byte("hello\r")); err != nil {
		testingContext.Fatalf("write: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(workflowClient.signals) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(workflowClient.signals) != 1 {
		testingContext.Fatalf("expected 1 signal, got %d", len(workflowClient.signals))
	}
	record := workflowClient.signals[0]
	if record.signalName != workflows.NotifySignalName {
		testingContext.Fatalf("expected notify signal, got %q", record.signalName)
	}
	notify, ok := record.payload.(workflows.NotifySignal)
	if !ok {
		testingContext.Fatalf("unexpected notify payload: %#v", record.payload)
	}
	if notify.Source != "codex-notify" {
		testingContext.Fatalf("expected source codex-notify, got %q", notify.Source)
	}
	if notify.EventType != "agent-turn-complete" {
		testingContext.Fatalf("expected event type agent-turn-complete, got %q", notify.EventType)
	}
	if notify.EventID == "" {
		testingContext.Fatalf("expected event id to be set")
	}
}
