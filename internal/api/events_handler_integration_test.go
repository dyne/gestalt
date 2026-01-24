package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/watcher"

	"github.com/gorilla/websocket"
)

type eventMessage struct {
	Type      string    `json:"type"`
	Path      string    `json:"path"`
	Timestamp time.Time `json:"timestamp"`
}

func TestEventsWebSocketStream(t *testing.T) {
	bus := event.NewBus[watcher.Event](context.Background(), event.BusOptions{Name: "watcher_events"})
	defer bus.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &EventsHandler{Bus: bus}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/events"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	time.Sleep(10 * time.Millisecond)
	bus.Publish(watcher.Event{
		Type:      watcher.EventTypeFileChanged,
		Path:      filepath.Join(".gestalt", "PLAN.org"),
		Timestamp: time.Now().UTC(),
	})

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var payload eventMessage
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if payload.Type != watcher.EventTypeFileChanged {
		t.Fatalf("expected type %q, got %q", watcher.EventTypeFileChanged, payload.Type)
	}
	if payload.Path != filepath.Join(".gestalt", "PLAN.org") {
		t.Fatalf("expected path .gestalt/PLAN.org, got %q", payload.Path)
	}
	if payload.Timestamp.IsZero() {
		t.Fatalf("expected timestamp to be set")
	}
}

func TestEventsWebSocketAuth(t *testing.T) {
	bus := event.NewBus[watcher.Event](context.Background(), event.BusOptions{Name: "watcher_events"})
	defer bus.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config: &http.Server{Handler: &EventsHandler{
			Bus:       bus,
			AuthToken: "secret",
		}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/events"
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

func TestEventsWebSocketBusUnavailable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &EventsHandler{}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/events"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var payload struct {
		Type      string `json:"type"`
		Message   string `json:"message"`
		Status    int    `json:"status"`
		CloseCode int    `json:"close_code"`
	}
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatalf("read error payload: %v", err)
	}
	if payload.Type != "error" {
		t.Fatalf("expected type error, got %q", payload.Type)
	}
	if payload.Message != "event bus unavailable" {
		t.Fatalf("expected message %q, got %q", "event bus unavailable", payload.Message)
	}
	if payload.Status != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, payload.Status)
	}

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Fatalf("expected close error")
	}
	closeErr, ok := err.(*websocket.CloseError)
	if !ok {
		t.Fatalf("expected close error, got %T", err)
	}
	if closeErr.Code != websocket.CloseInternalServerErr {
		t.Fatalf("expected close code %d, got %d", websocket.CloseInternalServerErr, closeErr.Code)
	}
}
