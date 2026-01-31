package main

import (
	"fmt"
	"strconv"
	"strings"

	"gestalt/internal/config/tomlkeys"
)

type overrideList []string

func (o *overrideList) String() string {
	if o == nil {
		return ""
	}
	return strings.Join(*o, ",")
}

func (o *overrideList) Set(value string) error {
	*o = append(*o, value)
	return nil
}

func parseConfigOverrides(entries []string) (map[string]any, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	overrides := make(map[string]any)
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			return nil, fmt.Errorf("config override cannot be empty")
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("config override must be key=value: %q", entry)
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			return nil, fmt.Errorf("config override key cannot be empty")
		}
		normalizedKey := tomlkeys.NormalizeKey(key)
		if normalizedKey == "" {
			return nil, fmt.Errorf("config override key cannot be empty")
		}
		value := parseOverrideValue(strings.TrimSpace(parts[1]))
		overrides[normalizedKey] = value
	}
	return overrides, nil
}

func parseConfigOverridesEnv(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	parts := strings.Split(trimmed, ",")
	entries := make([]string, 0, len(parts))
	for _, part := range parts {
		entry := strings.TrimSpace(part)
		if entry == "" {
			return nil, fmt.Errorf("config override entry cannot be empty")
		}
		entries = append(entries, entry)
	}
	return parseConfigOverrides(entries)
}

func parseOverrideValue(value string) any {
	if strings.EqualFold(value, "true") {
		return true
	}
	if strings.EqualFold(value, "false") {
		return false
	}
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		return parsed
	}
	return value
}
