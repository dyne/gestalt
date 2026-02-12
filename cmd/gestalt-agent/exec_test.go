package main

import (
	"strings"
	"testing"

	agentpkg "gestalt/internal/agent"
)

func TestBuildCodexArgsPreservesPrompt(t *testing.T) {
	config := map[string]interface{}{
		"model":                  "gpt-4",
		"notify":                 "bell",
		"developer_instructions": "old",
	}
	prompt := "line1\n\"quote\"\n"
	args := agentpkg.BuildCodexArgs(config, prompt)

	value, count := findArgValue(args, "developer_instructions")
	if count != 1 {
		t.Fatalf("expected one developer_instructions arg, got %d", count)
	}
	if value != prompt {
		t.Fatalf("unexpected developer_instructions value: %q", value)
	}
	if strings.Contains(value, "\\n") {
		t.Fatalf("expected literal newlines, got escaped value %q", value)
	}
	if strings.HasPrefix(value, "'") || strings.HasSuffix(value, "'") {
		t.Fatalf("unexpected shell quoting in developer_instructions: %q", value)
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
