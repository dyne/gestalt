package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeNotifyRequestMissingEventType(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/terminals/abc/notify", strings.NewReader(`{"terminal_id":"abc","agent_id":"agent","source":"manual"}`))
	_, err := decodeNotifyRequest(request)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", err.Status)
	}
	if err.Message != "missing event type" {
		t.Fatalf("expected missing event type, got %q", err.Message)
	}
}

func TestDecodeNotifyRequestInvalidJSON(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/terminals/abc/notify", strings.NewReader("{"))
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
	request := httptest.NewRequest(http.MethodPost, "/api/terminals/abc/notify", strings.NewReader(`{"terminal_id":"","agent_id":"agent","source":"manual","event_type":"plan-L1-wip"}`))
	_, err := decodeNotifyRequest(request)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", err.Status)
	}
	if err.Message != "missing terminal id" {
		t.Fatalf("expected missing terminal id, got %q", err.Message)
	}
}

func TestDecodeNotifyRequestValid(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/terminals/abc/notify", strings.NewReader(`{"terminal_id":"abc","agent_id":"agent","source":"manual","event_type":"plan-L1-wip","payload":{"plan_file":"plans/foo.org","heading":"WIP","state":"wip"}}`))
	payload, err := decodeNotifyRequest(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload.TerminalID != "abc" {
		t.Fatalf("expected terminal_id abc, got %q", payload.TerminalID)
	}
	if payload.AgentID != "agent" {
		t.Fatalf("expected agent_id agent, got %q", payload.AgentID)
	}
	if payload.EventType != "plan-L1-wip" {
		t.Fatalf("expected event_type plan-L1-wip, got %q", payload.EventType)
	}
	if len(payload.Payload) == 0 {
		t.Fatal("expected payload to be set")
	}
}
