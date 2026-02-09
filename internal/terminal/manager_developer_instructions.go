package terminal

import (
	"strings"

	"gestalt/internal/agent"
	"gestalt/internal/prompt"
	"gestalt/internal/skill"
)

func (m *Manager) buildCodexDeveloperInstructions(profile *agent.Agent, sessionID string) (agent.DeveloperInstructions, error) {
	if profile == nil || !strings.EqualFold(strings.TrimSpace(profile.CLIType), "codex") {
		return agent.DeveloperInstructions{}, nil
	}
	var renderer agent.PromptRenderer
	if m.promptParser != nil {
		renderer = func(promptName string, ctx prompt.RenderContext) (*prompt.RenderResult, error) {
			return m.promptParser.RenderWithContext(promptName, ctx)
		}
	}
	skills := m.resolveAgentSkills(profile)
	return agent.BuildDeveloperInstructions(profile.Prompts, skills, renderer, sessionID)
}

func (m *Manager) resolveAgentSkills(profile *agent.Agent) []*skill.Skill {
	if profile == nil || len(profile.Skills) == 0 || len(m.skills) == 0 {
		return nil
	}
	agentSkills := make([]*skill.Skill, 0, len(profile.Skills))
	for _, skillName := range profile.Skills {
		if skillEntry, ok := m.skills[skillName]; ok {
			agentSkills = append(agentSkills, skillEntry)
		}
	}
	return agentSkills
}
