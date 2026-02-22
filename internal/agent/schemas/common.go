package schemas

import (
	"fmt"
	"strings"

	"github.com/invopop/jsonschema"

	internalschema "gestalt/internal/schema"
)

func init() {
	_ = RegisterSchema("codex", CodexSchema)
	_ = RegisterSchema("copilot", CopilotSchema)
}

func SchemaFor(cliType string) (*jsonschema.Schema, error) {
	cliType = strings.ToLower(strings.TrimSpace(cliType))
	if cliType == "" {
		return nil, fmt.Errorf("cli type is required for schema lookup")
	}
	return internalschema.Resolve(cliType)
}

func RegisterSchema(cliType string, provider func() *jsonschema.Schema) error {
	cliType = strings.ToLower(strings.TrimSpace(cliType))
	if cliType == "" {
		return fmt.Errorf("cli type is required for schema registration")
	}
	if provider == nil {
		return fmt.Errorf("schema provider is required")
	}
	return internalschema.Register(cliType, provider)
}

func ClearSchemaCache() {
	internalschema.ClearCache()
}
