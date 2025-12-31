package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gestalt/internal/logging"
)

// Loader reads agent profiles from JSON files.
type Loader struct {
	Logger *logging.Logger
}

// Load scans dir for *.json files and returns a map keyed by agent ID.
func (l Loader) Load(dir, promptsDir string, skillIndex map[string]struct{}) (map[string]Agent, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Agent{}, nil
		}
		return nil, fmt.Errorf("read agents dir: %w", err)
	}

	agents := make(map[string]Agent)
	if strings.TrimSpace(promptsDir) == "" {
		promptsDir = filepath.Join("config", "prompts")
	}
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
		validatePromptNames(l.Logger, agentID, agent, promptsDir)
		agent.Skills = resolveSkills(l.Logger, agentID, agent.Skills, skillIndex)
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

func validatePromptNames(logger *logging.Logger, agentID string, agent Agent, promptsDir string) {
	if len(agent.Prompts) == 0 {
		return
	}
	for _, promptName := range agent.Prompts {
		promptName = strings.TrimSpace(promptName)
		if promptName == "" {
			continue
		}
		promptPath := filepath.Join(promptsDir, promptName+".txt")
		if _, err := os.Stat(promptPath); err != nil {
			if logger != nil {
				logger.Warn("agent prompt file missing", map[string]string{
					"agent_id": agentID,
					"prompt":   promptName,
					"error":    err.Error(),
				})
			}
		}
	}
}

func resolveSkills(logger *logging.Logger, agentID string, skills []string, skillIndex map[string]struct{}) []string {
	if len(skills) == 0 {
		return nil
	}

	cleaned := make([]string, 0, len(skills))
	seen := make(map[string]struct{}, len(skills))
	for _, skillName := range skills {
		skillName = strings.TrimSpace(skillName)
		if skillName == "" {
			continue
		}
		if _, exists := seen[skillName]; exists {
			continue
		}
		if skillIndex != nil {
			if _, ok := skillIndex[skillName]; !ok {
				if logger != nil {
					logger.Warn("agent skill missing", map[string]string{
						"agent_id": agentID,
						"skill":    skillName,
					})
				}
				continue
			}
		}
		cleaned = append(cleaned, skillName)
		seen[skillName] = struct{}{}
	}

	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}
