package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gestalt/internal/client"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type sendError struct {
	Code    int
	Message string
}

func (e *sendError) Error() string {
	return e.Message
}

func sendErr(code int, message string) *sendError {
	return &sendError{Code: code, Message: message}
}

func sendErrf(code int, format string, args ...any) *sendError {
	return &sendError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func handleSendError(err error, errOut io.Writer) int {
	var sendErr *sendError
	if errors.As(err, &sendErr) {
		fmt.Fprintln(errOut, sendErr.Message)
		if sendErr.Code != 0 {
			return sendErr.Code
		}
	}
	fmt.Fprintln(errOut, err.Error())
	return 3
}

func sendInput(cfg Config, payload []byte) error {
	return sendSessionInput(cfg, payload)
}

func sendSessionInput(cfg Config, payload []byte) error {
	sessionRef := strings.TrimSpace(cfg.SessionRef)
	if sessionRef == "" {
		return sendErr(2, "session reference is required")
	}
	baseURL := strings.TrimRight(cfg.URL, "/")
	sessions, err := client.FetchSessions(httpClient, baseURL, cfg.Token)
	if err != nil {
		var httpErr *client.HTTPError
		if errors.As(err, &httpErr) {
			return sendErr(3, httpErr.Message)
		}
		return sendErrf(3, "%v", err)
	}
	sessionID, err := client.ResolveSessionRefAgainstSessions(sessionRef, sessions)
	if err != nil {
		return sendErr(2, err.Error())
	}

	target := fmt.Sprintf("%s/api/sessions/%s/input", baseURL, sessionID)
	if cfg.Verbose {
		logf(cfg, "sending %d bytes to session %q (from %q) at %s", len(payload), sessionID, sessionRef, target)
		if strings.TrimSpace(cfg.Token) != "" {
			logf(cfg, "token: %s", maskToken(cfg.Token, cfg.Debug))
		}
	}
	if cfg.Debug && len(payload) > 0 {
		preview := payload
		if len(preview) > 100 {
			preview = preview[:100]
		}
		logf(cfg, "payload preview: %q", string(preview))
	}

	if err := client.SendSessionInput(httpClient, baseURL, cfg.Token, sessionID, payload); err != nil {
		var httpErr *client.HTTPError
		if errors.As(err, &httpErr) {
			if cfg.Verbose && httpErr.StatusCode != 0 {
				logf(cfg, "response status: %d %s", httpErr.StatusCode, http.StatusText(httpErr.StatusCode))
			}
			if httpErr.StatusCode == http.StatusNotFound {
				return sendErr(2, fmt.Sprintf("%s (resolved from %q)", httpErr.Message, sessionRef))
			}
			return sendErr(3, httpErr.Message)
		}
		return sendErrf(3, "%v", err)
	}
	if cfg.Verbose {
		logf(cfg, "response status: %d %s", http.StatusOK, http.StatusText(http.StatusOK))
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
