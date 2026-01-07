package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gestalt/internal/logging"
)

// Loader reads agent profiles from JSON files.
type Loader struct {
	Logger *logging.Logger
}

// Load scans dir for *.json files and returns a map keyed by agent ID.
func (l Loader) Load(agentFS fs.FS, dir, promptsDir string, skillIndex map[string]struct{}) (map[string]Agent, error) {
	if strings.TrimSpace(promptsDir) == "" {
		promptsDir = filepath.Join("config", "prompts")
	}
	agentFS, dir, promptsDir, err := normalizeAgentPaths(agentFS, dir, promptsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]Agent{}, nil
		}
		return nil, err
	}

	entries, err := fs.ReadDir(agentFS, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]Agent{}, nil
		}
		return nil, fmt.Errorf("read agents dir: %w", err)
	}

	agents := make(map[string]Agent)
	agentNames := make(map[string]string)
	var duplicateName string
	var duplicateFirst string
	var duplicateSecond string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}
		agentID := strings.TrimSuffix(name, ".json")
		filePath := path.Join(dir, name)
		agent, err := readAgentFile(agentFS, filePath)
		if err != nil {
			return nil, err
		}
		if _, exists := agents[agentID]; exists {
			return nil, fmt.Errorf("duplicate agent id %q", agentID)
		}
		validatePromptNames(l.Logger, agentFS, agentID, agent, promptsDir)
		agent.Skills = resolveSkills(l.Logger, agentID, agent.Skills, skillIndex)
		agents[agentID] = agent
		if prior, ok := agentNames[agent.Name]; ok {
			if duplicateName == "" {
				duplicateName = agent.Name
				duplicateFirst = prior
				duplicateSecond = filePath
			}
		} else {
			agentNames[agent.Name] = filePath
		}
	}

	if duplicateName != "" {
		return nil, fmt.Errorf("duplicate agent name %q in files: %s, %s", duplicateName, duplicateFirst, duplicateSecond)
	}

	return agents, nil
}

func readAgentFile(agentFS fs.FS, filePath string) (Agent, error) {
	data, err := fs.ReadFile(agentFS, filePath)
	if err != nil {
		return Agent{}, fmt.Errorf("read agent file %s: %w", filePath, err)
	}
	var agent Agent
	if err := json.Unmarshal(data, &agent); err != nil {
		return Agent{}, fmt.Errorf("parse agent file %s: %w", filePath, err)
	}
	if err := agent.Validate(); err != nil {
		return Agent{}, fmt.Errorf("validate agent file %s: %w", filePath, err)
	}
	return agent, nil
}

func validatePromptNames(logger *logging.Logger, agentFS fs.FS, agentID string, agent Agent, promptsDir string) {
	if len(agent.Prompts) == 0 {
		return
	}
	for _, promptName := range agent.Prompts {
		promptName = strings.TrimSpace(promptName)
		if promptName == "" {
			continue
		}
		promptPath := path.Join(promptsDir, promptName+".txt")
		if _, err := fs.Stat(agentFS, promptPath); err != nil {
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

func normalizeAgentPaths(agentFS fs.FS, dir, promptsDir string) (fs.FS, string, string, error) {
	if agentFS != nil {
		cleanDir, err := cleanFSPath(dir)
		if err != nil {
			return nil, "", "", err
		}
		cleanPrompts, err := cleanFSPath(promptsDir)
		if err != nil {
			return nil, "", "", err
		}
		return agentFS, cleanDir, cleanPrompts, nil
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}
	absPrompts, err := filepath.Abs(promptsDir)
	if err != nil {
		absPrompts = promptsDir
	}

	volume := filepath.VolumeName(absDir)
	if promptsVolume := filepath.VolumeName(absPrompts); promptsVolume != volume {
		return nil, "", "", fmt.Errorf("agent loader paths span volumes: %q, %q", absDir, absPrompts)
	}

	root := string(os.PathSeparator)
	if volume != "" {
		root = volume + string(os.PathSeparator)
	}

	relDir := strings.TrimPrefix(absDir, root)
	relPrompts := strings.TrimPrefix(absPrompts, root)

	cleanDir, err := cleanFSPath(relDir)
	if err != nil {
		return nil, "", "", err
	}
	cleanPrompts, err := cleanFSPath(relPrompts)
	if err != nil {
		return nil, "", "", err
	}
	return os.DirFS(root), cleanDir, cleanPrompts, nil
}

func cleanFSPath(pathValue string) (string, error) {
	slashPath := filepath.ToSlash(pathValue)
	slashPath = strings.TrimPrefix(slashPath, "/")
	if slashPath == "" {
		return ".", nil
	}
	cleaned := path.Clean(slashPath)
	if cleaned == "." {
		return ".", nil
	}
	if !fs.ValidPath(cleaned) {
		return "", fmt.Errorf("invalid fs path: %q", pathValue)
	}
	return cleaned, nil
}
