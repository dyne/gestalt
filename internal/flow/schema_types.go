package flow

// FlowBinding is the persisted binding DTO for a single flow file.
type FlowBinding struct {
	ActivityID string         `json:"activity_id" jsonschema:"required"`
	Config     map[string]any `json:"config,omitempty"`
}

// FlowFile is the persisted DTO for one trigger flow.
type FlowFile struct {
	ID        string            `json:"id" jsonschema:"required"`
	Label     string            `json:"label,omitempty"`
	EventType string            `json:"event_type" jsonschema:"required"`
	Where     map[string]string `json:"where,omitempty"`
	Bindings  []FlowBinding     `json:"bindings,omitempty"`
}

// FlowBundle is the import/export DTO for a set of flow files.
type FlowBundle struct {
	Version int        `json:"version" jsonschema:"required"`
	Flows   []FlowFile `json:"flows" jsonschema:"required"`
}
