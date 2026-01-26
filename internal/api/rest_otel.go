package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/otel"

	otellog "go.opentelemetry.io/otel/log"
	logglobal "go.opentelemetry.io/otel/log/global"
)

const (
	maxOTelQueryLimit      = 1000
	maxOTelLogBodyBytes    = 256 * 1024
	maxOTelLogAttributes   = 100
	maxOTelLogKeyLength    = 256
	maxOTelLogValueLength  = 2048
	otlpUILoggerName       = "gestalt/ui"
	defaultLogReplayWindow = time.Hour
)

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
	switch r.Method {
	case http.MethodGet:
		query, apiErr := parseOTelLogQuery(r)
		if apiErr != nil {
			return apiErr
		}
		var records []map[string]any
		if dataPath, ok := activeOTelDataPath(); ok {
			readRecords, readErr := otel.ReadLogRecordsTail(dataPath)
			if readErr != nil {
				return &apiError{Status: http.StatusInternalServerError, Message: "failed to read otel logs"}
			}
			records = readRecords
		} else if hub := otel.ActiveLogHub(); hub != nil {
			records = hub.SnapshotSince(time.Now().Add(-defaultLogReplayWindow))
		} else {
			return &apiError{Status: http.StatusServiceUnavailable, Message: "otel logs unavailable"}
		}
		filtered := filterOTelLogRecords(records, query)
		writeJSON(w, http.StatusOK, filtered)
		return nil
	case http.MethodPost:
		return ingestOTelLogRecord(w, r)
	default:
		return methodNotAllowed(w, "GET, POST")
	}
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

func ingestOTelLogRecord(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Body == nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}
	limited := http.MaxBytesReader(w, r.Body, maxOTelLogBodyBytes*2)
	decoder := json.NewDecoder(limited)
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil && err != io.EOF {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}

	severityNumber, severityText, apiErr := parseOTelSeverity(payload)
	if apiErr != nil {
		return apiErr
	}
	bodyValue, apiErr := parseOTelLogBody(payload["body"])
	if apiErr != nil {
		return apiErr
	}
	attributes, apiErr := parseOTelLogAttributes(payload["attributes"])
	if apiErr != nil {
		return apiErr
	}
	attributes = ensureOTelLogDefaults(attributes)
	if len(attributes) > maxOTelLogAttributes {
		return &apiError{Status: http.StatusBadRequest, Message: "too many attributes"}
	}

	logger := logglobal.Logger(otlpUILoggerName)
	now := time.Now().UTC()
	var record otellog.Record
	record.SetTimestamp(now)
	record.SetObservedTimestamp(now)
	if severityNumber > 0 {
		record.SetSeverity(otellog.Severity(severityNumber))
	}
	if severityText != "" {
		record.SetSeverityText(severityText)
	}
	record.SetBody(bodyValue)
	if len(attributes) > 0 {
		record.AddAttributes(attributes...)
	}
	logger.Emit(r.Context(), record)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func parseOTelSeverity(payload map[string]any) (int, string, *apiError) {
	var severityNumber int
	var severityText string
	if raw, ok := payload["severity_text"].(string); ok {
		severityText = strings.TrimSpace(raw)
	}
	if rawNumber, ok := payload["severity_number"]; ok {
		parsed, ok := parseOTelSeverityNumber(rawNumber)
		if !ok {
			return 0, "", &apiError{Status: http.StatusBadRequest, Message: "invalid severity number"}
		}
		severityNumber = parsed
	}
	if severityText == "" && severityNumber == 0 {
		severityText = "info"
		severityNumber = int(otellog.SeverityInfo)
	}
	if severityNumber == 0 && severityText != "" {
		severityNumber = int(severityFromText(severityText))
	}
	if severityText == "" && severityNumber > 0 {
		severityText = otellog.Severity(severityNumber).String()
	}
	return severityNumber, severityText, nil
}

func parseOTelSeverityNumber(raw any) (int, bool) {
	switch value := raw.(type) {
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return int(parsed), true
		}
		if parsed, err := value.Float64(); err == nil {
			return int(parsed), true
		}
	case float64:
		return int(value), true
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func severityFromText(raw string) otellog.Severity {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(normalized, "debug"), strings.HasPrefix(normalized, "trace"):
		return otellog.SeverityDebug
	case strings.HasPrefix(normalized, "warn"):
		return otellog.SeverityWarn
	case strings.HasPrefix(normalized, "err"), strings.HasPrefix(normalized, "fatal"):
		return otellog.SeverityError
	default:
		return otellog.SeverityInfo
	}
}

func parseOTelLogBody(raw any) (otellog.Value, *apiError) {
	if raw == nil {
		return otellog.Value{}, &apiError{Status: http.StatusBadRequest, Message: "missing body"}
	}
	value, apiErr := parseOTLPAnyValue(raw)
	if apiErr != nil {
		return otellog.Value{}, apiErr
	}
	if value.Kind() == otellog.KindString {
		if len([]byte(value.AsString())) > maxOTelLogBodyBytes {
			return otellog.Value{}, &apiError{Status: http.StatusBadRequest, Message: "body too large"}
		}
	}
	return value, nil
}

func parseOTelLogAttributes(raw any) ([]otellog.KeyValue, *apiError) {
	if raw == nil {
		return nil, nil
	}
	switch typed := raw.(type) {
	case map[string]any:
		attributes := make([]otellog.KeyValue, 0, len(typed))
		for key, value := range typed {
			if !validOTelKey(key) {
				return nil, &apiError{Status: http.StatusBadRequest, Message: "invalid attribute key"}
			}
			parsed, apiErr := parseOTLPAnyValue(value)
			if apiErr != nil {
				return nil, apiErr
			}
			if err := validateOTelValueLength(parsed); err != nil {
				return nil, err
			}
			attributes = append(attributes, otellog.KeyValue{Key: key, Value: parsed})
		}
		return attributes, nil
	case []any:
		attributes := make([]otellog.KeyValue, 0, len(typed))
		for _, entry := range typed {
			entryMap := asMap(entry)
			if entryMap == nil {
				continue
			}
			key, _ := extractString(entryMap, "key")
			if !validOTelKey(key) {
				return nil, &apiError{Status: http.StatusBadRequest, Message: "invalid attribute key"}
			}
			value, apiErr := parseOTLPAnyValue(entryMap["value"])
			if apiErr != nil {
				return nil, apiErr
			}
			if err := validateOTelValueLength(value); err != nil {
				return nil, err
			}
			attributes = append(attributes, otellog.KeyValue{Key: key, Value: value})
		}
		return attributes, nil
	default:
		return nil, &apiError{Status: http.StatusBadRequest, Message: "invalid attributes"}
	}
}

func parseOTLPAnyValue(raw any) (otellog.Value, *apiError) {
	switch value := raw.(type) {
	case string:
		return otellog.StringValue(value), nil
	case bool:
		return otellog.BoolValue(value), nil
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return otellog.Int64Value(parsed), nil
		}
		if parsed, err := value.Float64(); err == nil {
			return otellog.Float64Value(parsed), nil
		}
	case float64:
		return otellog.Float64Value(value), nil
	case map[string]any:
		if stringValue, ok := extractString(value, "stringValue"); ok {
			return otellog.StringValue(stringValue), nil
		}
		if boolValue, ok := value["boolValue"].(bool); ok {
			return otellog.BoolValue(boolValue), nil
		}
		if intValue, ok := value["intValue"]; ok {
			if parsed, ok := parseOTelIntValue(intValue); ok {
				return otellog.Int64Value(parsed), nil
			}
		}
		if doubleValue, ok := value["doubleValue"]; ok {
			if parsed, ok := doubleValue.(float64); ok {
				return otellog.Float64Value(parsed), nil
			}
		}
		if arrayValue, ok := value["arrayValue"].(map[string]any); ok {
			values := asSlice(arrayValue["values"])
			converted := make([]otellog.Value, 0, len(values))
			for _, entry := range values {
				parsed, apiErr := parseOTLPAnyValue(entry)
				if apiErr != nil {
					return otellog.Value{}, apiErr
				}
				converted = append(converted, parsed)
			}
			return otellog.SliceValue(converted...), nil
		}
		if kvlistValue, ok := value["kvlistValue"].(map[string]any); ok {
			values := asSlice(kvlistValue["values"])
			converted := make([]otellog.KeyValue, 0, len(values))
			for _, entry := range values {
				entryMap := asMap(entry)
				if entryMap == nil {
					continue
				}
				key, _ := extractString(entryMap, "key")
				if !validOTelKey(key) {
					return otellog.Value{}, &apiError{Status: http.StatusBadRequest, Message: "invalid attribute key"}
				}
				parsed, apiErr := parseOTLPAnyValue(entryMap["value"])
				if apiErr != nil {
					return otellog.Value{}, apiErr
				}
				if err := validateOTelValueLength(parsed); err != nil {
					return otellog.Value{}, err
				}
				converted = append(converted, otellog.KeyValue{Key: key, Value: parsed})
			}
			return otellog.MapValue(converted...), nil
		}
	}
	return otellog.Value{}, &apiError{Status: http.StatusBadRequest, Message: "invalid value"}
}

func parseOTelIntValue(raw any) (int64, bool) {
	switch value := raw.(type) {
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return parsed, true
		}
	case float64:
		return int64(value), true
	case string:
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func validateOTelValueLength(value otellog.Value) *apiError {
	if value.Kind() == otellog.KindString && len(value.AsString()) > maxOTelLogValueLength {
		return &apiError{Status: http.StatusBadRequest, Message: "attribute value too large"}
	}
	return nil
}

func validOTelKey(key string) bool {
	if strings.TrimSpace(key) == "" {
		return false
	}
	return len(key) <= maxOTelLogKeyLength
}

func ensureOTelLogDefaults(attributes []otellog.KeyValue) []otellog.KeyValue {
	foundSource := false
	foundCategory := false
	for _, entry := range attributes {
		switch entry.Key {
		case "gestalt.source":
			foundSource = true
		case "gestalt.category":
			foundCategory = true
		}
	}
	if !foundSource {
		attributes = append(attributes, otellog.String("gestalt.source", "frontend"))
	}
	if !foundCategory {
		attributes = append(attributes, otellog.String("gestalt.category", "ui"))
	}
	return attributes
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
