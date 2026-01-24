package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestServeWSStreamSendsPayloadAndCloses(t *testing.T) {
	if !canListenLocalWS(t) {
		t.Skip("local listener unavailable for websocket test")
	}
	output := make(chan string, 1)
	handlerDone := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveWSStream(w, r, wsStreamConfig[string]{
			Output: output,
			BuildPayload: func(value string) (any, bool) {
				return map[string]string{"value": value}, true
			},
		})
		close(handlerDone)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}

	output <- "hello"

	var payload map[string]string
	if err := conn.ReadJSON(&payload); err != nil {
		_ = conn.Close()
		t.Fatalf("read websocket: %v", err)
	}
	if payload["value"] != "hello" {
		_ = conn.Close()
		t.Fatalf("unexpected payload: %v", payload)
	}

	_ = conn.Close()

	select {
	case <-handlerDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("handler did not exit after close")
	}
}

func TestStartWSWriteLoopSendsPayloadAndStops(t *testing.T) {
	if !canListenLocalWS(t) {
		t.Skip("local listener unavailable for websocket test")
	}
	output := make(chan string, 1)
	handlerDone := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loop, err := startWSWriteLoop(w, r, wsStreamConfig[string]{
			Output: output,
			BuildPayload: func(value string) (any, bool) {
				return map[string]string{"value": value}, true
			},
		})
		if err != nil {
			t.Errorf("start ws loop: %v", err)
			close(handlerDone)
			return
		}
		defer loop.Stop()

		conn := loop.Conn
		defer conn.Close()

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				close(handlerDone)
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}

	output <- "hello"

	var payload map[string]string
	if err := conn.ReadJSON(&payload); err != nil {
		_ = conn.Close()
		t.Fatalf("read websocket: %v", err)
	}
	if payload["value"] != "hello" {
		_ = conn.Close()
		t.Fatalf("unexpected payload: %v", payload)
	}

	_ = conn.Close()

	select {
	case <-handlerDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("handler did not exit after close")
	}
}

func TestStartWSWriteLoopRequiresOutput(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	recorder := httptest.NewRecorder()

	if _, err := startWSWriteLoop(recorder, req, wsStreamConfig[string]{}); err == nil {
		t.Fatalf("expected error for nil output")
	}
}

func TestRequireWSTokenUnauthorized(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	recorder := httptest.NewRecorder()

	if requireWSToken(recorder, req, "secret", nil) {
		t.Fatalf("expected unauthorized result")
	}

	resp := recorder.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, resp.StatusCode)
	}
}

func TestWriteBinaryPayloadTypeMismatch(t *testing.T) {
	if err := writeBinaryPayload(nil, "not-bytes"); err == nil {
		t.Fatalf("expected type mismatch error")
	}
}

func TestWriteWSErrorClosesWithCodeAndReason(t *testing.T) {
	if !canListenLocalWS(t) {
		t.Skip("local listener unavailable for websocket test")
	}
	handlerDone := make(chan struct{})
	handlerErr := make(chan error, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgradeWebSocket(w, r, nil)
		if err != nil {
			handlerErr <- err
			close(handlerDone)
			return
		}

		writeWSError(w, r, conn, nil, wsError{
			Status:  http.StatusBadRequest,
			Message: "bad request",
		})
		handlerErr <- nil
		close(handlerDone)
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
	if closeErr.Code != websocket.CloseProtocolError {
		t.Fatalf("expected close code %d, got %d", websocket.CloseProtocolError, closeErr.Code)
	}
	if closeErr.Text != "bad request" {
		t.Fatalf("expected reason %q, got %q", "bad request", closeErr.Text)
	}

	if err := <-handlerErr; err != nil {
		t.Fatalf("handler error: %v", err)
	}

	select {
	case <-handlerDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("handler did not exit after close")
	}
}

func TestWriteWSErrorSendsEnvelope(t *testing.T) {
	if !canListenLocalWS(t) {
		t.Skip("local listener unavailable for websocket test")
	}
	handlerDone := make(chan struct{})
	handlerErr := make(chan error, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgradeWebSocket(w, r, nil)
		if err != nil {
			handlerErr <- err
			close(handlerDone)
			return
		}

		writeWSError(w, r, conn, nil, wsError{
			Status:       http.StatusBadRequest,
			Message:      "bad request",
			SendEnvelope: true,
		})
		handlerErr <- nil
		close(handlerDone)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
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
		t.Fatalf("read envelope: %v", err)
	}
	if payload.Type != "error" {
		t.Fatalf("expected type error, got %q", payload.Type)
	}
	if payload.Message != "bad request" {
		t.Fatalf("expected message %q, got %q", "bad request", payload.Message)
	}
	if payload.Status != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, payload.Status)
	}
	if payload.CloseCode != websocket.CloseProtocolError {
		t.Fatalf("expected close code %d, got %d", websocket.CloseProtocolError, payload.CloseCode)
	}

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Fatalf("expected close error")
	}

	if err := <-handlerErr; err != nil {
		t.Fatalf("handler error: %v", err)
	}

	select {
	case <-handlerDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("handler did not exit after close")
	}
}

func canListenLocalWS(t *testing.T) bool {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}
