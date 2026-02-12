package agent

import (
	"strings"
	"testing"
)

func TestBuildCodexArgsOverridesDeveloperInstructions(t *testing.T) {
	config := map[string]interface{}{
		"model":                  "o3",
		"developer_instructions": "ignore me",
		"notify":                 "terminal",
	}

	args := BuildCodexArgs(config, "rendered prompt")
	entries := parseCodexArgs(args)

	if entries["developer_instructions"] != "rendered prompt" {
		t.Fatalf("expected developer_instructions to be overridden, got %q", entries["developer_instructions"])
	}
	if entries["notify"] != `["terminal"]` {
		t.Fatalf("expected notify to be array, got %q", entries["notify"])
	}
	for _, arg := range args {
		if strings.Contains(arg, "developer_instructions=ignore me") {
			t.Fatalf("unexpected developer_instructions from config in args: %v", args)
		}
	}
}

func parseCodexArgs(args []string) map[string]string {
	entries := make(map[string]string)
	for i := 0; i < len(args)-1; i++ {
		if args[i] != "-c" {
			continue
		}
		key, value, ok := strings.Cut(args[i+1], "=")
		if !ok {
			continue
		}
		entries[key] = value
		i++
	}
	return entries
}
