package agent

import (
	"strings"
	"testing"
)

func TestCodexConfigValid(t *testing.T) {
	config := map[string]interface{}{
		"model":           "o3",
		"review_model":    "gpt-5.1-codex-max",
		"approval_policy": "never",
		"sandbox_mode":    "read-only",
		"notify":          "terminal",
		"tui": map[string]interface{}{
			"notifications":          true,
			"scroll_events_per_tick": 2,
			"scroll_wheel_lines":     3,
		},
		"model_provider":                 "openai",
		"model_providers":                map[string]interface{}{"openai": map[string]interface{}{"base_url": "https://api.example.com"}},
		"mcp_servers":                    map[string]interface{}{"local": map[string]interface{}{"command": "mcp"}},
		"ghost_snapshot":                 map[string]interface{}{"ignore_large_untracked_files": 2048},
		"features":                       map[string]interface{}{"experimental_ui": true},
		"check_for_update_on_startup":    false,
		"windows_wsl_setup_acknowledged": true,
	}

	if err := ValidateAgentConfig("codex", config); err != nil {
		t.Fatalf("expected config to be valid, got %v", err)
	}
}

func TestCodexConfigUnknownField(t *testing.T) {
	err := ValidateAgentConfig("codex", map[string]interface{}{
		"unknown_field": true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown_field") {
		t.Fatalf("expected unknown field in error, got %v", err)
	}
}

func TestCodexConfigTypeMismatch(t *testing.T) {
	err := ValidateAgentConfig("codex", map[string]interface{}{
		"model_context_window": "large",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "model_context_window") {
		t.Fatalf("expected field path in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "integer") {
		t.Fatalf("expected integer type error, got %v", err)
	}
}

func TestCodexConfigNotificationsVariant(t *testing.T) {
	if err := ValidateAgentConfig("codex", map[string]interface{}{
		"tui": map[string]interface{}{
			"notifications": []interface{}{"foo"},
		},
	}); err != nil {
		t.Fatalf("expected notifications array to be valid, got %v", err)
	}

	err := ValidateAgentConfig("codex", map[string]interface{}{
		"tui": map[string]interface{}{
			"notifications": "nope",
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "tui.notifications") {
		t.Fatalf("expected field path in error, got %v", err)
	}
}

func TestCodexConfigFeatureValueType(t *testing.T) {
	err := ValidateAgentConfig("codex", map[string]interface{}{
		"features": map[string]interface{}{"flag": "true"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "features.flag") {
		t.Fatalf("expected nested field path, got %v", err)
	}
	if !strings.Contains(err.Error(), "boolean") {
		t.Fatalf("expected boolean type error, got %v", err)
	}
}
