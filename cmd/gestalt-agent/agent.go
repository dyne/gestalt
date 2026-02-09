package main

import (
	"fmt"
	"io/fs"
	"strings"

	agentpkg "gestalt/internal/agent"
	"gestalt/internal/app"
	"gestalt/internal/skill"
)

func loadAgents(configFS fs.FS, root string) (map[string]agentpkg.Agent, error) {
	agents, _, err := loadAgentsAndSkills(configFS, root)
	return agents, err
}

func loadAgentsAndSkills(configFS fs.FS, root string) (map[string]agentpkg.Agent, map[string]*skill.Skill, error) {
	skills, err := app.LoadSkills(nil, configFS, root)
	if err != nil {
		return nil, nil, err
	}
	agents, err := app.LoadAgents(nil, configFS, root, app.BuildSkillIndex(skills))
	if err != nil {
		return nil, nil, err
	}
	return agents, skills, nil
}

func selectAgent(agents map[string]agentpkg.Agent, agentID string) (agentpkg.Agent, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return agentpkg.Agent{}, fmt.Errorf("agent id is required")
	}
	profile, ok := agents[agentID]
	if !ok {
		return agentpkg.Agent{}, fmt.Errorf("agent %q not found", agentID)
	}
	if !strings.EqualFold(profile.CLIType, "codex") {
		return agentpkg.Agent{}, fmt.Errorf("agent %q cli_type %q is not supported", agentID, profile.CLIType)
	}
	return profile, nil
}
