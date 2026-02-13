package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchAgentsFiltersResults(t *testing.T) {
	requireLocalListener(t)
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/agents" {
			t.Fatalf("expected path /api/agents, got %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"id":"codex","name":"Codex"},{"id":"","name":"skip"}]`)
	}))
	t.Cleanup(server.Close)

	agents, err := FetchAgents(server.Client(), server.URL, "token")
	if err != nil {
		t.Fatalf("fetch agents: %v", err)
	}
	if gotAuth != "Bearer token" {
		t.Fatalf("expected auth header, got %q", gotAuth)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].ID != "codex" || agents[0].Name != "Codex" {
		t.Fatalf("unexpected agent: %+v", agents[0])
	}
}

func TestFetchAgentsHTTPError(t *testing.T) {
	requireLocalListener(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"boom"}`)
	}))
	t.Cleanup(server.Close)

	_, err := FetchAgents(server.Client(), server.URL, "")
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %v", err)
	}
	if httpErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", httpErr.StatusCode)
	}
	if httpErr.Message != "boom" {
		t.Fatalf("expected message boom, got %q", httpErr.Message)
	}
}

func TestSendSessionInputHTTPError(t *testing.T) {
	requireLocalListener(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/sessions/session-1/input" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "missing")
	}))
	t.Cleanup(server.Close)

	err := SendSessionInput(server.Client(), server.URL, "", "session-1", []byte("hi"))
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %v", err)
	}
	if httpErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", httpErr.StatusCode)
	}
	if httpErr.Message != "missing" {
		t.Fatalf("expected message missing, got %q", httpErr.Message)
	}
}

func TestStartAgentSuccess(t *testing.T) {
	requireLocalListener(t)
	var gotPayload map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/sessions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(server.Close)

	if err := StartAgent(server.Client(), server.URL, "", "agent-1"); err != nil {
		t.Fatalf("start agent: %v", err)
	}
	if gotPayload["agent"] != "agent-1" {
		t.Fatalf("expected agent payload, got %+v", gotPayload)
	}
}

func TestSendSessionInputAddsToken(t *testing.T) {
	requireLocalListener(t)
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	err := SendSessionInput(server.Client(), server.URL, "token", "session-1", bytes.NewBufferString("hi").Bytes())
	if err != nil {
		t.Fatalf("send input: %v", err)
	}
	if gotAuth != "Bearer token" {
		t.Fatalf("expected auth header, got %q", gotAuth)
	}
}

func requireLocalListener(t *testing.T) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("local listener unavailable for httptest")
	}
	_ = listener.Close()
}
