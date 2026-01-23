package agent

import (
	"sync"
)

type AgentCache struct {
	mu     sync.RWMutex
	agents map[string]*Agent
}

func NewAgentCache(initial map[string]Agent) *AgentCache {
	cache := &AgentCache{
		agents: make(map[string]*Agent),
	}
	for id, profile := range initial {
		agentCopy := profile
		if agentCopy.ConfigHash == "" {
			agentCopy.ConfigHash = ComputeConfigHash(&agentCopy)
		}
		cache.agents[id] = &agentCopy
	}
	return cache
}

func (c *AgentCache) LoadOrReload(agentID string, agentsDir string) (*Agent, bool, error) {
	if c == nil {
		return nil, false, nil
	}
	latest, err := LoadAgentByID(agentID, agentsDir)
	if err != nil {
		return nil, false, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	cached, ok := c.agents[agentID]
	if ok && cached != nil && cached.ConfigHash == latest.ConfigHash {
		return cached, false, nil
	}
	reloaded := ok && cached != nil
	c.agents[agentID] = latest
	return latest, reloaded, nil
}

func (c *AgentCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.agents = make(map[string]*Agent)
	c.mu.Unlock()
}
