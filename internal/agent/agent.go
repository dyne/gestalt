package agent

import (
	"fmt"
	"strings"
)

// Agent defines a terminal profile loaded from config/agents/*.json.
type Agent struct {
	Name       string `json:"name"`
	Shell      string `json:"shell"`
	PromptFile string `json:"prompt_file"`
	LLMType    string `json:"llm_type"`
	LLMModel   string `json:"llm_model"`
}

// Validate ensures required fields are present and values are supported.
func (a Agent) Validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("agent name is required")
	}
	if strings.TrimSpace(a.Shell) == "" {
		return fmt.Errorf("agent shell is required")
	}
	if strings.TrimSpace(a.LLMType) == "" {
		return fmt.Errorf("agent llm_type is required")
	}
	switch a.LLMType {
	case "copilot", "codex", "promptline":
		return nil
	default:
		return fmt.Errorf("agent llm_type %q is invalid", a.LLMType)
	}
}
