package main

import (
	"fmt"
	"io/fs"
	"strings"

	agentpkg "gestalt/internal/agent"
	"gestalt/internal/app"
)

func loadAgents(configFS fs.FS, root string) (map[string]agentpkg.Agent, error) {
	skills, err := app.LoadSkills(nil, configFS, root)
	if err != nil {
		return nil, err
	}
	return app.LoadAgents(nil, configFS, root, app.BuildSkillIndex(skills))
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
