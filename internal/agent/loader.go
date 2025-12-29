package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Loader reads agent profiles from JSON files.
type Loader struct{}

// Load scans dir for *.json files and returns a map keyed by agent ID.
func (l Loader) Load(dir string) (map[string]Agent, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Agent{}, nil
		}
		return nil, fmt.Errorf("read agents dir: %w", err)
	}

	agents := make(map[string]Agent)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}
		agentID := strings.TrimSuffix(name, ".json")
		path := filepath.Join(dir, name)
		agent, err := readAgentFile(path)
		if err != nil {
			return nil, err
		}
		if _, exists := agents[agentID]; exists {
			return nil, fmt.Errorf("duplicate agent id %q", agentID)
		}
		agents[agentID] = agent
	}

	return agents, nil
}

func readAgentFile(path string) (Agent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Agent{}, fmt.Errorf("read agent file %s: %w", path, err)
	}
	var agent Agent
	if err := json.Unmarshal(data, &agent); err != nil {
		return Agent{}, fmt.Errorf("parse agent file %s: %w", path, err)
	}
	if err := agent.Validate(); err != nil {
		return Agent{}, fmt.Errorf("validate agent file %s: %w", path, err)
	}
	return agent, nil
}
