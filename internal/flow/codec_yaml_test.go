package flow

import (
	"errors"
	"strings"
	"testing"

	internalschema "gestalt/internal/schema"
)

func TestDecodeFlowBundleYAMLUnknownField(t *testing.T) {
	_, err := DecodeFlowBundleYAML([]byte(`
version: 1
flows:
  - id: trigger-one
    event_type: file_changed
    unknown: true
    bindings: []
`), ActivityCatalog())
	if err == nil {
		t.Fatal("expected schema error")
	}
	var schemaErr *internalschema.ValidationError
	if !errors.As(err, &schemaErr) {
		t.Fatalf("expected schema validation error, got %T (%v)", err, err)
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestDecodeFlowBundleYAMLBadType(t *testing.T) {
	_, err := DecodeFlowBundleYAML([]byte(`
version: "one"
flows: []
`), ActivityCatalog())
	if err == nil {
		t.Fatal("expected schema error")
	}
	var schemaErr *internalschema.ValidationError
	if !errors.As(err, &schemaErr) {
		t.Fatalf("expected schema validation error, got %T (%v)", err, err)
	}
	if !strings.Contains(err.Error(), "version") {
		t.Fatalf("expected version path in schema error, got %v", err)
	}
}

func TestDecodeFlowBundleYAMLMissingRequired(t *testing.T) {
	_, err := DecodeFlowBundleYAML([]byte(`
flows: []
`), ActivityCatalog())
	if err == nil {
		t.Fatal("expected schema error")
	}
	var schemaErr *internalschema.ValidationError
	if !errors.As(err, &schemaErr) {
		t.Fatalf("expected schema validation error, got %T (%v)", err, err)
	}
	if !strings.Contains(err.Error(), "version") {
		t.Fatalf("expected version path in schema error, got %v", err)
	}
}

func TestDecodeFlowBundleYAMLSemanticConflict(t *testing.T) {
	_, err := DecodeFlowBundleYAML([]byte(`
version: 1
flows:
  - id: duplicate
    label: first
    event_type: file_changed
    where: {}
    bindings:
      - activity_id: toast_notification
        config:
          level: info
          message_template: hi
  - id: duplicate
    label: second
    event_type: file_changed
    where: {}
    bindings: []
`), ActivityCatalog())
	if err == nil {
		t.Fatal("expected semantic validation error")
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected flow validation error, got %T (%v)", err, err)
	}
	if validationErr.Kind != ValidationConflict {
		t.Fatalf("expected conflict validation kind, got %q", validationErr.Kind)
	}
}

func TestDecodeFlowBundleYAMLSemanticActivityConfig(t *testing.T) {
	_, err := DecodeFlowBundleYAML([]byte(`
version: 1
flows:
  - id: trigger-one
    label: trigger
    event_type: file_changed
    where: {}
    bindings:
      - activity_id: toast_notification
        config:
          level: 5
`), ActivityCatalog())
	if err == nil {
		t.Fatal("expected semantic validation error")
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected flow validation error, got %T (%v)", err, err)
	}
	if validationErr.Kind != ValidationBadRequest {
		t.Fatalf("expected bad request validation kind, got %q", validationErr.Kind)
	}
}
