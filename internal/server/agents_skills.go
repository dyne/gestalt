package server

import (
	"io/fs"
	"path"

	"gestalt/internal/agent"
	"gestalt/internal/logging"
	"gestalt/internal/skill"
)

func LoadAgents(logger *logging.Logger, configFS fs.FS, configRoot string, skillIndex map[string]struct{}) (map[string]agent.Agent, error) {
	loader := agent.Loader{Logger: logger}
	return loader.Load(configFS, path.Join(configRoot, "agents"), path.Join(configRoot, "prompts"), skillIndex)
}

func LoadSkills(logger *logging.Logger, configFS fs.FS, configRoot string) (map[string]*skill.Skill, error) {
	loader := skill.Loader{Logger: logger}
	return loader.Load(configFS, path.Join(configRoot, "skills"))
}

func BuildSkillIndex(skills map[string]*skill.Skill) map[string]struct{} {
	if len(skills) == 0 {
		return map[string]struct{}{}
	}
	index := make(map[string]struct{}, len(skills))
	for name := range skills {
		index[name] = struct{}{}
	}
	return index
}
