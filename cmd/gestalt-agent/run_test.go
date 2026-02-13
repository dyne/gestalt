package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestRunUsageExitCode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithExec([]string{}, &stdout, &stderr, nil)
	if code != exitUsage {
		t.Fatalf("expected exit code %d, got %d", exitUsage, code)
	}
	if !strings.Contains(stderr.String(), "agent id required") {
		t.Fatalf("expected error message, got %q", stderr.String())
	}
}

func TestRunAgentServerUnreachable(t *testing.T) {
	code, err := runAgent(Config{AgentID: "coder", Host: "127.0.0.1", Port: 1}, bytes.NewReader(nil), bytes.NewBuffer(nil), nil)
	if code != exitServer {
		t.Fatalf("expected exit code %d, got %d", exitServer, code)
	}
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunAgentAttachesToTmux(t *testing.T) {
	server := newTestSessionServer(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"session-1"}`))
	})
	defer server.Close()

	var gotArgs []string
	exec := func(args []string) (int, error) {
		gotArgs = append([]string(nil), args...)
		return 0, nil
	}
	host, port := testHostPort(t, server.URL)
	code, err := runAgent(Config{AgentID: "coder", Host: host, Port: port}, bytes.NewReader(nil), bytes.NewBuffer(nil), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if len(gotArgs) == 0 {
		t.Fatalf("expected tmux args")
	}
	if gotArgs[0] != "attach" && gotArgs[0] != "switch-client" {
		t.Fatalf("unexpected tmux command: %#v", gotArgs)
	}
}

func TestRunAgentExecError(t *testing.T) {
	server := newTestSessionServer(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createSessionResponse{ID: "session-1"})
	})
	defer server.Close()

	exec := func(args []string) (int, error) {
		return 7, errors.New("tmux failed")
	}
	host, port := testHostPort(t, server.URL)
	code, err := runAgent(Config{AgentID: "coder", Host: host, Port: port}, bytes.NewReader(nil), bytes.NewBuffer(nil), exec)
	if err == nil || !strings.Contains(err.Error(), "tmux failed") {
		t.Fatalf("expected tmux error, got %v", err)
	}
	if code != 7 {
		t.Fatalf("expected exit code 7, got %d", code)
	}
}

func TestRunDryRunPrintsCommand(t *testing.T) {
	server := newTestSessionServer(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createSessionResponse{ID: "session-1"})
	})
	defer server.Close()

	host, port := testHostPort(t, server.URL)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithExec([]string{"--dryrun", "--host", host, "--port", strconv.Itoa(port), "coder"}, &stdout, &stderr, func(args []string) (int, error) {
		return 0, errors.New("should not execute")
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "tmux") {
		t.Fatalf("expected command output, got %q", output)
	}
	if !strings.Contains(output, "attach") && !strings.Contains(output, "switch-client") {
		t.Fatalf("expected tmux attach/switch command, got %q", output)
	}
}

func newTestSessionServer(t *testing.T, handler func(w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/sessions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		handler(w)
	}))
}

func testHostPort(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	host := parsed.Hostname()
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return host, port
}
