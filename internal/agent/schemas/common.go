package schemas

import (
	"fmt"
	"strings"
	"sync"

	"github.com/invopop/jsonschema"
)

type schemaProvider func() *jsonschema.Schema

var (
	registry = map[string]schemaProvider{
		"codex":   CodexSchema,
		"copilot": CopilotSchema,
	}
	registryMu sync.RWMutex
	cache      = map[string]*jsonschema.Schema{}
	cacheMu    sync.RWMutex
)

func SchemaFor(cliType string) (*jsonschema.Schema, error) {
	cliType = strings.ToLower(strings.TrimSpace(cliType))
	if cliType == "" {
		return nil, fmt.Errorf("cli type is required for schema lookup")
	}

	cacheMu.RLock()
	if schema, ok := cache[cliType]; ok {
		cacheMu.RUnlock()
		return schema, nil
	}
	cacheMu.RUnlock()

	registryMu.RLock()
	provider, ok := registry[cliType]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown cli type %q", cliType)
	}

	schema := provider()
	cacheMu.Lock()
	cache[cliType] = schema
	cacheMu.Unlock()
	return schema, nil
}

func RegisterSchema(cliType string, provider schemaProvider) error {
	cliType = strings.ToLower(strings.TrimSpace(cliType))
	if cliType == "" {
		return fmt.Errorf("cli type is required for schema registration")
	}
	if provider == nil {
		return fmt.Errorf("schema provider is required")
	}

	registryMu.Lock()
	registry[cliType] = provider
	registryMu.Unlock()

	cacheMu.Lock()
	delete(cache, cliType)
	cacheMu.Unlock()
	return nil
}

func ClearSchemaCache() {
	cacheMu.Lock()
	cache = map[string]*jsonschema.Schema{}
	cacheMu.Unlock()
}
