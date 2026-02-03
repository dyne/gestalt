package main

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
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

func TestSendNotifyEventClientError(t *testing.T) {
	withMockClient(t, func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(`{"error":"bad"}`)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	}, func() {
		cfg := Config{
			URL:        "http://example.invalid",
			SessionID:  "term-1",
			EventType:  "plan-L1-wip",
		}
		err := sendNotifyEvent(cfg)
		var notifyErr *notifyError
		if !errors.As(err, &notifyErr) {
			t.Fatalf("expected notify error, got %v", err)
		}
		if notifyErr.Code != 2 {
			t.Fatalf("expected code 2, got %d", notifyErr.Code)
		}
	})
}

func TestSendNotifyEventServerError(t *testing.T) {
	withMockClient(t, func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("oops")),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	}, func() {
		cfg := Config{
			URL:        "http://example.invalid",
			SessionID:  "term-1",
			EventType:  "plan-L1-wip",
		}
		err := sendNotifyEvent(cfg)
		var notifyErr *notifyError
		if !errors.As(err, &notifyErr) {
			t.Fatalf("expected notify error, got %v", err)
		}
		if notifyErr.Code != 3 {
			t.Fatalf("expected code 3, got %d", notifyErr.Code)
		}
	})
}

func TestSendNotifyEventNetworkError(t *testing.T) {
	withMockClient(t, func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	}, func() {
		cfg := Config{
			URL:        "http://example.invalid",
			SessionID:  "term-1",
			EventType:  "plan-L1-wip",
		}
		err := sendNotifyEvent(cfg)
		var notifyErr *notifyError
		if !errors.As(err, &notifyErr) {
			t.Fatalf("expected notify error, got %v", err)
		}
		if notifyErr.Code != 3 {
			t.Fatalf("expected code 3, got %d", notifyErr.Code)
		}
	})
}

func TestSendNotifyEventEscapesSessionID(t *testing.T) {
	withMockClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.EscapedPath() != "/api/sessions/Coder%201/notify" {
			t.Fatalf("expected escaped path, got %q", r.URL.EscapedPath())
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "\"agent_id\"") || strings.Contains(string(body), "\"agent_name\"") || strings.Contains(string(body), "\"source\"") {
			t.Fatalf("expected notify body without agent_id/agent_name/source, got %q", string(body))
		}
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	}, func() {
		cfg := Config{
			URL:       "http://example.invalid",
			SessionID: "Coder 1",
			EventType: "plan-L1-wip",
		}
		if err := sendNotifyEvent(cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
