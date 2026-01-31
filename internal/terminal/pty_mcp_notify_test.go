package terminal

import (
	"os/exec"
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
