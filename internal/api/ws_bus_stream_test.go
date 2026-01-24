package api

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gestalt/internal/event"

	"github.com/gorilla/websocket"
)

func TestServeWSBusStreamDeliversPayload(t *testing.T) {
	if !canListenLocal(t) {
		t.Skip("local listener unavailable for websocket test")
	}
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
	if !canListenLocal(t) {
		t.Skip("local listener unavailable for websocket test")
	}
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
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatalf("expected handshake error")
	}
	if resp == nil {
		t.Fatalf("expected http response")
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "stream unavailable") {
		t.Fatalf("expected error body to mention stream unavailable, got %q", string(body))
	}
}

func canListenLocal(t *testing.T) bool {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}
