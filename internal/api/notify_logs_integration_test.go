package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/logging"
	"gestalt/internal/notify"
	"gestalt/internal/otel"
	"gestalt/internal/terminal"
)

func TestNotifyEndpointPublishesToLogsSSEReplayAndLive(t *testing.T) {
	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(100), logging.LevelDebug, nil)
	hub := otel.NewLogHub(time.Hour)
	previous := otel.ActiveLogHub()
	otel.SetActiveLogHub(hub)
	stopFallback := otel.StartLogHubFallback(logger, otel.SDKOptions{ServiceName: "gestalt-test"})
	t.Cleanup(func() {
		stopFallback()
		otel.SetActiveLogHub(previous)
	})

	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &fakeFactory{},
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex", Shell: "/bin/bash", CLIType: "codex"},
		},
	})
	session, err := manager.CreateWithOptions(terminal.CreateOptions{AgentID: "codex"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() {
		_ = manager.Delete(session.ID)
	})

	rest := &RestHandler{Manager: manager, Logger: logger, NotificationSink: notify.NewMemorySink()}
	mux := http.NewServeMux()
	mux.Handle("/api/logs/stream", &LogsSSEHandler{Logger: logger})
	mux.HandleFunc("/api/sessions/", restHandler("", nil, rest.handleTerminal))
	server := newSSETestServer(t, mux)
	defer server.Close()

	if err := postNotify(server.URL, session.ID, "manual:preconnect"); err != nil {
		t.Fatalf("post preconnect notify: %v", err)
	}

	resp, err := http.Get(server.URL + "/api/logs/stream?level=info")
	if err != nil {
		t.Fatalf("get logs stream: %v", err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)

	replayEntry, err := readNotifyLogEntryByEventID(reader, "manual:preconnect", time.Second)
	if err != nil {
		t.Fatalf("read replay notify entry: %v", err)
	}
	if attrValue(replayEntry, "gestalt.category") != "notification" {
		t.Fatalf("expected notification category, got %q", attrValue(replayEntry, "gestalt.category"))
	}
	if attrValue(replayEntry, "notify.type") != "progress" {
		t.Fatalf("expected notify.type progress, got %q", attrValue(replayEntry, "notify.type"))
	}
	if attrValue(replayEntry, "session.id") != session.ID {
		t.Fatalf("expected session.id %q, got %q", session.ID, attrValue(replayEntry, "session.id"))
	}
	if attrValue(replayEntry, "gestalt.replay_window") != "1h" {
		t.Fatalf("expected replay marker, got %q", attrValue(replayEntry, "gestalt.replay_window"))
	}

	if err := postNotify(server.URL, session.ID, "manual:live"); err != nil {
		t.Fatalf("post live notify: %v", err)
	}

	liveEntry, err := readNotifyLogEntryByEventID(reader, "manual:live", time.Second)
	if err != nil {
		t.Fatalf("read live notify entry: %v", err)
	}
	if attrValue(liveEntry, "notify.dispatch") != "flow_unavailable" {
		t.Fatalf("expected flow_unavailable dispatch, got %q", attrValue(liveEntry, "notify.dispatch"))
	}
}

func postNotify(baseURL, sessionID, eventID string) error {
	body := map[string]any{
		"session_id": sessionID,
		"event_id":   eventID,
		"payload": map[string]any{
			"type":       "progress",
			"plan_file":  "plan.org",
			"task_level": 1,
			"task_state": "WIP",
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := http.Post(baseURL+"/api/sessions/"+sessionID+"/notify", "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return errors.New("unexpected status")
	}
	return nil
}

func readNotifyLogEntryByEventID(reader *bufio.Reader, eventID string, timeout time.Duration) (map[string]any, error) {
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
		if logBody(entry) != "notify event accepted" {
			continue
		}
		if attrValue(entry, "notify.event_id") == eventID {
			return entry, nil
		}
	}
	return nil, errors.New("notify log entry not found")
}
