package flow

import "github.com/invopop/jsonschema"

type FlowWhere map[string]string

func (FlowWhere) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:                 "object",
		AdditionalProperties: &jsonschema.Schema{Type: "string"},
	}
}

type FlowBindingConfig map[string]any

func (FlowBindingConfig) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:                 "object",
		AdditionalProperties: &jsonschema.Schema{},
	}
}

// FlowBinding is the persisted binding DTO for a single flow file.
type FlowBinding struct {
	ActivityID string            `json:"activity_id" yaml:"activity_id" jsonschema:"required"`
	Config     FlowBindingConfig `json:"config,omitempty" yaml:"config,omitempty"`
}

// FlowFile is the persisted DTO for one trigger flow.
type FlowFile struct {
	ID        string        `json:"id" yaml:"id" jsonschema:"required"`
	Label     string        `json:"label,omitempty" yaml:"label,omitempty"`
	EventType string        `json:"event_type" yaml:"event_type" jsonschema:"required"`
	Where     FlowWhere     `json:"where,omitempty" yaml:"where,omitempty"`
	Bindings  []FlowBinding `json:"bindings,omitempty" yaml:"bindings,omitempty"`
}

// FlowBundle is the import/export DTO for a set of flow files.
type FlowBundle struct {
	Version int        `json:"version" yaml:"version" jsonschema:"required"`
	Flows   []FlowFile `json:"flows" yaml:"flows" jsonschema:"required"`
}
