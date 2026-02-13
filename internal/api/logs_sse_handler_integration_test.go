package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/logging"
	"gestalt/internal/otel"
	"gestalt/internal/temporal/activities"
	"gestalt/internal/terminal"
)

func TestLogsSSEStreamFiltersAndSanitizes(t *testing.T) {
	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(50), logging.LevelDebug, nil)
	hub := otel.NewLogHub(time.Hour)
	previous := otel.ActiveLogHub()
	otel.SetActiveLogHub(hub)
	stopFallback := otel.StartLogHubFallback(logger, otel.SDKOptions{ServiceName: "gestalt-test"})
	t.Cleanup(func() {
		stopFallback()
		otel.SetActiveLogHub(previous)
	})

	factory := &ansiPtyFactory{
		output:   "ok\x1b[31mred\x1b[0m\n-----\n",
		closeErr: errors.New("pty close failed"),
	}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:       "/bin/sh",
		PtyFactory:  factory,
		BufferLines: 20,
		Logger:      logger,
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex"},
		},
	})

	session, err := manager.Create("codex", "role", "title")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if !waitForSessionOutput(session, 2) {
		t.Fatalf("timed out waiting for session output")
	}

	activity := activities.NewSessionActivities(manager, logger, 0)
	bellErr := activity.RecordBellActivity(context.Background(), session.ID, time.Now().UTC(), "bell\x1b[31m-alert\x1b[0m-----")
	if bellErr != nil {
		t.Fatalf("record bell: %v", bellErr)
	}

	if err := manager.Delete(session.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	server := newSSETestServer(t, &LogsSSEHandler{Logger: logger})
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/logs/stream?level=warning")
	if err != nil {
		t.Fatalf("get sse stream: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	entry, err := readSSELogEntryWithMessage(reader, "session close error", 750*time.Millisecond)
	if err != nil {
		t.Fatalf("read close entry: %v", err)
	}
	assertFilteredContext(t, attrValue(entry, "output_tail"))
	if attrValue(entry, "gestalt.replay_window") != "1h" {
		t.Fatalf("expected replay_window attribute")
	}
}

func readSSELogEntryWithMessage(reader *bufio.Reader, message string, timeout time.Duration) (map[string]any, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		frame, err := readSSEFrameWithTimeout(reader, remaining)
		if err != nil {
			return nil, err
		}
		if len(frame.Data) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(frame.Data, &entry); err != nil {
			continue
		}
		if logBody(entry) == message {
			return entry, nil
		}
	}
	return nil, errors.New("log entry not found")
}
