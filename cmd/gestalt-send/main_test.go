package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"gestalt/internal/version"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (rt roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}

func withMockClient(t *testing.T, rt roundTripperFunc, fn func()) {
	t.Helper()
	previous := httpClient
	httpClient = &http.Client{Transport: rt}
	t.Cleanup(func() {
		httpClient = previous
	})
	fn()
}

func withAgentCacheDisabled(t *testing.T, fn func()) {
	t.Helper()
	previous := agentCacheTTL
	agentCacheTTL = 0
	t.Cleanup(func() {
		agentCacheTTL = previous
	})
	fn()
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = writer
	fn()
	_ = writer.Close()
	os.Stdout = previous
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	_ = reader.Close()
	return string(data)
}

func TestParseArgsMissingAgent(t *testing.T) {
	var stderr bytes.Buffer
	if _, err := parseArgs([]string{}, &stderr); err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(stderr.String(), "Usage: gestalt-send") {
		t.Fatalf("expected usage output, got %q", stderr.String())
	}
}

func TestParseArgsUsesEnvDefaults(t *testing.T) {
	t.Setenv("GESTALT_URL", "http://example.com")
	t.Setenv("GESTALT_TOKEN", "secret")
	var stderr bytes.Buffer

	cfg, err := parseArgs([]string{"codex"}, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.URL != "http://example.com" {
		t.Fatalf("expected URL to match env, got %q", cfg.URL)
	}
	if cfg.Token != "secret" {
		t.Fatalf("expected token to match env, got %q", cfg.Token)
	}
	if cfg.AgentRef != "codex" {
		t.Fatalf("expected agent ref codex, got %q", cfg.AgentRef)
	}
}

func TestRunWithSenderVersionFlag(t *testing.T) {
	previous := version.Version
	version.Version = "1.2.3"
	t.Cleanup(func() {
		version.Version = previous
	})

	output := captureStdout(t, func() {
		exitCode := runWithSender([]string{"--version"}, strings.NewReader(""), io.Discard, nil)
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", exitCode)
		}
	})
	if output != "gestalt-send version 1.2.3\n" {
		t.Fatalf("unexpected version output: %q", output)
	}
}

func TestRunWithSenderVersionFlagDev(t *testing.T) {
	previous := version.Version
	version.Version = "dev"
	t.Cleanup(func() {
		version.Version = previous
	})

	output := captureStdout(t, func() {
		exitCode := runWithSender([]string{"--version"}, strings.NewReader(""), io.Discard, nil)
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", exitCode)
		}
	})
	if output != "gestalt-send dev\n" {
		t.Fatalf("unexpected version output: %q", output)
	}
}

func TestResolveAgentCaseInsensitive(t *testing.T) {
	withAgentCacheDisabled(t, func() {
		withMockClient(t, func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/agents" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`[{"id":"codex","name":"Codex"}]`)),
				Header:     make(http.Header),
				Request:    r,
			}, nil
		}, func() {
			cfg := Config{
				URL:      "http://example.invalid",
				AgentRef: "CODEX",
			}
			if err := resolveAgent(&cfg); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.AgentID != "codex" {
				t.Fatalf("expected agent id codex, got %q", cfg.AgentID)
			}
			if cfg.AgentName != "Codex" {
				t.Fatalf("expected agent name Codex, got %q", cfg.AgentName)
			}
		})
	})
}

func TestResolveAgentAmbiguous(t *testing.T) {
	withAgentCacheDisabled(t, func() {
		withMockClient(t, func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`[
					{"id":"coder","name":"Architect"},
					{"id":"assistant","name":"Coder"}
				]`)),
				Header:  make(http.Header),
				Request: r,
			}, nil
		}, func() {
			cfg := Config{
				URL:      "http://example.invalid",
				AgentRef: "Coder",
			}
			err := resolveAgent(&cfg)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), "matches agent id") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	})
}

func TestSendAgentInputSuccess(t *testing.T) {
	withMockClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/agents/Codex/input" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("unexpected auth header: %q", r.Header.Get("Authorization"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "hello" {
			t.Fatalf("expected payload %q, got %q", "hello", string(body))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	}, func() {
		cfg := Config{
			URL:       "http://example.invalid",
			Token:     "token",
			AgentName: "Codex",
		}
		if err := sendAgentInput(cfg, []byte("hello")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunWithSenderReturnsAgentNotFound(t *testing.T) {
	withAgentCacheDisabled(t, func() {
		withMockClient(t, func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/api/agents":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`[{"id":"missing","name":"Missing"}]`)),
					Header:     make(http.Header),
					Request:    r,
				}, nil
			case "/api/agents/Missing/input":
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader(`{"error":"agent not running"}`)),
					Header:     make(http.Header),
					Request:    r,
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("")),
					Header:     make(http.Header),
					Request:    r,
				}, nil
			}
		}, func() {
			t.Setenv("GESTALT_URL", "http://example.invalid")
			var stderr bytes.Buffer

			code := runWithSender([]string{"missing"}, strings.NewReader("hi"), &stderr, sendAgentInput)
			if code != 2 {
				t.Fatalf("expected exit code 2, got %d", code)
			}
			if !strings.Contains(stderr.String(), "agent not running") {
				t.Fatalf("expected error message, got %q", stderr.String())
			}
		})
	})
}

func TestSendAgentInputAutoStart(t *testing.T) {
	inputCalls := 0
	withMockClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/agents/Codex/input":
			inputCalls++
			if inputCalls == 1 {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader(`{"error":"agent not running"}`)),
					Header:     make(http.Header),
					Request:    r,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
				Request:    r,
			}, nil
		case "/api/terminals":
			return &http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
				Request:    r,
			}, nil
		default:
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
				Request:    r,
			}, nil
		}
	}, func() {
		previousDelay := startRetryDelay
		startRetryDelay = 0
		t.Cleanup(func() {
			startRetryDelay = previousDelay
		})

		cfg := Config{
			URL:       "http://example.invalid",
			AgentID:   "codex",
			AgentName: "Codex",
			Start:     true,
		}
		if err := sendAgentInput(cfg, []byte("hello")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunWithSenderAmbiguousAgent(t *testing.T) {
	withAgentCacheDisabled(t, func() {
		withMockClient(t, func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`[
					{"id":"coder","name":"Architect"},
					{"id":"assistant","name":"Coder"}
				]`)),
				Header:  make(http.Header),
				Request: r,
			}, nil
		}, func() {
			var stderr bytes.Buffer
			code := runWithSender([]string{"Coder"}, strings.NewReader("hi"), &stderr, nil)
			if code != 2 {
				t.Fatalf("expected exit code 2, got %d", code)
			}
			if !strings.Contains(stderr.String(), "matches agent id") {
				t.Fatalf("unexpected error message: %q", stderr.String())
			}
		})
	})
}

func TestRunWithSenderAgentFetchFailure(t *testing.T) {
	withAgentCacheDisabled(t, func() {
		withMockClient(t, func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("boom")
		}, func() {
			var stderr bytes.Buffer
			code := runWithSender([]string{"Coder"}, strings.NewReader("hi"), &stderr, nil)
			if code != 3 {
				t.Fatalf("expected exit code 3, got %d", code)
			}
			if !strings.Contains(stderr.String(), "failed to fetch agents") {
				t.Fatalf("unexpected error message: %q", stderr.String())
			}
		})
	})
}

func TestHandleSendErrorMapping(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantCode    int
		wantMessage string
	}{
		{
			name:        "agent missing",
			err:         sendErr(2, "agent not running"),
			wantCode:    2,
			wantMessage: "agent not running",
		},
		{
			name:        "server error",
			err:         sendErr(3, "server error"),
			wantCode:    3,
			wantMessage: "server error",
		},
		{
			name:        "generic error",
			err:         errors.New("boom"),
			wantCode:    3,
			wantMessage: "boom",
		},
	}

	for _, testCase := range cases {
		var stderr bytes.Buffer
		code := handleSendError(testCase.err, &stderr)
		if code != testCase.wantCode {
			t.Fatalf("%s: expected code %d, got %d", testCase.name, testCase.wantCode, code)
		}
		if !strings.Contains(stderr.String(), testCase.wantMessage) {
			t.Fatalf("%s: expected message %q, got %q", testCase.name, testCase.wantMessage, stderr.String())
		}
	}
}
