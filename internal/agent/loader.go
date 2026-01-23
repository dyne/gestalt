package agent

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gestalt/internal/config"
	"gestalt/internal/event"
	"gestalt/internal/logging"
)

// Loader reads agent profiles from TOML files.
type Loader struct {
	Logger *logging.Logger
}

// Load scans dir for *.toml files and returns a map keyed by agent ID.
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
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".toml" {
			if ext == ".json" {
				agentID := strings.TrimSuffix(name, ext)
				filePath := path.Join(dir, name)
				l.warnLoadError(agentID, filePath, fmt.Errorf("only TOML agent configs are supported"))
			}
			continue
		}
		agentID := strings.TrimSuffix(name, ".toml")
		filePath := path.Join(dir, name)
		agent, err := readAgentFile(agentFS, filePath)
		if err != nil {
			l.warnLoadError(agentID, filePath, err)
			continue
		}
		if _, exists := agents[agentID]; exists {
			l.warnDuplicateID(agentID, filePath)
			continue
		}
		if prior, ok := agentNames[agent.Name]; ok {
			l.warnDuplicateName(agent.Name, prior, filePath)
			continue
		}
		validatePromptNames(l.Logger, agentFS, agentID, agent, promptsDir)
		agent.Skills = resolveSkills(l.Logger, agentID, agent.Skills, skillIndex)
		agents[agentID] = agent
		agentNames[agent.Name] = filePath
	}

	return agents, nil
}

// LoadAgentByID loads a single agent config by ID from the filesystem.
func LoadAgentByID(agentID string, agentsDir string) (*Agent, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	agentsDir = strings.TrimSpace(agentsDir)
	if agentsDir == "" {
		agentsDir = filepath.Join("config", "agents")
	}
	if strings.HasSuffix(strings.ToLower(agentID), ".toml") {
		agentID = strings.TrimSuffix(agentID, filepath.Ext(agentID))
	}
	filePath := filepath.Join(agentsDir, agentID+".toml")
	data, err := os.ReadFile(filePath)
	if err != nil {
		emitConfigValidationError(filePath)
		return nil, fmt.Errorf("read agent file %s: %w", filePath, err)
	}
	agent, err := loadAgentFromBytes(filePath, data)
	if err != nil {
		emitConfigValidationError(filePath)
		return nil, err
	}
	return &agent, nil
}

func readAgentFile(agentFS fs.FS, filePath string) (Agent, error) {
	data, err := fs.ReadFile(agentFS, filePath)
	if err != nil {
		emitConfigValidationError(filePath)
		return Agent{}, fmt.Errorf("read agent file %s: %w", filePath, err)
	}
	agent, err := loadAgentFromBytes(filePath, data)
	if err != nil {
		emitConfigValidationError(filePath)
		return Agent{}, err
	}
	return agent, nil
}

func emitConfigValidationError(filePath string) {
	config.Bus().Publish(event.NewConfigEvent("agent", filePath, "validation_error"))
}

func (l Loader) warnLoadError(agentID, path string, err error) {
	if l.Logger == nil {
		return
	}
	l.Logger.Warn("agent load failed", map[string]string{
		"agent_id": agentID,
		"path":     path,
		"error":    err.Error(),
	})
}

func (l Loader) warnDuplicateID(agentID, path string) {
	if l.Logger == nil {
		return
	}
	l.Logger.Warn("agent duplicate id ignored", map[string]string{
		"agent_id": agentID,
		"path":     path,
	})
}

func (l Loader) warnDuplicateName(name, firstPath, secondPath string) {
	if l.Logger == nil {
		return
	}
	l.Logger.Warn("agent duplicate name ignored", map[string]string{
		"name":     name,
		"path":     secondPath,
		"existing": firstPath,
	})
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
		if _, err := resolvePromptPath(agentFS, promptsDir, promptName); err != nil {
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

func resolvePromptPath(agentFS fs.FS, promptsDir, promptName string) (string, error) {
	extensions := []string{".tmpl", ".md", ".txt"}
	for _, ext := range extensions {
		promptPath := path.Join(promptsDir, promptName+ext)
		if _, err := fs.Stat(agentFS, promptPath); err == nil {
			return promptPath, nil
		}
	}
	return "", fmt.Errorf("prompt %q not found", promptName)
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
