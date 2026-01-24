package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/otel"
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

		if dataPath, ok := activeOTelDataPath(); ok {
			entries, readErr := otelLogEntries(dataPath)
			if readErr != nil {
				return &apiError{Status: http.StatusServiceUnavailable, Message: "otel logs unavailable"}
			}
			filtered := filterLogEntries(entries, query)
			writeJSON(w, http.StatusOK, filtered)
			return nil
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

func otelLogEntries(dataPath string) ([]logging.LogEntry, error) {
	records, err := otel.ReadLogRecords(dataPath)
	if err != nil {
		return nil, err
	}
	entries := make([]logging.LogEntry, 0, len(records))
	for _, record := range records {
		entries = append(entries, otelLogRecordToEntry(record))
	}
	return entries, nil
}

func otelLogRecordToEntry(record map[string]any) logging.LogEntry {
	timestamp := time.Now().UTC()
	if parsed, ok := extractTimestamp(record, logTimestampKeys()...); ok {
		timestamp = parsed
	}
	level := otelLogLevel(record)
	message := extractBodyString(record)
	context := otelAttributesToContext(record)
	if len(context) == 0 {
		context = nil
	}
	return logging.LogEntry{
		Timestamp: timestamp,
		Level:     level,
		Message:   message,
		Context:   context,
	}
}

func otelAttributesToContext(record map[string]any) map[string]string {
	attributes := asSlice(record["attributes"])
	if len(attributes) == 0 {
		return nil
	}
	context := make(map[string]string, len(attributes))
	for _, attr := range attributes {
		attrMap := asMap(attr)
		if attrMap == nil {
			continue
		}
		key, _ := extractString(attrMap, "key")
		if key == "" {
			continue
		}
		value := otelValueToString(attrMap["value"])
		if value == "" {
			continue
		}
		context[key] = value
	}
	return context
}

func otelValueToString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case bool:
		return strconv.FormatBool(typed)
	case map[string]any:
		if text, ok := extractString(typed, "stringValue", "StringValue", "value", "Value"); ok {
			return text
		}
		if number, ok := extractNumber(typed, "intValue", "doubleValue", "boolValue"); ok {
			return strconv.FormatFloat(number, 'f', -1, 64)
		}
		if raw, err := json.Marshal(typed); err == nil {
			return string(raw)
		}
	}
	return fmt.Sprint(value)
}
