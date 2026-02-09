package main

import (
	"fmt"
	"io/fs"
	"path"

	"gestalt/internal/agent"
	"gestalt/internal/ports"
	"gestalt/internal/prompt"
	"gestalt/internal/skill"
)

func renderDeveloperPrompt(profile agent.Agent, skills map[string]*skill.Skill, promptFS fs.FS, root string, resolver ports.PortResolver) (string, error) {
	promptDir := path.Join(root, "prompts")
	parser := prompt.NewParser(promptFS, promptDir, ".", resolver)
	resolvedSkills := resolveAgentSkills(profile.Skills, skills)
	result, err := agent.BuildDeveloperInstructions(profile.Prompts, resolvedSkills, parser.RenderWithContext, "")
	if err != nil {
		return "", fmt.Errorf("render prompt from %s: %w", promptDir, err)
	}
	return result.Instructions, nil
}

func resolveAgentSkills(names []string, skills map[string]*skill.Skill) []*skill.Skill {
	if len(names) == 0 || len(skills) == 0 {
		return nil
	}
	resolved := make([]*skill.Skill, 0, len(names))
	for _, name := range names {
		if entry, ok := skills[name]; ok {
			resolved = append(resolved, entry)
		}
	}
	return resolved
}
