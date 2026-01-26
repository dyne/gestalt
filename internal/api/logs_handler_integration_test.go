package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"gestalt/internal/otel"

	"github.com/gorilla/websocket"
)

func TestLogsWebSocketStream(t *testing.T) {
	hub := otel.NewLogHub(time.Hour)
	previous := otel.ActiveLogHub()
	otel.SetActiveLogHub(hub)
	t.Cleanup(func() { otel.SetActiveLogHub(previous) })

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &LogsHandler{}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/logs"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	hub.Append(buildOTLPRecord("hello", "INFO", map[string]string{"component": "test"}))

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var entry map[string]any
	if err := conn.ReadJSON(&entry); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if logBody(entry) != "hello" {
		t.Fatalf("expected message hello, got %q", logBody(entry))
	}
	if text, _ := entry["severityText"].(string); text != "INFO" {
		t.Fatalf("expected severity INFO, got %v", entry["severityText"])
	}
	if attrValue(entry, "component") != "test" {
		t.Fatalf("expected attribute component=test, got %q", attrValue(entry, "component"))
	}
}

func TestLogsWebSocketSnapshot(t *testing.T) {
	hub := otel.NewLogHub(time.Hour)
	previous := otel.ActiveLogHub()
	otel.SetActiveLogHub(hub)
	t.Cleanup(func() { otel.SetActiveLogHub(previous) })
	hub.Append(buildOTLPRecord("snapshot entry", "WARN", map[string]string{"source": "preconnect"}))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &LogsHandler{}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/logs"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var entry map[string]any
	if err := conn.ReadJSON(&entry); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if logBody(entry) != "snapshot entry" {
		t.Fatalf("expected snapshot entry, got %q", logBody(entry))
	}
	if text, _ := entry["severityText"].(string); text != "WARN" {
		t.Fatalf("expected severity WARN, got %v", entry["severityText"])
	}
	if attrValue(entry, "source") != "preconnect" {
		t.Fatalf("expected attribute source=preconnect, got %q", attrValue(entry, "source"))
	}
	if attrValue(entry, "gestalt.replay_window") != "1h" {
		t.Fatalf("expected replay_window attribute, got %q", attrValue(entry, "gestalt.replay_window"))
	}
}

func TestLogsWebSocketAuth(t *testing.T) {
	hub := otel.NewLogHub(time.Hour)
	previous := otel.ActiveLogHub()
	otel.SetActiveLogHub(hub)
	t.Cleanup(func() { otel.SetActiveLogHub(previous) })

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config: &http.Server{Handler: &LogsHandler{
			AuthToken: "secret",
		}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/logs"
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

func TestLogsWebSocketQueryFilter(t *testing.T) {
	hub := otel.NewLogHub(time.Hour)
	previous := otel.ActiveLogHub()
	otel.SetActiveLogHub(hub)
	t.Cleanup(func() { otel.SetActiveLogHub(previous) })

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &LogsHandler{}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/logs?level=warning"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	hub.Append(buildOTLPRecord("info msg", "INFO", nil))
	hub.Append(buildOTLPRecord("warn msg", "WARN", nil))

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var entry map[string]any
	if err := conn.ReadJSON(&entry); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if text, _ := entry["severityText"].(string); text != "WARN" {
		t.Fatalf("expected WARN, got %v", entry["severityText"])
	}
	if logBody(entry) != "warn msg" {
		t.Fatalf("expected warn msg, got %q", logBody(entry))
	}
}

func TestLogsWebSocketMessageFilter(t *testing.T) {
	hub := otel.NewLogHub(time.Hour)
	previous := otel.ActiveLogHub()
	otel.SetActiveLogHub(hub)
	t.Cleanup(func() { otel.SetActiveLogHub(previous) })

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &LogsHandler{}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/logs"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"level":"error"}`)); err != nil {
		t.Fatalf("write filter message: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	hub.Append(buildOTLPRecord("warn msg", "WARN", nil))
	hub.Append(buildOTLPRecord("error msg", "ERROR", nil))

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var entry map[string]any
	if err := conn.ReadJSON(&entry); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if text, _ := entry["severityText"].(string); text != "ERROR" {
		t.Fatalf("expected ERROR, got %v", entry["severityText"])
	}
	if logBody(entry) != "error msg" {
		t.Fatalf("expected error msg, got %q", logBody(entry))
	}
}

func TestLogsWebSocketCloseEndsHandler(t *testing.T) {
	hub := otel.NewLogHub(time.Hour)
	previous := otel.ActiveLogHub()
	otel.SetActiveLogHub(hub)
	t.Cleanup(func() { otel.SetActiveLogHub(previous) })
	handlerDone := make(chan struct{})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			(&LogsHandler{}).ServeHTTP(w, r)
			close(handlerDone)
		})},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/logs"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}

	_ = conn.Close()

	select {
	case <-handlerDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("handler did not exit after close")
	}
}

func buildOTLPRecord(message, severity string, attrs map[string]string) map[string]any {
	record := map[string]any{
		"timeUnixNano": strconv.FormatInt(time.Now().UnixNano(), 10),
		"severityText": severity,
		"body":         map[string]any{"stringValue": message},
	}
	if len(attrs) == 0 {
		return record
	}
	attributes := make([]any, 0, len(attrs))
	for key, value := range attrs {
		attributes = append(attributes, map[string]any{
			"key": key,
			"value": map[string]any{
				"stringValue": value,
			},
		})
	}
	record["attributes"] = attributes
	return record
}

func attrValue(record map[string]any, key string) string {
	attributes := asSlice(record["attributes"])
	for _, entry := range attributes {
		entryMap := asMap(entry)
		if entryMap == nil {
			continue
		}
		entryKey, _ := extractString(entryMap, "key")
		if entryKey != key {
			continue
		}
		valueMap := asMap(entryMap["value"])
		if valueMap == nil {
			return ""
		}
		if value, ok := valueMap["stringValue"].(string); ok {
			return value
		}
	}
	return ""
}
