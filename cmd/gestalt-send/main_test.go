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

func TestSendInputSessionIDSuccess(t *testing.T) {
	withMockClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/sessions/s-1/input" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
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
			SessionID: "s-1",
		}
		if err := sendInput(cfg, []byte("hello")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunWithSenderSessionID(t *testing.T) {
	sawInput := false
	withMockClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/sessions/s-1/input":
			sawInput = true
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
				Request:    r,
			}, nil
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		return nil, nil
	}, func() {
		var stderr bytes.Buffer
		code := runWithSender([]string{"--session-id", "s-1"}, strings.NewReader("hi"), &stderr, sendInput)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d: %s", code, stderr.String())
		}
		if !sawInput {
			t.Fatalf("expected input call")
		}
	})
}

func TestHandleSendErrorMapping(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantCode    int
		wantMessage string
	}{
		{name: "session missing", err: sendErr(2, "session not found"), wantCode: 2, wantMessage: "session not found"},
		{name: "server error", err: sendErr(3, "server error"), wantCode: 3, wantMessage: "server error"},
		{name: "generic error", err: errors.New("boom"), wantCode: 3, wantMessage: "boom"},
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

func TestRunWithSenderNonZeroWritesStderr(t *testing.T) {
	t.Run("usage error", func(t *testing.T) {
		var stderr bytes.Buffer
		code := runWithSender([]string{}, strings.NewReader(""), &stderr, nil)
		if code != 1 {
			t.Fatalf("expected exit code 1, got %d", code)
		}
		if strings.TrimSpace(stderr.String()) == "" {
			t.Fatalf("expected stderr output")
		}
	})

	t.Run("network error", func(t *testing.T) {
		withMockClient(t, func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("network down")
		}, func() {
			var stderr bytes.Buffer
			code := runWithSender([]string{"--session-id", "s-1"}, strings.NewReader("hi"), &stderr, sendInput)
			if code != 3 {
				t.Fatalf("expected exit code 3, got %d", code)
			}
			if strings.TrimSpace(stderr.String()) == "" {
				t.Fatalf("expected stderr output")
			}
		})
	})
}
