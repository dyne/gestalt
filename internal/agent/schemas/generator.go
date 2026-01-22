package schemas

import "github.com/invopop/jsonschema"

func newReflector() *jsonschema.Reflector {
	return &jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
		ExpandedStruct:            true,
	}
}

func GenerateSchema(value any) *jsonschema.Schema {
	reflector := newReflector()
	schema := reflector.Reflect(value)
	if schema.Version == "" {
		schema.Version = jsonschema.Version
	}
	return schema
}
