package main

import (
	"strings"
	"testing"
)

func TestBuildCodexArgsPreservesPrompt(t *testing.T) {
	config := map[string]interface{}{
		"model":            "gpt-4",
		"notify":           "bell",
		"developer_prompt": "old",
	}
	prompt := "line1\n\"quote\"\n"
	args := buildCodexArgs(config, prompt)

	value, count := findArgValue(args, "developer_prompt")
	if count != 1 {
		t.Fatalf("expected one developer_prompt arg, got %d", count)
	}
	if value != prompt {
		t.Fatalf("unexpected developer_prompt value: %q", value)
	}
	if strings.Contains(value, "\\n") {
		t.Fatalf("expected literal newlines, got escaped value %q", value)
	}
	if strings.HasPrefix(value, "'") || strings.HasSuffix(value, "'") {
		t.Fatalf("unexpected shell quoting in developer_prompt: %q", value)
	}

	notify, _ := findArgValue(args, "notify")
	if notify != "[\"bell\"]" {
		t.Fatalf("expected notify array, got %q", notify)
	}
}

func findArgValue(args []string, key string) (string, int) {
	count := 0
	value := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] != "-c" {
			continue
		}
		entry := args[i+1]
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if parts[0] == key {
			count++
			value = parts[1]
		}
	}
	return value, count
}
