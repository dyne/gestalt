package ports

import (
	"strings"
	"sync"
)

// PortResolver exposes read-only service-to-port lookups.
type PortResolver interface {
	Get(service string) (int, bool)
}

// PortRegistry stores runtime-allocated ports for known services.
// It is safe for concurrent access.
type PortRegistry struct {
	mutex sync.RWMutex
	ports map[string]int
}

// NewPortRegistry constructs an empty registry.
func NewPortRegistry() *PortRegistry {
	return &PortRegistry{
		ports: make(map[string]int),
	}
}

// Set records the port for a service. Invalid inputs are ignored.
func (registry *PortRegistry) Set(service string, port int) {
	normalizedService, ok := normalizeService(service)
	if !ok || port <= 0 {
		return
	}
	registry.mutex.Lock()
	registry.ports[normalizedService] = port
	registry.mutex.Unlock()
}

// Get returns the port for a service, if present.
func (registry *PortRegistry) Get(service string) (int, bool) {
	normalizedService, ok := normalizeService(service)
	if !ok {
		return 0, false
	}
	registry.mutex.RLock()
	port, found := registry.ports[normalizedService]
	registry.mutex.RUnlock()
	return port, found
}

func normalizeService(service string) (string, bool) {
	normalizedService := strings.ToLower(strings.TrimSpace(service))
	if normalizedService == "" {
		return "", false
	}
	return normalizedService, true
}
