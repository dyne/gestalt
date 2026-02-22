package schema

import (
	"fmt"
	"strings"
	"sync"

	"github.com/invopop/jsonschema"
)

// Provider builds and returns a schema for a registered key.
type Provider func() *jsonschema.Schema

var (
	registryMu sync.RWMutex
	registry   = map[string]Provider{}

	cacheMu sync.RWMutex
	cache   = map[string]*jsonschema.Schema{}
)

// Register installs or replaces the provider under the provided key.
func Register(name string, provider Provider) error {
	name = normalizeName(name)
	if name == "" {
		return fmt.Errorf("schema name is required for registration")
	}
	if provider == nil {
		return fmt.Errorf("schema provider is required")
	}

	registryMu.Lock()
	registry[name] = provider
	registryMu.Unlock()

	cacheMu.Lock()
	delete(cache, name)
	cacheMu.Unlock()
	return nil
}

// Resolve finds the schema provider for name and returns its cached schema.
func Resolve(name string) (*jsonschema.Schema, error) {
	name = normalizeName(name)
	if name == "" {
		return nil, fmt.Errorf("schema name is required for lookup")
	}

	cacheMu.RLock()
	if schema, ok := cache[name]; ok {
		cacheMu.RUnlock()
		return schema, nil
	}
	cacheMu.RUnlock()

	registryMu.RLock()
	provider, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown schema %q", name)
	}

	s := provider()
	cacheMu.Lock()
	cache[name] = s
	cacheMu.Unlock()
	return s, nil
}

// ClearCache clears all cached schema instances.
func ClearCache() {
	cacheMu.Lock()
	cache = map[string]*jsonschema.Schema{}
	cacheMu.Unlock()
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
