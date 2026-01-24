package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gestalt/internal/event"

	"github.com/gorilla/websocket"
)

func TestServeWSBusStreamDeliversPayload(t *testing.T) {
	bus := event.NewBus[string](context.Background(), event.BusOptions{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveWSBusStream(w, r, wsBusStreamConfig[string]{
			Bus: bus,
			BuildPayload: func(value string) (any, bool) {
				return map[string]string{"value": value}, true
			},
		})
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	go func() {
		time.Sleep(10 * time.Millisecond)
		bus.Publish("hello")
	}()

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var payload map[string]string
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if payload["value"] != "hello" {
		t.Fatalf("unexpected payload: %v", payload)
	}
}

func TestServeWSBusStreamUnavailableCloses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveWSBusStream(w, r, wsBusStreamConfig[string]{
			Bus:               nil,
			UnavailableReason: "stream unavailable",
			BuildPayload: func(value string) (any, bool) {
				return map[string]string{"value": value}, true
			},
		})
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

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
	if closeErr.Text != "stream unavailable" {
		t.Fatalf("expected reason %q, got %q", "stream unavailable", closeErr.Text)
	}
}
