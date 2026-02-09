package agent

import (
	"errors"
	"strings"

	"gestalt/internal/prompt"
	"gestalt/internal/skill"
)

type PromptRenderer func(promptName string, ctx prompt.RenderContext) (*prompt.RenderResult, error)

type DeveloperInstructions struct {
	Instructions string
	PromptFiles  []string
}

const developerInstructionSeparator = "\n\n"

func BuildDeveloperInstructions(promptNames []string, skills []*skill.Skill, renderer PromptRenderer, sessionID string) (DeveloperInstructions, error) {
	var result DeveloperInstructions
	promptText, promptFiles, err := renderPromptInstructions(promptNames, renderer, sessionID)
	if err != nil {
		return result, err
	}
	result.PromptFiles = promptFiles

	skillsXML := skill.GeneratePromptXML(skills)
	result.Instructions = composeDeveloperInstructions(skillsXML, promptText)
	return result, nil
}

func composeDeveloperInstructions(skillsXML, promptText string) string {
	switch {
	case skillsXML == "" && promptText == "":
		return ""
	case skillsXML == "":
		return promptText
	case promptText == "":
		return skillsXML
	default:
		return skillsXML + developerInstructionSeparator + promptText
	}
}

func renderPromptInstructions(promptNames []string, renderer PromptRenderer, sessionID string) (string, []string, error) {
	cleaned := make([]string, 0, len(promptNames))
	for _, name := range promptNames {
		name = strings.TrimSpace(name)
		if name != "" {
			cleaned = append(cleaned, name)
		}
	}
	if len(cleaned) == 0 {
		return "", nil, nil
	}
	if renderer == nil {
		return "", nil, errors.New("prompt renderer unavailable")
	}
	var builder strings.Builder
	files := make([]string, 0, len(cleaned))
	for i, name := range cleaned {
		result, err := renderer(name, prompt.RenderContext{SessionID: sessionID})
		if err != nil {
			return "", nil, err
		}
		if i > 0 {
			builder.WriteString(developerInstructionSeparator)
		}
		builder.Write(result.Content)
		if len(result.Files) > 0 {
			files = append(files, result.Files...)
		}
	}
	return builder.String(), files, nil
}
