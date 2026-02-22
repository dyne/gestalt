package agent

import (
	"github.com/invopop/jsonschema"

	"gestalt/internal/agent/schemas"
	internalschema "gestalt/internal/schema"
)

// ValidationError preserves agent error compatibility while using shared schema validation.
type ValidationError = internalschema.ValidationError

func ValidateAgentConfig(cliType string, config map[string]interface{}) error {
	if len(config) == 0 {
		return nil
	}
	s, err := schemas.SchemaFor(cliType)
	if err != nil {
		return err
	}
	return validateConfigWithSchema(s, config)
}

func validateConfigWithSchema(s *jsonschema.Schema, config map[string]interface{}) error {
	if s == nil {
		return nil
	}
	return internalschema.ValidateObject(s, config)
}
