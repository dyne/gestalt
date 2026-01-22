package agent

import (
	"strings"
	"testing"

	"gestalt/internal/agent/schemas"
)

type requiredConfig struct {
	Name   string        `json:"name" jsonschema:"required"`
	Nested *nestedConfig `json:"nested,omitempty"`
}

type nestedConfig struct {
	Count int `json:"count,omitempty"`
}

func TestValidateAgentConfigTypeMismatch(t *testing.T) {
	err := ValidateAgentConfig("codex", map[string]interface{}{
		"model": 123,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "model") {
		t.Fatalf("expected error to mention field, got %v", err)
	}
	if !strings.Contains(err.Error(), "string") {
		t.Fatalf("expected error to mention expected type, got %v", err)
	}
}

func TestValidateAgentConfigUnknownField(t *testing.T) {
	err := ValidateAgentConfig("codex", map[string]interface{}{
		"unknown_field": "value",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown_field") {
		t.Fatalf("expected error to mention field, got %v", err)
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestValidateAgentConfigRequiredField(t *testing.T) {
	schema := schemas.GenerateSchema(requiredConfig{})
	err := validateConfigWithSchema(schema, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("expected missing field path, got %v", err)
	}
	if !strings.Contains(err.Error(), "missing required") {
		t.Fatalf("expected missing required field message, got %v", err)
	}
}

func TestValidateAgentConfigNestedTypeMismatch(t *testing.T) {
	schema := schemas.GenerateSchema(requiredConfig{})
	err := validateConfigWithSchema(schema, map[string]interface{}{
		"name": "ok",
		"nested": map[string]interface{}{
			"count": "bad",
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "nested.count") {
		t.Fatalf("expected nested path, got %v", err)
	}
	if !strings.Contains(err.Error(), "integer") {
		t.Fatalf("expected integer type error, got %v", err)
	}
}
