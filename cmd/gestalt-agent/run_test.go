package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gestalt/internal/runner/launchspec"
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
	code, err := runAgent(Config{AgentID: "coder", URL: "http://127.0.0.1:1"}, bytes.NewReader(nil), bytes.NewBuffer(nil), nil)
	if code != exitServer {
		t.Fatalf("expected exit code %d, got %d", exitServer, code)
	}
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunAgentMissingLaunch(t *testing.T) {
	server := newTestSessionServer(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"session-1"}`))
	})
	defer server.Close()

	code, err := runAgent(Config{AgentID: "coder", URL: server.URL}, bytes.NewReader(nil), bytes.NewBuffer(nil), nil)
	if code != exitServer {
		t.Fatalf("expected exit code %d, got %d", exitServer, code)
	}
	if err == nil || !strings.Contains(err.Error(), "launch spec") {
		t.Fatalf("expected launch error, got %v", err)
	}
}

func TestRunAgentRunsCodexArgs(t *testing.T) {
	server := newTestSessionServer(t, func(w http.ResponseWriter) {
		response := createSessionResponse{
			ID: "session-1",
			Launch: &launchspec.LaunchSpec{
				Argv: []string{"codex", "-c", "model=o3"},
			},
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	var gotArgs []string
	exec := func(args []string) (int, error) {
		gotArgs = append([]string(nil), args...)
		return 0, nil
	}
	code, err := runAgent(Config{AgentID: "coder", URL: server.URL}, bytes.NewReader(nil), bytes.NewBuffer(nil), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "-c" || gotArgs[1] != "model=o3" {
		t.Fatalf("unexpected exec args: %#v", gotArgs)
	}
}

func TestRunDryRunPrintsCommand(t *testing.T) {
	server := newTestSessionServer(t, func(w http.ResponseWriter) {
		response := createSessionResponse{
			ID: "session-1",
			Launch: &launchspec.LaunchSpec{
				Argv: []string{"codex", "-c", "model=o3"},
			},
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithExec([]string{"--dryrun", "--url", server.URL, "coder"}, &stdout, &stderr, func(args []string) (int, error) {
		return 0, errors.New("should not execute")
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "codex") {
		t.Fatalf("expected command output, got %q", output)
	}
	if !strings.Contains(output, "model=o3") {
		t.Fatalf("expected model in output, got %q", output)
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

func TestPrintTmuxAttachHintOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	var out bytes.Buffer
	printTmuxAttachHint(&out, "Fixer 1")
	text := out.String()
	if !strings.Contains(text, "Session is running in tmux.") {
		t.Fatalf("missing session message: %q", text)
	}
	if !strings.Contains(text, `Attach with: tmux attach -t "Gestalt `+filepath.Base(mustGetwd(t))+`"`) {
		t.Fatalf("expected attach target for workdir session, got %q", text)
	}
	if !strings.Contains(text, `Then switch with: tmux select-window -t "Fixer 1"`) {
		t.Fatalf("expected window switch hint, got %q", text)
	}
}

func TestPrintTmuxAttachHintInsideTmux(t *testing.T) {
	t.Setenv("TMUX", "1")
	var out bytes.Buffer
	printTmuxAttachHint(&out, "Fixer 1")
	text := out.String()
	if !strings.Contains(text, `Switch with: tmux select-window -t "Fixer 1"`) {
		t.Fatalf("expected select-window hint, got %q", text)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}
