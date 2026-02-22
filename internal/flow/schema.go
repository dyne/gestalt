package flow

import (
	"github.com/invopop/jsonschema"

	internalschema "gestalt/internal/schema"
)

const (
	SchemaFlowFile   = "flow-file"
	SchemaFlowBundle = "flow-bundle"
)

func init() {
	_ = internalschema.Register(SchemaFlowFile, flowFileSchema)
	_ = internalschema.Register(SchemaFlowBundle, flowBundleSchema)
}

func flowFileSchema() *jsonschema.Schema {
	return generateSchema(FlowFile{})
}

func flowBundleSchema() *jsonschema.Schema {
	return generateSchema(FlowBundle{})
}

func generateSchema(value any) *jsonschema.Schema {
	reflector := &jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
		ExpandedStruct:            true,
	}
	s := reflector.Reflect(value)
	if s.Version == "" {
		s.Version = jsonschema.Version
	}
	return s
}
