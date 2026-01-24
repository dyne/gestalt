package agent

import (
	"errors"
	"os"
	"strings"
	"sync"
)

// Registry manages agent profiles with optional reload support.
type Registry struct {
	mu        sync.RWMutex
	agents    map[string]Agent
	cache     *AgentCache
	agentsDir string
}

// RegistryOptions configures a Registry.
type RegistryOptions struct {
	Agents    map[string]Agent
	AgentsDir string
}

// NewRegistry builds a registry backed by an AgentCache when available.
func NewRegistry(options RegistryOptions) *Registry {
	agents := make(map[string]Agent)
	for id, profile := range options.Agents {
		agents[id] = profile
	}
	cache := NewAgentCache(agents)
	return &Registry{
		agents:    agents,
		cache:     cache,
		agentsDir: strings.TrimSpace(options.AgentsDir),
	}
}

// Get returns a copy of an agent profile by ID.
func (r *Registry) Get(agentID string) (Agent, bool) {
	if r == nil {
		return Agent{}, false
	}
	r.mu.RLock()
	profile, ok := r.agents[agentID]
	r.mu.RUnlock()
	return profile, ok
}

// Snapshot returns a copy of the registry contents.
func (r *Registry) Snapshot() map[string]Agent {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	agents := make(map[string]Agent, len(r.agents))
	for id, profile := range r.agents {
		agents[id] = profile
	}
	r.mu.RUnlock()
	return agents
}

// LoadOrReload returns the current profile, reloading from disk if needed.
func (r *Registry) LoadOrReload(agentID string) (*Agent, bool, error) {
	if r == nil {
		return nil, false, errors.New("agent registry is nil")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, false, errors.New("agent id is required")
	}
	if r.cache == nil {
		profile, ok := r.Get(agentID)
		if !ok {
			return nil, false, os.ErrNotExist
		}
		profileCopy := profile
		return &profileCopy, false, nil
	}

	profile, reloaded, err := r.cache.LoadOrReload(agentID, r.agentsDir)
	if err != nil {
		return nil, false, err
	}
	if profile == nil {
		return nil, false, os.ErrNotExist
	}
	r.mu.Lock()
	r.agents[agentID] = *profile
	r.mu.Unlock()
	return profile, reloaded, nil
}
