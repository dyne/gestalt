package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/logging"
)

const (
	defaultSSEHeartbeatInterval = 15 * time.Second
	defaultSSERetryInterval     = 5 * time.Second
)

var errSSENoFlusher = errors.New("sse response writer does not support flushing")

type sseStreamConfig[T any] struct {
	Logger            *logging.Logger
	Output            <-chan T
	BuildPayload      func(T) (any, bool)
	EventName         string
	HeartbeatInterval time.Duration
	RetryInterval     time.Duration
}

type sseError struct {
	Status  int
	Message string
	Err     error
}

type sseWriter struct {
	writer  http.ResponseWriter
	flusher http.Flusher
}

func requireSSEToken(w http.ResponseWriter, r *http.Request, token string, logger *logging.Logger) bool {
	if !validateToken(r, token) {
		writeSSEHTTPError(w, r, logger, sseError{
			Status:  http.StatusUnauthorized,
			Message: "unauthorized",
		})
		return false
	}
	return true
}

func serveSSEStream[T any](w http.ResponseWriter, r *http.Request, config sseStreamConfig[T]) {
	if config.Output == nil {
		return
	}

	writer, err := startSSEWriter(w)
	if err != nil {
		logSSEError(config.Logger, r, sseError{
			Status:  http.StatusInternalServerError,
			Message: "sse stream unavailable",
			Err:     err,
		})
		return
	}
	runSSEStream(r, writer, config)
}

func runSSEStream[T any](r *http.Request, writer *sseWriter, config sseStreamConfig[T]) {
	if writer == nil || config.Output == nil {
		return
	}

	retryInterval := config.RetryInterval
	if retryInterval <= 0 {
		retryInterval = defaultSSERetryInterval
	}
	if err := writer.WriteRetry(retryInterval); err != nil {
		return
	}

	heartbeatInterval := config.HeartbeatInterval
	if heartbeatInterval <= 0 {
		heartbeatInterval = defaultSSEHeartbeatInterval
	}
	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()

	buildPayload := config.BuildPayload
	if buildPayload == nil {
		buildPayload = func(value T) (any, bool) {
			return value, true
		}
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeatTicker.C:
			if err := writer.WriteComment("ping"); err != nil {
				return
			}
		case event, ok := <-config.Output:
			if !ok {
				return
			}
			payload, ok := buildPayload(event)
			if !ok {
				continue
			}
			if err := writer.WriteEvent(config.EventName, payload); err != nil {
				return
			}
		}
	}
}

func writeSSEErrorEvent(writer *sseWriter, status int, message string) error {
	payload := wsErrorPayload{
		Type:    "error",
		Message: message,
		Status:  status,
	}
	return writer.WriteEvent("", payload)
}

func startSSEWriter(w http.ResponseWriter) (*sseWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errSSENoFlusher
	}

	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", cacheControlNoStore)
	headers.Set("Connection", "keep-alive")
	headers.Set("X-Accel-Buffering", "no")

	flusher.Flush()
	return &sseWriter{writer: w, flusher: flusher}, nil
}

func (writer *sseWriter) WriteRetry(retry time.Duration) error {
	if writer == nil {
		return errors.New("sse writer missing")
	}
	if retry <= 0 {
		return nil
	}

	milliseconds := retry.Milliseconds()
	if milliseconds <= 0 {
		milliseconds = int64(retry / time.Millisecond)
	}
	if _, err := io.WriteString(writer.writer, "retry: "); err != nil {
		return err
	}
	if _, err := io.WriteString(writer.writer, strconv.FormatInt(milliseconds, 10)); err != nil {
		return err
	}
	if _, err := io.WriteString(writer.writer, "\n\n"); err != nil {
		return err
	}
	writer.flusher.Flush()
	return nil
}

func (writer *sseWriter) WriteComment(comment string) error {
	if writer == nil {
		return errors.New("sse writer missing")
	}
	if _, err := io.WriteString(writer.writer, ": "); err != nil {
		return err
	}
	if _, err := io.WriteString(writer.writer, strings.TrimSpace(comment)); err != nil {
		return err
	}
	if _, err := io.WriteString(writer.writer, "\n\n"); err != nil {
		return err
	}
	writer.flusher.Flush()
	return nil
}

func (writer *sseWriter) WriteEvent(eventName string, payload any) error {
	if writer == nil {
		return errors.New("sse writer missing")
	}

	if eventName != "" {
		if _, err := io.WriteString(writer.writer, "event: "); err != nil {
			return err
		}
		if _, err := io.WriteString(writer.writer, eventName); err != nil {
			return err
		}
		if _, err := io.WriteString(writer.writer, "\n"); err != nil {
			return err
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := writeSSEData(writer.writer, data); err != nil {
		return err
	}
	writer.flusher.Flush()
	return nil
}

func writeSSEData(writer io.Writer, data []byte) error {
	if len(data) == 0 {
		_, err := io.WriteString(writer, "data:\n\n")
		return err
	}

	for _, line := range bytes.Split(data, []byte("\n")) {
		if _, err := io.WriteString(writer, "data: "); err != nil {
			return err
		}
		if _, err := writer.Write(line); err != nil {
			return err
		}
		if _, err := io.WriteString(writer, "\n"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(writer, "\n")
	return err
}

func writeSSEHTTPError(w http.ResponseWriter, r *http.Request, logger *logging.Logger, sseErr sseError) {
	status := sseErr.Status
	if status == 0 {
		status = http.StatusInternalServerError
	}

	reason := strings.TrimSpace(sseErr.Message)
	if reason == "" {
		reason = http.StatusText(status)
		if reason == "" {
			reason = "sse error"
		}
	}

	logSSEError(logger, r, sseError{
		Status:  status,
		Message: reason,
		Err:     sseErr.Err,
	})

	http.Error(w, reason, status)
}

func logSSEError(logger *logging.Logger, r *http.Request, sseErr sseError) {
	if logger == nil || r == nil {
		return
	}

	fields := map[string]string{
		"path":    r.URL.Path,
		"status":  strconv.Itoa(sseErr.Status),
		"message": sseErr.Message,
	}
	if r.RemoteAddr != "" {
		fields["remote_addr"] = r.RemoteAddr
	}
	if userAgent := strings.TrimSpace(r.UserAgent()); userAgent != "" {
		fields["user_agent"] = userAgent
	}
	if sseErr.Err != nil {
		fields["error"] = sseErr.Err.Error()
	}

	if sseErr.Status >= http.StatusInternalServerError {
		logger.Error("sse error", fields)
	} else {
		logger.Warn("sse error", fields)
	}
}
