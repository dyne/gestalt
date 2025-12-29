package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// PromptList supports "prompt" as a string or array in JSON.
type PromptList []string

func (p *PromptList) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*p = nil
		return nil
	}

	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		single = strings.TrimSpace(single)
		if single == "" {
			*p = nil
			return nil
		}
		*p = PromptList{single}
		return nil
	}

	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	cleaned := make(PromptList, 0, len(many))
	for _, entry := range many {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			cleaned = append(cleaned, entry)
		}
	}
	if len(cleaned) == 0 {
		*p = nil
		return nil
	}
	*p = cleaned
	return nil
}

// Agent defines a terminal profile loaded from config/agents/*.json.
type Agent struct {
	Name        string     `json:"name"`
	Shell       string     `json:"shell"`
	Prompts     PromptList `json:"prompt,omitempty"`
	OnAirString string     `json:"onair_string,omitempty"`
	LLMType     string     `json:"llm_type"`
	LLMModel    string     `json:"llm_model"`
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
		// continue
	default:
		return fmt.Errorf("agent llm_type %q is invalid", a.LLMType)
	}

	for i, prompt := range a.Prompts {
		if strings.TrimSpace(prompt) == "" {
			return fmt.Errorf("agent prompt %d is empty", i)
		}
	}

	return nil
}
