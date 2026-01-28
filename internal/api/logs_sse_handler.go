package api

import (
	"context"
	"net/http"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/otel"
)

type LogsSSEHandler struct {
	Logger    *logging.Logger
	AuthToken string
}

func (h *LogsSSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !requireSSEToken(w, r, h.AuthToken, h.Logger) {
		return
	}

	spanCtx, span := startSSESpan(r, "/api/logs/stream")
	defer span.End()

	ctx, cancel := context.WithCancel(spanCtx)
	defer cancel()
	r = r.WithContext(ctx)

	filterLevel := logging.Level("")
	if rawLevel := r.URL.Query().Get("level"); rawLevel != "" {
		if level, ok := logging.ParseLevel(rawLevel); ok {
			filterLevel = level
		}
	}

	hub := otel.ActiveLogHub()
	if hub == nil {
		writeSSEUnavailable(w, r, h.Logger, http.StatusServiceUnavailable, "log stream unavailable")
		return
	}

	output, cancelSubscription := hub.Subscribe()
	if output == nil {
		writeSSEUnavailable(w, r, h.Logger, http.StatusServiceUnavailable, "log stream unavailable")
		return
	}
	defer cancelSubscription()

	writer, err := startSSEWriter(w)
	if err != nil {
		logSSEError(h.Logger, r, sseError{
			Status:  http.StatusInternalServerError,
			Message: "log stream unavailable",
			Err:     err,
		})
		return
	}

	if err := writer.WriteRetry(defaultSSERetryInterval); err != nil {
		return
	}

	snapshot := hub.SnapshotSince(time.Now().Add(-time.Hour))
	if err := writeSSELogSnapshot(writer, snapshot, filterLevel); err != nil {
		return
	}

	runSSEStream(r, writer, sseStreamConfig[map[string]any]{
		Logger:    h.Logger,
		Output:    output,
		SkipRetry: true,
		BuildPayload: func(entry map[string]any) (any, bool) {
			if entry == nil {
				return nil, false
			}
			if filterLevel != "" && !logging.LevelAtLeast(otelLogLevel(entry), filterLevel) {
				return nil, false
			}
			return entry, true
		},
	})

	cancel()
}

func writeSSELogSnapshot(writer *sseWriter, entries []map[string]any, minLevel logging.Level) error {
	if writer == nil || len(entries) == 0 {
		return nil
	}
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if minLevel != "" && !logging.LevelAtLeast(otelLogLevel(entry), minLevel) {
			continue
		}
		if err := writer.WriteEvent("", annotateReplay(entry)); err != nil {
			return err
		}
	}
	return nil
}
