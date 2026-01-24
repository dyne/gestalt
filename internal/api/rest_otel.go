package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/otel"
)

const maxOTelQueryLimit = 1000

type otelLogQuery struct {
	Limit int
	Since *time.Time
	Until *time.Time
	Level logging.Level
	Query string
}

type otelTraceQuery struct {
	Limit    int
	Since    *time.Time
	Until    *time.Time
	TraceID  string
	SpanName string
	Query    string
}

type otelMetricQuery struct {
	Limit int
	Since *time.Time
	Until *time.Time
	Name  string
	Query string
}

func (h *RestHandler) handleOTelLogs(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	query, apiErr := parseOTelLogQuery(r)
	if apiErr != nil {
		return apiErr
	}
	dataPath, ok := activeOTelDataPath()
	if !ok {
		return &apiError{Status: http.StatusServiceUnavailable, Message: "otel logs unavailable"}
	}
	records, readErr := otel.ReadLogRecords(dataPath)
	if readErr != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to read otel logs"}
	}
	filtered := filterOTelLogRecords(records, query)
	writeJSON(w, http.StatusOK, filtered)
	return nil
}

func (h *RestHandler) handleOTelTraces(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	query, apiErr := parseOTelTraceQuery(r)
	if apiErr != nil {
		return apiErr
	}
	dataPath, ok := activeOTelDataPath()
	if !ok {
		return &apiError{Status: http.StatusServiceUnavailable, Message: "otel traces unavailable"}
	}
	records, readErr := otel.ReadTraceRecords(dataPath)
	if readErr != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to read otel traces"}
	}
	filtered := filterOTelTraceRecords(records, query)
	writeJSON(w, http.StatusOK, filtered)
	return nil
}

func (h *RestHandler) handleOTelMetrics(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	query, apiErr := parseOTelMetricQuery(r)
	if apiErr != nil {
		return apiErr
	}
	dataPath, ok := activeOTelDataPath()
	if !ok {
		return &apiError{Status: http.StatusServiceUnavailable, Message: "otel metrics unavailable"}
	}
	records, readErr := otel.ReadMetricRecords(dataPath)
	if readErr != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to read otel metrics"}
	}
	filtered := filterOTelMetricRecords(records, query)
	writeJSON(w, http.StatusOK, filtered)
	return nil
}

func activeOTelDataPath() (string, bool) {
	info, ok := otel.ActiveCollector()
	if !ok || strings.TrimSpace(info.DataPath) == "" {
		return "", false
	}
	return info.DataPath, true
}

func parseOTelLogQuery(r *http.Request) (otelLogQuery, *apiError) {
	values := r.URL.Query()
	query := otelLogQuery{Limit: 200}
	limit, err := parseOTelLimit(values.Get("limit"), query.Limit)
	if err != nil {
		return query, err
	}
	query.Limit = limit

	if rawLevel := strings.TrimSpace(values.Get("level")); rawLevel != "" {
		level, ok := logging.ParseLevel(rawLevel)
		if !ok {
			return query, &apiError{Status: http.StatusBadRequest, Message: "invalid log level"}
		}
		query.Level = level
	}

	if parsed, err := parseTimeParam(values.Get("since")); err != nil {
		return query, err
	} else {
		query.Since = parsed
	}

	if parsed, err := parseTimeParam(values.Get("until")); err != nil {
		return query, err
	} else {
		query.Until = parsed
	}

	query.Query = strings.TrimSpace(values.Get("query"))
	return query, nil
}

func parseOTelTraceQuery(r *http.Request) (otelTraceQuery, *apiError) {
	values := r.URL.Query()
	query := otelTraceQuery{Limit: 200}
	limit, err := parseOTelLimit(values.Get("limit"), query.Limit)
	if err != nil {
		return query, err
	}
	query.Limit = limit
	query.TraceID = strings.TrimSpace(values.Get("trace_id"))
	query.SpanName = strings.TrimSpace(values.Get("span_name"))
	if parsed, err := parseTimeParam(values.Get("since")); err != nil {
		return query, err
	} else {
		query.Since = parsed
	}
	if parsed, err := parseTimeParam(values.Get("until")); err != nil {
		return query, err
	} else {
		query.Until = parsed
	}
	query.Query = strings.TrimSpace(values.Get("query"))
	return query, nil
}

func parseOTelMetricQuery(r *http.Request) (otelMetricQuery, *apiError) {
	values := r.URL.Query()
	query := otelMetricQuery{Limit: 200}
	limit, err := parseOTelLimit(values.Get("limit"), query.Limit)
	if err != nil {
		return query, err
	}
	query.Limit = limit
	query.Name = strings.TrimSpace(values.Get("name"))
	if parsed, err := parseTimeParam(values.Get("since")); err != nil {
		return query, err
	} else {
		query.Since = parsed
	}
	if parsed, err := parseTimeParam(values.Get("until")); err != nil {
		return query, err
	} else {
		query.Until = parsed
	}
	query.Query = strings.TrimSpace(values.Get("query"))
	return query, nil
}

func parseOTelLimit(raw string, defaultLimit int) (int, *apiError) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, &apiError{Status: http.StatusBadRequest, Message: "invalid limit"}
	}
	if limit > maxOTelQueryLimit {
		limit = maxOTelQueryLimit
	}
	return limit, nil
}

func parseTimeParam(raw string) (*time.Time, *apiError) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, &apiError{Status: http.StatusBadRequest, Message: "invalid timestamp"}
	}
	return &parsed, nil
}

func filterOTelLogRecords(records []map[string]any, query otelLogQuery) []map[string]any {
	filtered := make([]map[string]any, 0, len(records))
	for _, record := range records {
		if query.Level != "" && !logging.LevelAtLeast(otelLogLevel(record), query.Level) {
			continue
		}
		if (query.Since != nil || query.Until != nil) && !recordInRange(record, query.Since, query.Until, logTimestampKeys()) {
			continue
		}
		if query.Query != "" && !recordMatchesQuery(record, query.Query) {
			continue
		}
		filtered = append(filtered, record)
	}
	return limitRecords(filtered, query.Limit)
}

func filterOTelTraceRecords(records []map[string]any, query otelTraceQuery) []map[string]any {
	filtered := make([]map[string]any, 0, len(records))
	for _, record := range records {
		if query.TraceID != "" && !strings.EqualFold(otelTraceID(record), query.TraceID) {
			continue
		}
		if query.SpanName != "" && !strings.EqualFold(otelSpanName(record), query.SpanName) {
			continue
		}
		if (query.Since != nil || query.Until != nil) && !recordInRange(record, query.Since, query.Until, traceTimestampKeys()) {
			continue
		}
		if query.Query != "" && !recordMatchesQuery(record, query.Query) {
			continue
		}
		filtered = append(filtered, buildTraceSummary(record))
	}
	return limitRecords(filtered, query.Limit)
}

func filterOTelMetricRecords(records []map[string]any, query otelMetricQuery) []map[string]any {
	filtered := make([]map[string]any, 0, len(records))
	for _, record := range records {
		if query.Name != "" && !strings.EqualFold(otelMetricName(record), query.Name) {
			continue
		}
		if query.Query != "" && !recordMatchesQuery(record, query.Query) {
			continue
		}
		filtered = append(filtered, record)
	}
	return limitRecords(filtered, query.Limit)
}

func limitRecords(records []map[string]any, limit int) []map[string]any {
	if limit <= 0 || len(records) <= limit {
		return records
	}
	return records[len(records)-limit:]
}

func recordInRange(record map[string]any, since, until *time.Time, keys []string) bool {
	timestamp, ok := extractTimestamp(record, keys...)
	if !ok {
		return since == nil && until == nil
	}
	if since != nil && timestamp.Before(*since) {
		return false
	}
	if until != nil && timestamp.After(*until) {
		return false
	}
	return true
}

func logTimestampKeys() []string {
	return []string{
		"timeUnixNano",
		"time_unix_nano",
		"timestamp",
		"time",
		"observedTimeUnixNano",
		"observed_time_unix_nano",
		"observed_timestamp",
		"observedTimestamp",
	}
}

func traceTimestampKeys() []string {
	return []string{
		"startTimeUnixNano",
		"start_time_unix_nano",
		"start_time",
		"startTime",
	}
}

func extractTimestamp(record map[string]any, keys ...string) (time.Time, bool) {
	for _, key := range keys {
		if value, ok := record[key]; ok {
			if ts, ok := parseTimeValue(value); ok {
				return ts, true
			}
		}
	}
	return time.Time{}, false
}

func parseTimeValue(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return time.Time{}, false
		}
		if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return parsed, true
		}
		if numeric, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return timeFromEpoch(numeric), true
		}
	case float64:
		return timeFromEpoch(int64(typed)), true
	case json.Number:
		if numeric, err := typed.Int64(); err == nil {
			return timeFromEpoch(numeric), true
		}
	case int64:
		return timeFromEpoch(typed), true
	case int:
		return timeFromEpoch(int64(typed)), true
	}
	return time.Time{}, false
}

func timeFromEpoch(value int64) time.Time {
	switch {
	case value > 1e15:
		return time.Unix(0, value)
	case value > 1e12:
		return time.UnixMilli(value)
	case value > 1e9:
		return time.Unix(value, 0)
	default:
		return time.Unix(0, value)
	}
}

func otelLogLevel(record map[string]any) logging.Level {
	if text, ok := extractString(record, "severityText", "severity_text", "severity", "level"); ok {
		if level, parsed := logging.ParseLevel(text); parsed {
			return level
		}
	}
	if value, ok := extractNumber(record, "severityNumber", "severity_number"); ok {
		return severityFromNumber(value)
	}
	return logging.LevelInfo
}

func severityFromNumber(value float64) logging.Level {
	switch {
	case value >= 17:
		return logging.LevelError
	case value >= 13:
		return logging.LevelWarning
	case value >= 9:
		return logging.LevelInfo
	default:
		return logging.LevelDebug
	}
}

func otelTraceID(record map[string]any) string {
	value, _ := extractString(record, "traceId", "trace_id")
	return value
}

func otelSpanName(record map[string]any) string {
	value, _ := extractString(record, "name", "span_name")
	return value
}

func otelMetricName(record map[string]any) string {
	value, _ := extractString(record, "name", "metric_name")
	return value
}

func buildTraceSummary(record map[string]any) map[string]any {
	summary := make(map[string]any, len(record)+6)
	for key, value := range record {
		summary[key] = value
	}
	traceID := otelTraceID(record)
	if traceID != "" {
		summary["trace_id"] = traceID
	}
	if spanID, ok := extractString(record, "spanId", "span_id"); ok {
		summary["span_id"] = spanID
	}
	if name, ok := extractString(record, "name", "span_name"); ok {
		summary["name"] = name
	}
	if start, ok := extractTimestamp(record, "startTimeUnixNano", "start_time_unix_nano", "start_time", "startTime"); ok {
		summary["start_time"] = start.UTC().Format(time.RFC3339Nano)
	}
	if duration := spanDuration(record); duration > 0 {
		summary["duration_ms"] = duration
	}
	if status := spanStatus(record); status != "" {
		summary["status"] = status
	}
	if service := resourceServiceName(record); service != "" {
		summary["service_name"] = service
	}
	return summary
}

func spanDuration(record map[string]any) float64 {
	start, ok := extractTimestamp(record, "startTimeUnixNano", "start_time_unix_nano", "start_time", "startTime")
	if !ok {
		return 0
	}
	end, ok := extractTimestamp(record, "endTimeUnixNano", "end_time_unix_nano", "end_time", "endTime")
	if !ok {
		return 0
	}
	duration := end.Sub(start).Seconds() * 1000
	if duration < 0 {
		return 0
	}
	return duration
}

func spanStatus(record map[string]any) string {
	status := asMap(record["status"])
	if status == nil {
		return ""
	}
	if code, ok := extractNumber(status, "code"); ok {
		if code >= 2 {
			return "error"
		}
		return "ok"
	}
	return ""
}

func resourceServiceName(record map[string]any) string {
	resource := asMap(record["resource"])
	if resource == nil {
		return ""
	}
	attributes := asSlice(resource["attributes"])
	for _, attr := range attributes {
		attrMap := asMap(attr)
		if attrMap == nil {
			continue
		}
		key, _ := extractString(attrMap, "key")
		if key != "service.name" {
			continue
		}
		value := asMap(attrMap["value"])
		if value == nil {
			return ""
		}
		if text, ok := extractString(value, "stringValue", "StringValue", "value"); ok {
			return text
		}
	}
	return ""
}

func recordMatchesQuery(record map[string]any, query string) bool {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return true
	}
	if body := extractBodyString(record); body != "" {
		if strings.Contains(strings.ToLower(body), query) {
			return true
		}
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(raw)), query)
}

func extractBodyString(record map[string]any) string {
	body, ok := record["body"]
	if !ok {
		body = record["Body"]
	}
	switch typed := body.(type) {
	case string:
		return typed
	case map[string]any:
		if text, ok := extractString(typed, "stringValue", "StringValue", "value", "Value"); ok {
			return text
		}
	}
	if message, ok := extractString(record, "message", "Message", "event_name", "eventName"); ok {
		return message
	}
	return ""
}

func extractString(values map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			switch typed := value.(type) {
			case string:
				trimmed := strings.TrimSpace(typed)
				if trimmed == "" {
					continue
				}
				return trimmed, true
			case json.Number:
				return typed.String(), true
			}
		}
	}
	return "", false
}

func extractNumber(values map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			switch typed := value.(type) {
			case float64:
				return typed, true
			case float32:
				return float64(typed), true
			case int:
				return float64(typed), true
			case int64:
				return float64(typed), true
			case json.Number:
				if parsed, err := typed.Float64(); err == nil {
					return parsed, true
				}
			case string:
				if parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
					return parsed, true
				}
			}
		}
	}
	return 0, false
}

func asMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	mapped, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return mapped
}

func asSlice(value any) []any {
	if value == nil {
		return nil
	}
	slice, ok := value.([]any)
	if !ok {
		return nil
	}
	return slice
}
