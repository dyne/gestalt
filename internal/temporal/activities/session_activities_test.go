package activities

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

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

	spawnError := activities.SpawnTerminalActivity(activityContext, "activity-session", "/bin/sh")
	if spawnError != nil {
		testingContext.Fatalf("spawn error: %v", spawnError)
	}

	if session, ok := manager.Get("activity-session"); !ok || session == nil {
		testingContext.Fatal("expected session to be created")
	}

	spawnError = activities.SpawnTerminalActivity(activityContext, "activity-session", "/bin/sh")
	if spawnError != nil {
		testingContext.Fatalf("spawn idempotency error: %v", spawnError)
	}

	terminateError := activities.TerminateTerminalActivity(activityContext, "activity-session")
	if terminateError != nil {
		testingContext.Fatalf("terminate error: %v", terminateError)
	}

	if _, ok := manager.Get("activity-session"); ok {
		testingContext.Fatal("expected session to be deleted")
	}

	terminateError = activities.TerminateTerminalActivity(activityContext, "activity-session")
	if terminateError != nil {
		testingContext.Fatalf("terminate idempotency error: %v", terminateError)
	}
}

func TestSessionActivitiesRecordBellAndUpdateTask(testingContext *testing.T) {
	activities, _ := newTestActivities(nil)
	activityContext := context.Background()

	spawnError := activities.SpawnTerminalActivity(activityContext, "bell-session", "/bin/sh")
	if spawnError != nil {
		testingContext.Fatalf("spawn error: %v", spawnError)
	}

	bellError := activities.RecordBellActivity(activityContext, "bell-session", time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), "bell")
	if bellError != nil {
		testingContext.Fatalf("bell error: %v", bellError)
	}

	updateError := activities.UpdateTaskActivity(activityContext, "bell-session", "L1", "L2")
	if updateError != nil {
		testingContext.Fatalf("update error: %v", updateError)
	}
}

func TestSessionActivitiesGetOutputActivity(testingContext *testing.T) {
	output := []byte("first line\nsecond line\n")
	activities, manager := newTestActivities(output)
	activityContext := context.Background()

	spawnError := activities.SpawnTerminalActivity(activityContext, "output-session", "/bin/sh")
	if spawnError != nil {
		testingContext.Fatalf("spawn error: %v", spawnError)
	}

	session, ok := manager.Get("output-session")
	if !ok || session == nil {
		testingContext.Fatal("expected output session to exist")
	}
	if !waitForOutputLines(session, 2) {
		testingContext.Fatal("timed out waiting for output")
	}

	outputText, outputError := activities.GetOutputActivity(activityContext, "output-session")
	if outputError != nil {
		testingContext.Fatalf("output error: %v", outputError)
	}
	if !strings.Contains(outputText, "first line") {
		testingContext.Fatalf("unexpected output: %q", outputText)
	}
}
