package flow

import (
	"strings"
	"testing"

	internalschema "gestalt/internal/schema"
)

func TestFlowFileSchemaRejectsMissingRequired(t *testing.T) {
	s, err := internalschema.Resolve(SchemaFlowFile)
	if err != nil {
		t.Fatalf("resolve flow-file schema: %v", err)
	}

	err = internalschema.ValidateObject(s, map[string]any{
		"event_type": "git.push",
	})
	if err == nil {
		t.Fatal("expected required field error")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Fatalf("expected id path in error, got %v", err)
	}
}

func TestFlowFileSchemaRejectsUnknownField(t *testing.T) {
	s, err := internalschema.Resolve(SchemaFlowFile)
	if err != nil {
		t.Fatalf("resolve flow-file schema: %v", err)
	}

	err = internalschema.ValidateObject(s, map[string]any{
		"id":         "flow-1",
		"event_type": "git.push",
		"unknown":    true,
	})
	if err == nil {
		t.Fatal("expected unknown field error")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field message, got %v", err)
	}
}

func TestFlowBundleSchemaResolves(t *testing.T) {
	if _, err := internalschema.Resolve(SchemaFlowBundle); err != nil {
		t.Fatalf("resolve flow-bundle schema: %v", err)
	}
}
