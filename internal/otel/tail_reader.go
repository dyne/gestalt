package otel

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
)

const defaultTailBytes = 16 * 1024 * 1024

type TailOption func(*tailOptions)

type tailOptions struct {
	maxBytes   int64
	maxRecords int
}

func WithTailMaxBytes(bytes int64) TailOption {
	return func(options *tailOptions) {
		if bytes > 0 {
			options.maxBytes = bytes
		}
	}
}

func WithTailMaxRecords(records int) TailOption {
	return func(options *tailOptions) {
		if records > 0 {
			options.maxRecords = records
		}
	}
}

func ReadLogRecordsTail(path string, opts ...TailOption) ([]map[string]any, error) {
	if path == "" {
		return nil, errors.New("otel data path is required")
	}
	options := tailOptions{
		maxBytes:   defaultTailBytes,
		maxRecords: maxRecordLimit(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if options.maxBytes <= 0 {
		options.maxBytes = defaultTailBytes
	}

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	start := int64(0)
	if size > options.maxBytes {
		start = size - options.maxBytes
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	buffer, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	if start > 0 {
		if offset := bytes.IndexByte(buffer, '\n'); offset >= 0 {
			buffer = buffer[offset+1:]
		} else {
			return nil, nil
		}
	}

	lines := bytes.Split(buffer, []byte{'\n'})
	records := make([]map[string]any, 0, 128)
	for lineIndex := len(lines) - 1; lineIndex >= 0; lineIndex-- {
		line := bytes.TrimSpace(lines[lineIndex])
		if len(line) == 0 {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(line, &payload); err != nil {
			continue
		}
		lineRecords := collectOTelRecords(payload, []string{"resourceLogs", "resource_logs"}, []string{"scopeLogs", "instrumentationLibraryLogs"}, "logRecords")
		for recordIndex := len(lineRecords) - 1; recordIndex >= 0; recordIndex-- {
			records = append(records, lineRecords[recordIndex])
			if options.maxRecords > 0 && len(records) >= options.maxRecords {
				reverseRecords(records)
				return records, nil
			}
		}
	}
	reverseRecords(records)
	return records, nil
}

func collectOTelRecords(payload map[string]any, resourceKeys, scopeKeys []string, recordKey string) []map[string]any {
	records := make([]map[string]any, 0, 16)
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
				records = append(records, record)
			}
		}
	}
	return records
}

func reverseRecords(records []map[string]any) {
	for left, right := 0, len(records)-1; left < right; left, right = left+1, right-1 {
		records[left], records[right] = records[right], records[left]
	}
}
