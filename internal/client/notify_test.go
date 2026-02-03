package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostNotifyEventEscapesSessionID(t *testing.T) {
	requireLocalListener(t)
	var gotURI string
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.RequestURI
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	payload := NotifyRequest{
		Payload: json.RawMessage(`{"type":"plan-L1-wip","plan_file":"plan.org"}`),
	}
	if err := PostNotifyEvent(server.Client(), server.URL, "", "Coder 1", payload); err != nil {
		t.Fatalf("post notify: %v", err)
	}
	if gotURI != "/api/sessions/Coder%201/notify" {
		t.Fatalf("expected escaped path, got %q", gotURI)
	}
	if !strings.Contains(gotBody, "\"session_id\":\"Coder 1\"") {
		t.Fatalf("expected session id in body, got %q", gotBody)
	}
}
