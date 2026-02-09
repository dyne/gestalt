package agent

import (
	"fmt"
	"testing"

	"gestalt/internal/prompt"
	"gestalt/internal/skill"
)

func TestBuildDeveloperInstructionsPromptOnly(t *testing.T) {
	renderer := stubPromptRenderer(map[string]*prompt.RenderResult{
		"first":  {Content: []byte("hello"), Files: []string{"first.txt"}},
		"second": {Content: []byte("world"), Files: []string{"second.txt"}},
	})

	result, err := BuildDeveloperInstructions([]string{"first", "second"}, nil, renderer, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Instructions != "hello\n\nworld" {
		t.Fatalf("unexpected instructions: %q", result.Instructions)
	}
	if len(result.PromptFiles) != 2 || result.PromptFiles[0] != "first.txt" || result.PromptFiles[1] != "second.txt" {
		t.Fatalf("unexpected prompt files: %v", result.PromptFiles)
	}
}

func TestBuildDeveloperInstructionsSkillsOnly(t *testing.T) {
	skills := []*skill.Skill{{Name: "alpha", Description: "Alpha skill"}}
	result, err := BuildDeveloperInstructions(nil, skills, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := skill.GeneratePromptXML(skills)
	if result.Instructions != expected {
		t.Fatalf("unexpected instructions: %q", result.Instructions)
	}
	if len(result.PromptFiles) != 0 {
		t.Fatalf("expected no prompt files, got %v", result.PromptFiles)
	}
}

func TestBuildDeveloperInstructionsCombined(t *testing.T) {
	skills := []*skill.Skill{{Name: "alpha", Description: "Alpha skill"}}
	renderer := stubPromptRenderer(map[string]*prompt.RenderResult{
		"main": {Content: []byte("prompt"), Files: []string{"main.txt"}},
	})

	result, err := BuildDeveloperInstructions([]string{"main"}, skills, renderer, "session-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := skill.GeneratePromptXML(skills) + "\n\nprompt"
	if result.Instructions != expected {
		t.Fatalf("unexpected instructions: %q", result.Instructions)
	}
	if len(result.PromptFiles) != 1 || result.PromptFiles[0] != "main.txt" {
		t.Fatalf("unexpected prompt files: %v", result.PromptFiles)
	}
}

func stubPromptRenderer(results map[string]*prompt.RenderResult) PromptRenderer {
	return func(promptName string, ctx prompt.RenderContext) (*prompt.RenderResult, error) {
		result, ok := results[promptName]
		if !ok {
			return nil, fmt.Errorf("prompt %q not found", promptName)
		}
		return result, nil
	}
}
