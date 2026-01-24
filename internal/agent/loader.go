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
	"gestalt/internal/fsutil"
	"gestalt/internal/logging"
	"gestalt/internal/prompt"
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

	entries, err := fsutil.ReadDirOrEmpty(agentFS, dir)
	if err != nil {
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
		if l.Logger != nil && l.Logger.Enabled(logging.LevelDebug) && len(agent.CLIConfig) > 0 {
			shell := strings.TrimSpace(agent.Shell)
			if shell != "" {
				l.Logger.Debug("agent shell command rendered", map[string]string{
					"agent_id": agentID,
					"cli_type": agent.CLIType,
					"path":     filePath,
					"shell":    shell,
				})
			}
		}
		if _, exists := agents[agentID]; exists {
			l.warnDuplicateID(agentID, filePath)
			continue
		}
		normalizedName := normalizeAgentName(agent.Name)
		if prior, ok := agentNames[normalizedName]; ok {
			l.warnDuplicateName(agent.Name, prior, filePath)
			continue
		}
		validatePromptNames(l.Logger, agentFS, agentID, agent, promptsDir)
		agent.Skills = resolveSkills(l.Logger, agentID, agent.Skills, skillIndex)
		agents[agentID] = agent
		agentNames[normalizedName] = filePath
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
		emitConfigValidationError(filePath, err)
		return nil, fmt.Errorf("read agent file %s: %w", filePath, err)
	}
	agent, err := loadAgentFromBytes(filePath, data)
	if err != nil {
		emitConfigValidationError(filePath, err)
		return nil, err
	}
	return &agent, nil
}

func readAgentFile(agentFS fs.FS, filePath string) (Agent, error) {
	data, err := fs.ReadFile(agentFS, filePath)
	if err != nil {
		emitConfigValidationError(filePath, err)
		return Agent{}, fmt.Errorf("read agent file %s: %w", filePath, err)
	}
	agent, err := loadAgentFromBytes(filePath, data)
	if err != nil {
		emitConfigValidationError(filePath, err)
		return Agent{}, err
	}
	return agent, nil
}

func emitConfigValidationErrorWithMessage(filePath string, message string) {
	config.Bus().Publish(event.NewConfigEvent("agent", filePath, "validation_error", message))
}

func emitConfigValidationError(filePath string, err error) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	emitConfigValidationErrorWithMessage(filePath, message)
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
		if _, err := prompt.ResolvePromptPath(agentFS, promptsDir, promptName); err != nil {
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

func normalizeAgentName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
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
	fsys, cleaned, err := fsutil.NormalizeFSPaths(agentFS, "agent loader", dir, promptsDir)
	if err != nil {
		return nil, "", "", err
	}
	if len(cleaned) < 2 {
		return nil, "", "", fmt.Errorf("agent loader paths missing")
	}
	return fsys, cleaned[0], cleaned[1], nil
}
