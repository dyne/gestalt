package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"gestalt/internal/config"
	"gestalt/internal/event"
	"gestalt/internal/terminal"
	"gestalt/internal/watcher"
)

func TestEventsSSEStreamDeliversWatcherConfigWorkflow(t *testing.T) {
	bus := event.NewBus[watcher.Event](context.Background(), event.BusOptions{Name: "watcher_events"})
	defer bus.Close()

	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &fakeFactory{},
	})

	server := newSSEEventsServer(t, &EventsSSEHandler{Bus: bus, Manager: manager})
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/events/stream")
	if err != nil {
		t.Fatalf("get sse stream: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)

	timestamp := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	bus.Publish(watcher.Event{
		Type:      watcher.EventTypeFileChanged,
		Path:      filepath.Join(".gestalt", "plans", "example.org"),
		Timestamp: timestamp,
	})

	data := readSSEDataFrame(t, reader, time.Second)
	var eventPayload eventPayload
	if err := json.Unmarshal(data, &eventPayload); err != nil {
		t.Fatalf("decode watcher payload: %v", err)
	}
	if eventPayload.Type != watcher.EventTypeFileChanged {
		t.Fatalf("expected type %q, got %q", watcher.EventTypeFileChanged, eventPayload.Type)
	}
	if eventPayload.Path != filepath.Join(".gestalt", "plans", "example.org") {
		t.Fatalf("expected path .gestalt/plans/example.org, got %q", eventPayload.Path)
	}
	if !eventPayload.Timestamp.Equal(timestamp) {
		t.Fatalf("expected timestamp %v, got %v", timestamp, eventPayload.Timestamp)
	}

	configEvent := event.ConfigEvent{
		EventType:  "config_extracted",
		ConfigType: "agent",
		Path:       "/config/agents/example.toml",
		ChangeType: "extracted",
		Message:    "ok",
		OccurredAt: timestamp,
	}
	config.Bus().Publish(configEvent)

	data = readSSEDataFrame(t, reader, time.Second)
	var configPayload configEventPayload
	if err := json.Unmarshal(data, &configPayload); err != nil {
		t.Fatalf("decode config payload: %v", err)
	}
	if configPayload.Type != configEvent.EventType {
		t.Fatalf("expected event type %q, got %q", configEvent.EventType, configPayload.Type)
	}
	if configPayload.ConfigType != configEvent.ConfigType {
		t.Fatalf("expected config type %q, got %q", configEvent.ConfigType, configPayload.ConfigType)
	}
	if configPayload.Path != configEvent.Path {
		t.Fatalf("expected path %q, got %q", configEvent.Path, configPayload.Path)
	}
	if configPayload.ChangeType != configEvent.ChangeType {
		t.Fatalf("expected change type %q, got %q", configEvent.ChangeType, configPayload.ChangeType)
	}
	if configPayload.Message != configEvent.Message {
		t.Fatalf("expected message %q, got %q", configEvent.Message, configPayload.Message)
	}
	if !configPayload.Timestamp.Equal(configEvent.OccurredAt) {
		t.Fatalf("expected timestamp %v, got %v", configEvent.OccurredAt, configPayload.Timestamp)
	}

	workflowEvent := event.WorkflowEvent{
		EventType:  "workflow_started",
		WorkflowID: "workflow-123",
		SessionID:  "session-123",
		OccurredAt: time.Now().UTC(),
	}
	manager.WorkflowBus().Publish(workflowEvent)

	data = readSSEDataFrame(t, reader, time.Second)
	var workflowPayload workflowEventPayload
	if err := json.Unmarshal(data, &workflowPayload); err != nil {
		t.Fatalf("decode workflow payload: %v", err)
	}
	if workflowPayload.Type != workflowEvent.EventType {
		t.Fatalf("expected event type %q, got %q", workflowEvent.EventType, workflowPayload.Type)
	}
	if workflowPayload.WorkflowID != workflowEvent.WorkflowID {
		t.Fatalf("expected workflow ID %q, got %q", workflowEvent.WorkflowID, workflowPayload.WorkflowID)
	}
	if workflowPayload.SessionID != workflowEvent.SessionID {
		t.Fatalf("expected session ID %q, got %q", workflowEvent.SessionID, workflowPayload.SessionID)
	}
	if workflowPayload.Timestamp.IsZero() {
		t.Fatalf("expected timestamp to be set")
	}
}

func TestEventsSSEStreamFiltersTypes(t *testing.T) {
	bus := event.NewBus[watcher.Event](context.Background(), event.BusOptions{Name: "watcher_events"})
	defer bus.Close()

	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &fakeFactory{},
	})

	server := newSSEEventsServer(t, &EventsSSEHandler{Bus: bus, Manager: manager})
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/events/stream?types=config_extracted")
	if err != nil {
		t.Fatalf("get sse stream: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)

	bus.Publish(watcher.Event{
		Type:      watcher.EventTypeFileChanged,
		Path:      filepath.Join(".gestalt", "plans", "ignored.org"),
		Timestamp: time.Now().UTC(),
	})

	configEvent := event.ConfigEvent{
		EventType:  "config_extracted",
		ConfigType: "agent",
		Path:       "/config/agents/example.toml",
		ChangeType: "extracted",
		Message:    "ok",
		OccurredAt: time.Now().UTC(),
	}
	config.Bus().Publish(configEvent)

	data := readSSEDataFrame(t, reader, time.Second)
	var payload configEventPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Type != configEvent.EventType {
		t.Fatalf("expected type %q, got %q", configEvent.EventType, payload.Type)
	}
}

func readSSEDataFrame(t *testing.T, reader *bufio.Reader, timeout time.Duration) []byte {
	t.Helper()
	deadline := time.Now().Add(timeout)

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("timed out waiting for sse frame")
		}
		frame, err := readSSEFrameWithTimeout(reader, remaining)
		if err != nil {
			t.Fatalf("read sse frame: %v", err)
		}
		if len(frame.Data) == 0 {
			continue
		}
		return frame.Data
	}
}

func readSSEFrameWithTimeout(reader *bufio.Reader, timeout time.Duration) (sseFrame, error) {
	frameCh := make(chan sseFrame, 1)
	errCh := make(chan error, 1)

	go func() {
		frame, err := readSSEFrame(reader)
		if err != nil {
			errCh <- err
			return
		}
		frameCh <- frame
	}()

	select {
	case frame := <-frameCh:
		return frame, nil
	case err := <-errCh:
		return sseFrame{}, err
	case <-time.After(timeout):
		return sseFrame{}, errors.New("timeout")
	}
}

func newSSEEventsServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping sse test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}
	server.Start()
	return server
}
