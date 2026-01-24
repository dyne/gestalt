package otel

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
)

const (
	defaultScanBufferSize = 64 * 1024
	maxScanBufferSize     = 10 * 1024 * 1024
	defaultMaxRecords     = 5000
)

func ReadLogRecords(path string) ([]map[string]any, error) {
	return readOTelRecords(path, []string{"resourceLogs", "resource_logs"}, []string{"scopeLogs", "instrumentationLibraryLogs"}, "logRecords")
}

func ReadTraceRecords(path string) ([]map[string]any, error) {
	return readOTelRecords(path, []string{"resourceSpans", "resource_spans"}, []string{"scopeSpans", "instrumentationLibrarySpans"}, "spans")
}

func ReadMetricRecords(path string) ([]map[string]any, error) {
	return readOTelRecords(path, []string{"resourceMetrics", "resource_metrics"}, []string{"scopeMetrics", "instrumentationLibraryMetrics"}, "metrics")
}

func readOTelRecords(path string, resourceKeys, scopeKeys []string, recordKey string) ([]map[string]any, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("otel data path is required")
	}

	records := make([]map[string]any, 0, 128)
	maxRecords := maxRecordLimit()
	err := scanOTelFile(path, func(payload map[string]any) {
		resources := firstSlice(payload, resourceKeys...)
		for _, resourceEntry := range resources {
			resourceLog, ok := resourceEntry.(map[string]any)
			if !ok {
				continue
			}
			resource := asMap(resourceLog["resource"])
			scopes := firstSlice(resourceLog, scopeKeys...)
			for _, scopeEntry := range scopes {
				scopeLog, ok := scopeEntry.(map[string]any)
				if !ok {
					continue
				}
				scope := asMap(scopeLog["scope"])
				recordEntries := asSlice(scopeLog[recordKey])
				for _, recordEntry := range recordEntries {
					record := asMap(recordEntry)
					if record == nil {
						continue
					}
					if resource != nil {
						record["resource"] = resource
					}
					if scope != nil {
						record["scope"] = scope
					}
					if maxRecords > 0 && len(records) >= maxRecords {
						records = records[1:]
					}
					records = append(records, record)
				}
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}

func scanOTelFile(path string, handle func(map[string]any)) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, defaultScanBufferSize), maxScanBufferSize)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			continue
		}
		handle(payload)
	}
	return scanner.Err()
}

func maxRecordLimit() int {
	raw := strings.TrimSpace(os.Getenv("GESTALT_OTEL_MAX_RECORDS"))
	if raw == "" {
		return defaultMaxRecords
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return defaultMaxRecords
	}
	return parsed
}

func firstSlice(values map[string]any, keys ...string) []any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if slice, ok := value.([]any); ok {
				return slice
			}
		}
	}
	return nil
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
