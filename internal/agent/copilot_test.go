package agent

import (
	"strings"
	"testing"
)

func TestCopilotConfigValid(t *testing.T) {
	config := map[string]interface{}{
		"model":           "gpt-5",
		"allow_all_tools": true,
		"add_dir":         []interface{}{`/tmp`},
		"log_level":       "info",
		"resume":          true,
		"stream":          "on",
	}

	if err := ValidateAgentConfig("copilot", config); err != nil {
		t.Fatalf("expected config to be valid, got %v", err)
	}
}

func TestCopilotConfigResumeString(t *testing.T) {
	config := map[string]interface{}{
		"resume": "session-123",
	}
	if err := ValidateAgentConfig("copilot", config); err != nil {
		t.Fatalf("expected resume string to be valid, got %v", err)
	}
}

func TestCopilotConfigUnknownField(t *testing.T) {
	err := ValidateAgentConfig("copilot", map[string]interface{}{
		"not_a_flag": true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not_a_flag") {
		t.Fatalf("expected unknown field in error, got %v", err)
	}
}

func TestCopilotConfigTypeMismatch(t *testing.T) {
	err := ValidateAgentConfig("copilot", map[string]interface{}{
		"allow_all_tools": "yes",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "allow_all_tools") {
		t.Fatalf("expected field path in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "boolean") {
		t.Fatalf("expected boolean type error, got %v", err)
	}
}

func TestCopilotConfigEnumValidation(t *testing.T) {
	err := ValidateAgentConfig("copilot", map[string]interface{}{
		"model": "unknown-model",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "model") {
		t.Fatalf("expected field path in error, got %v", err)
	}
}
