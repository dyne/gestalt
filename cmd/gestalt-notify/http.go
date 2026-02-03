package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"gestalt/internal/client"
)

var httpClient = &http.Client{Timeout: defaultNotifyTimeout}

type notifyError struct {
	Code    int
	Message string
}

func (e *notifyError) Error() string {
	return e.Message
}

func notifyErr(code int, message string) *notifyError {
	return &notifyError{Code: code, Message: message}
}

func notifyErrf(code int, format string, args ...any) *notifyError {
	return &notifyError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func notifyErrFromClient(err error) *notifyError {
	var httpErr *client.HTTPError
	if errors.As(err, &httpErr) {
		if httpErr.StatusCode >= http.StatusBadRequest && httpErr.StatusCode < http.StatusInternalServerError {
			return notifyErr(2, httpErr.Message)
		}
		return notifyErr(3, httpErr.Message)
	}
	return notifyErrf(3, "%v", err)
}

func handleNotifyError(err error, errOut io.Writer) int {
	var notifyErr *notifyError
	if errors.As(err, &notifyErr) {
		fmt.Fprintln(errOut, notifyErr.Message)
		if notifyErr.Code != 0 {
			return notifyErr.Code
		}
	}
	fmt.Fprintln(errOut, err.Error())
	return 3
}

func sendNotifyEvent(cfg Config) error {
	baseURL := strings.TrimRight(cfg.URL, "/")
	if baseURL == "" {
		baseURL = defaultServerURL
	}

	payload := client.NotifyRequest{
		SessionID:  cfg.SessionID,
		OccurredAt: cfg.OccurredAt,
		Payload:    cfg.Payload,
		Raw:        cfg.Raw,
	}

	if cfg.Verbose {
		escapedID := url.PathEscape(cfg.SessionID)
		target := fmt.Sprintf("%s/api/sessions/%s/notify", baseURL, escapedID)
		logf(cfg, "posting notify event to %s", target)
		if strings.TrimSpace(cfg.Token) != "" {
			logf(cfg, "token: %s", maskToken(cfg.Token, cfg.Debug))
		}
	}
	if cfg.Debug {
		if len(cfg.Payload) > 0 {
			preview := cfg.Payload
			if len(preview) > 200 {
				preview = preview[:200]
			}
			logf(cfg, "payload preview: %s", string(preview))
		}
		if strings.TrimSpace(cfg.Raw) != "" {
			logf(cfg, "raw payload: %s", cfg.Raw)
		}
	}

	if err := client.PostNotifyEvent(httpClient, baseURL, cfg.Token, cfg.SessionID, payload); err != nil {
		return notifyErrFromClient(err)
	}
	if cfg.Verbose {
		logf(cfg, "response status: %d %s", http.StatusNoContent, http.StatusText(http.StatusNoContent))
	}
	return nil
}

func logf(cfg Config, format string, args ...any) {
	if cfg.LogWriter == nil || !(cfg.Verbose || cfg.Debug) {
		return
	}
	fmt.Fprintf(cfg.LogWriter, format+"\n", args...)
}

func maskToken(token string, debug bool) string {
	if debug {
		return token
	}
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 4 {
		return "****"
	}
	return trimmed[:2] + strings.Repeat("*", len(trimmed)-4) + trimmed[len(trimmed)-2:]
}

func applyTimeout(cfg Config) {
	if cfg.Timeout <= 0 {
		return
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
		return
	}
	if httpClient.Timeout != cfg.Timeout {
		httpClient.Timeout = cfg.Timeout
	}
}
