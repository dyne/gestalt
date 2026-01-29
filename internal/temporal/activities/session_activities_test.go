package activities

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/logging"
	"gestalt/internal/terminal"
)

type scriptedPty struct {
	reader *bytes.Reader
}

func (pty *scriptedPty) Read(data []byte) (int, error) {
	return pty.reader.Read(data)
}

func (pty *scriptedPty) Write(data []byte) (int, error) {
	return len(data), nil
}

func (pty *scriptedPty) Close() error {
	return nil
}

func (pty *scriptedPty) Resize(cols, rows uint16) error {
	return nil
}

type scriptedFactory struct {
	output []byte
}

func (factory *scriptedFactory) Start(command string, args ...string) (terminal.Pty, *exec.Cmd, error) {
	return &scriptedPty{reader: bytes.NewReader(factory.output)}, &exec.Cmd{}, nil
}

const testAgentID = "codex"

func newTestActivities(output []byte) (*SessionActivities, *terminal.Manager) {
	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(logging.DefaultBufferSize), logging.LevelDebug, nil)
	factory := &scriptedFactory{output: output}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:           "/bin/sh",
		PtyFactory:      factory,
		BufferLines:     50,
		Logger:          logger,
		SessionLogDir:   "",
		InputHistoryDir: "",
		Agents: map[string]agent.Agent{
			testAgentID: {Name: "Codex"},
		},
	})
	return &SessionActivities{
		Manager: manager,
		Logger:  logger,
	}, manager
}

func waitForOutputLines(session *terminal.Session, minimumLines int) bool {
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if session != nil && len(session.OutputLines()) >= minimumLines {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestSessionActivitiesSpawnAndTerminate(testingContext *testing.T) {
	activities, manager := newTestActivities(nil)
	activityContext := context.Background()

	spawnError := activities.SpawnTerminalActivity(activityContext, "Codex 1", testAgentID, "/bin/sh")
	if spawnError != nil {
		testingContext.Fatalf("spawn error: %v", spawnError)
	}

	if session, ok := manager.Get("Codex 1"); !ok || session == nil {
		testingContext.Fatal("expected session to be created")
	}

	spawnError = activities.SpawnTerminalActivity(activityContext, "Codex 1", testAgentID, "/bin/sh")
	if spawnError != nil {
		testingContext.Fatalf("spawn idempotency error: %v", spawnError)
	}

	terminateError := activities.TerminateTerminalActivity(activityContext, "Codex 1")
	if terminateError != nil {
		testingContext.Fatalf("terminate error: %v", terminateError)
	}

	if _, ok := manager.Get("Codex 1"); ok {
		testingContext.Fatal("expected session to be deleted")
	}

	terminateError = activities.TerminateTerminalActivity(activityContext, "Codex 1")
	if terminateError != nil {
		testingContext.Fatalf("terminate idempotency error: %v", terminateError)
	}
}

func TestSessionActivitiesRecordBellAndUpdateTask(testingContext *testing.T) {
	activities, _ := newTestActivities(nil)
	activityContext := context.Background()

	spawnError := activities.SpawnTerminalActivity(activityContext, "Codex 1", testAgentID, "/bin/sh")
	if spawnError != nil {
		testingContext.Fatalf("spawn error: %v", spawnError)
	}

	bellContext := "bell\x1b[31m-alert\x1b[0m-----"
	bellError := activities.RecordBellActivity(activityContext, "Codex 1", time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), bellContext)
	if bellError != nil {
		testingContext.Fatalf("bell error: %v", bellError)
	}

	var bellEntry logging.LogEntry
	for _, entry := range activities.Logger.Buffer().List() {
		if entry.Message == "temporal bell recorded" {
			bellEntry = entry
			break
		}
	}
	if bellEntry.Message == "" {
		testingContext.Fatal("expected bell log entry")
	}
	contextValue := bellEntry.Context["context"]
	if strings.Contains(contextValue, "\x1b") {
		testingContext.Fatalf("expected context filtered, got %q", contextValue)
	}
	if strings.Contains(contextValue, "-----") {
		testingContext.Fatalf("expected repeated chars collapsed, got %q", contextValue)
	}

	updateError := activities.UpdateTaskActivity(activityContext, "Codex 1", "L1", "L2")
	if updateError != nil {
		testingContext.Fatalf("update error: %v", updateError)
	}
}

func TestSessionActivitiesGetOutputActivity(testingContext *testing.T) {
	output := []byte("first line\nsecond line\n")
	activities, manager := newTestActivities(output)
	activityContext := context.Background()

	spawnError := activities.SpawnTerminalActivity(activityContext, "Codex 1", testAgentID, "/bin/sh")
	if spawnError != nil {
		testingContext.Fatalf("spawn error: %v", spawnError)
	}

	session, ok := manager.Get("Codex 1")
	if !ok || session == nil {
		testingContext.Fatal("expected output session to exist")
	}
	if !waitForOutputLines(session, 2) {
		testingContext.Fatal("timed out waiting for output")
	}

	outputText, outputError := activities.GetOutputActivity(activityContext, "Codex 1")
	if outputError != nil {
		testingContext.Fatalf("output error: %v", outputError)
	}
	if !strings.Contains(outputText, "first line") {
		testingContext.Fatalf("unexpected output: %q", outputText)
	}
}

func TestSessionActivitiesGetOutputTailActivity(testingContext *testing.T) {
	output := []byte("first line\nsecond line\n")
	activities, manager := newTestActivities(output)
	activityContext := context.Background()

	spawnError := activities.SpawnTerminalActivity(activityContext, "Codex 1", testAgentID, "/bin/sh")
	if spawnError != nil {
		testingContext.Fatalf("spawn error: %v", spawnError)
	}

	session, ok := manager.Get("Codex 1")
	if !ok || session == nil {
		testingContext.Fatal("expected output session to exist")
	}
	if !waitForOutputLines(session, 2) {
		testingContext.Fatal("timed out waiting for output")
	}

	outputText, outputError := activities.GetOutputTailActivity(activityContext, "Codex 1", 1)
	if outputError != nil {
		testingContext.Fatalf("output error: %v", outputError)
	}
	if !strings.Contains(outputText, "second line") {
		testingContext.Fatalf("unexpected output: %q", outputText)
	}
	if strings.Contains(outputText, "first line") {
		testingContext.Fatalf("expected tail output, got %q", outputText)
	}
}
