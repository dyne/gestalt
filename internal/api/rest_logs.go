package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/logging"
)

func (h *RestHandler) handleLogs(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireLogger(); err != nil {
		return err
	}
	switch r.Method {
	case http.MethodGet:
		query, err := parseLogQuery(r)
		if err != nil {
			return err
		}

		entries := h.Logger.Buffer().List()
		filtered := filterLogEntries(entries, query)
		writeJSON(w, http.StatusOK, filtered)
		return nil
	case http.MethodPost:
		return h.createLogEntry(w, r)
	default:
		return methodNotAllowed(w, "GET, POST")
	}
}

func (h *RestHandler) createLogEntry(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Body == nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}

	var request clientLogRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil && err != io.EOF {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}

	message := strings.TrimSpace(request.Message)
	if message == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "missing log message"}
	}

	level := logging.LevelInfo
	if rawLevel := strings.TrimSpace(request.Level); rawLevel != "" {
		parsed, ok := logging.ParseLevel(rawLevel)
		if !ok {
			return &apiError{Status: http.StatusBadRequest, Message: "invalid log level"}
		}
		level = parsed
	}

	fields := make(map[string]string, len(request.Context)+2)
	for key, value := range request.Context {
		if strings.TrimSpace(key) == "" {
			continue
		}
		fields[key] = value
	}
	if _, ok := fields["source"]; !ok {
		fields["source"] = "frontend"
	}
	if _, ok := fields["toast"]; !ok {
		fields["toast"] = "true"
	}

	switch level {
	case logging.LevelDebug:
		h.Logger.Debug(message, fields)
	case logging.LevelWarning:
		h.Logger.Warn(message, fields)
	case logging.LevelError:
		h.Logger.Error(message, fields)
	default:
		h.Logger.Info(message, fields)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func parseLogQuery(r *http.Request) (logQuery, *apiError) {
	values := r.URL.Query()
	query := logQuery{
		Limit: 100,
	}

	if rawLimit := strings.TrimSpace(values.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 {
			return query, &apiError{Status: http.StatusBadRequest, Message: "invalid limit"}
		}
		query.Limit = limit
	}

	if rawSince := strings.TrimSpace(values.Get("since")); rawSince != "" {
		parsed, err := time.Parse(time.RFC3339, rawSince)
		if err != nil {
			return query, &apiError{Status: http.StatusBadRequest, Message: "invalid since timestamp"}
		}
		query.Since = &parsed
	}

	if rawLevel := strings.TrimSpace(values.Get("level")); rawLevel != "" {
		level, ok := logging.ParseLevel(rawLevel)
		if !ok {
			return query, &apiError{Status: http.StatusBadRequest, Message: "invalid log level"}
		}
		query.Level = level
	}

	return query, nil
}

func filterLogEntries(entries []logging.LogEntry, query logQuery) []logging.LogEntry {
	filtered := make([]logging.LogEntry, 0, len(entries))
	for _, entry := range entries {
		if query.Level != "" && !logging.LevelAtLeast(entry.Level, query.Level) {
			continue
		}
		if query.Since != nil && entry.Timestamp.Before(*query.Since) {
			continue
		}
		filtered = append(filtered, entry)
	}

	if query.Limit > 0 && len(filtered) > query.Limit {
		filtered = filtered[len(filtered)-query.Limit:]
	}

	return filtered
}
