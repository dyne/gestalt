package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeNotifyRequestMissingPayload(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/sessions/abc/notify", strings.NewReader(`{"session_id":"abc"}`))
	_, err := decodeNotifyRequest(request)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", err.Status)
	}
	if err.Message != "missing payload" {
		t.Fatalf("expected missing payload, got %q", err.Message)
	}
}

func TestDecodeNotifyRequestInvalidJSON(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/sessions/abc/notify", strings.NewReader("{"))
	_, err := decodeNotifyRequest(request)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", err.Status)
	}
	if err.Message != "invalid request body" {
		t.Fatalf("expected invalid request body, got %q", err.Message)
	}
}

func TestDecodeNotifyRequestMissingTerminalID(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/sessions/abc/notify", strings.NewReader(`{"session_id":"","payload":{"type":"plan-L1-wip"}}`))
	_, err := decodeNotifyRequest(request)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", err.Status)
	}
	if err.Message != "missing session id" {
		t.Fatalf("expected missing session id, got %q", err.Message)
	}
}

func TestDecodeNotifyRequestValid(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/sessions/abc/notify", strings.NewReader(`{"session_id":"abc","payload":{"type":"plan-L1-wip","plan_file":"plans/foo.org","heading":"WIP","state":"wip"}}`))
	payload, err := decodeNotifyRequest(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload.SessionID != "abc" {
		t.Fatalf("expected session_id abc, got %q", payload.SessionID)
	}
	if payload.EventType != "plan-L1-wip" {
		t.Fatalf("expected event_type plan-L1-wip, got %q", payload.EventType)
	}
	if len(payload.Payload) == 0 {
		t.Fatal("expected payload to be set")
	}
}

func TestDecodeNotifyRequestRejectsAgentID(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/sessions/abc/notify", strings.NewReader(`{"session_id":"abc","agent_id":"agent"}`))
	_, err := decodeNotifyRequest(request)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", err.Status)
	}
	if err.Message != "invalid request body" {
		t.Fatalf("expected invalid request body, got %q", err.Message)
	}
}

func TestDecodeNotifyRequestRejectsAgentName(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/sessions/abc/notify", strings.NewReader(`{"session_id":"abc","agent_name":"Coder 1"}`))
	_, err := decodeNotifyRequest(request)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", err.Status)
	}
	if err.Message != "invalid request body" {
		t.Fatalf("expected invalid request body, got %q", err.Message)
	}
}

func TestDecodeNotifyRequestRejectsSource(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/sessions/abc/notify", strings.NewReader(`{"session_id":"abc","source":"manual"}`))
	_, err := decodeNotifyRequest(request)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", err.Status)
	}
	if err.Message != "invalid request body" {
		t.Fatalf("expected invalid request body, got %q", err.Message)
	}
}

func TestDecodeNotifyRequestMissingPayloadType(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/sessions/abc/notify", strings.NewReader(`{"session_id":"abc","payload":{"plan_file":"plan.org"}}`))
	_, err := decodeNotifyRequest(request)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", err.Status)
	}
	if err.Message != "missing payload type" {
		t.Fatalf("expected missing payload type, got %q", err.Message)
	}
}

func TestDecodeNotifyRequestRejectsEventTypeField(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/sessions/abc/notify", strings.NewReader(`{"session_id":"abc","event_type":"plan-L1-wip","payload":{"type":"plan-L1-wip"}}`))
	_, err := decodeNotifyRequest(request)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", err.Status)
	}
	if err.Message != "invalid request body" {
		t.Fatalf("expected invalid request body, got %q", err.Message)
	}
}
