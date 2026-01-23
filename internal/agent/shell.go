package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type flatEntry struct {
	key   string
	value interface{}
}

// BuildShellCommand builds a shell command string from cli_config settings.
func BuildShellCommand(cliType string, config map[string]interface{}) string {
	args := buildShellArgs(cliType, config)
	if len(args) == 0 {
		return ""
	}
	return strings.Join(args, " ")
}

func buildShellArgs(cliType string, config map[string]interface{}) []string {
	cliType = strings.ToLower(strings.TrimSpace(cliType))
	if cliType == "" {
		return nil
	}

	entries := flattenConfig(config)
	args := []string{cliType}
	switch cliType {
	case "codex":
		for _, entry := range entries {
			value := formatValue(entry.value)
			args = append(args, "-c", escapeShellArg(fmt.Sprintf("%s:%s", entry.key, value)))
		}
	case "copilot":
		for _, entry := range entries {
			flag := "--" + normalizeFlag(entry.key)
			switch value := entry.value.(type) {
			case bool:
				if value {
					args = append(args, flag)
				} else {
					args = append(args, "--no-"+normalizeFlag(entry.key))
				}
			default:
				args = append(args, flag, escapeShellArg(formatValue(value)))
			}
		}
	default:
		return nil
	}
	return args
}

func flattenConfig(config map[string]interface{}) []flatEntry {
	entries := []flatEntry{}
	flattenMap("", config, &entries)
	return entries
}

func flattenMap(prefix string, value interface{}, entries *[]flatEntry) {
	if value == nil {
		return
	}

	if list, ok := value.([]interface{}); ok {
		for _, item := range list {
			*entries = append(*entries, flatEntry{key: prefix, value: item})
		}
		return
	}

	if list, ok := value.([]string); ok {
		for _, item := range list {
			*entries = append(*entries, flatEntry{key: prefix, value: item})
		}
		return
	}

	mapValue, ok := asStringMap(value)
	if !ok {
		*entries = append(*entries, flatEntry{key: prefix, value: value})
		return
	}

	keys := make([]string, 0, len(mapValue))
	for key := range mapValue {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		child := mapValue[key]
		childKey := key
		if prefix != "" {
			childKey = prefix + "." + key
		}
		flattenMap(childKey, child, entries)
	}
}

func normalizeFlag(name string) string {
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, "_", "-")
	return name
}

func formatValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int8, int16, int32, int64:
		return fmt.Sprintf("%d", typed)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", typed)
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(payload)
}

func escapeShellArg(value string) string {
	if value == "" {
		return "''"
	}
	if !needsQuoting(value) {
		return value
	}
	replacer := strings.NewReplacer("'", "'\"'\"'")
	return "'" + replacer.Replace(value) + "'"
}

func needsQuoting(value string) bool {
	for _, r := range value {
		switch r {
		case ' ', '\t', '\n', '\r', '\'', '"', '\\', '$', '&', ';', '|', '>', '<', '(', ')', '*', '?', '[', ']', '{', '}', '!', '#':
			return true
		}
	}
	return false
}
