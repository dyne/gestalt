package schemas

import (
	"encoding/json"
	"testing"
)

type sampleSchemaConfig struct {
	Name  string `json:"name" jsonschema:"required"`
	Count int    `json:"count,omitempty"`
}

func TestGenerateSchemaAddsVersionAndRequired(t *testing.T) {
	schema := GenerateSchema(sampleSchemaConfig{})
	if schema == nil {
		t.Fatal("expected schema")
	}
	if schema.Version == "" {
		t.Fatalf("expected schema version, got %q", schema.Version)
	}

	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	required, ok := payload["required"].([]any)
	if !ok || len(required) != 1 || required[0] != "name" {
		t.Fatalf("expected required [name], got %#v", payload["required"])
	}

	if additional, ok := payload["additionalProperties"].(bool); !ok || additional {
		t.Fatalf("expected additionalProperties false, got %#v", payload["additionalProperties"])
	}
}
