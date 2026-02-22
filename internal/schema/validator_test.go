package schema

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/invopop/jsonschema"
)

func TestValidateObjectRequiredField(t *testing.T) {
	s := &jsonschema.Schema{
		Type:     "object",
		Required: []string{"name"},
	}

	err := ValidateObject(s, map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("expected path in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "missing required field") {
		t.Fatalf("expected required field message, got %v", err)
	}
}

func TestValidateObjectUnknownField(t *testing.T) {
	var additional jsonschema.Schema
	if err := json.Unmarshal([]byte("false"), &additional); err != nil {
		t.Fatalf("unmarshal false schema: %v", err)
	}

	s := &jsonschema.Schema{
		Type: "object",
		AdditionalProperties: &additional,
	}

	err := ValidateObject(s, map[string]any{"extra": true})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestValidateValueAnyOf(t *testing.T) {
	s := &jsonschema.Schema{
		AnyOf: []*jsonschema.Schema{
			{Type: "string"},
			{Type: "integer"},
		},
	}

	if err := ValidateValue(s, "ok"); err != nil {
		t.Fatalf("expected string to match anyOf: %v", err)
	}
	if err := ValidateValue(s, 12); err != nil {
		t.Fatalf("expected int to match anyOf: %v", err)
	}
	if err := ValidateValue(s, true); err == nil {
		t.Fatal("expected bool to fail anyOf")
	}
}

func TestValidateValueOneOf(t *testing.T) {
	s := &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{Type: "string"},
			{Type: "integer"},
		},
	}

	if err := ValidateValue(s, "ok"); err != nil {
		t.Fatalf("expected string to match oneOf: %v", err)
	}
	if err := ValidateValue(s, 12); err != nil {
		t.Fatalf("expected int to match oneOf: %v", err)
	}
	if err := ValidateValue(s, true); err == nil {
		t.Fatal("expected bool to fail oneOf")
	}
}

func TestValidateValueEnum(t *testing.T) {
	s := &jsonschema.Schema{
		Type: "string",
		Enum: []any{"a", "b"},
	}

	if err := ValidateValue(s, "a"); err != nil {
		t.Fatalf("expected enum value to pass: %v", err)
	}
	if err := ValidateValue(s, "x"); err == nil {
		t.Fatal("expected enum mismatch")
	}
}
