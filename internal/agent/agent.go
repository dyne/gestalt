package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// PromptList supports "prompt" as a string or array in JSON/TOML.
type PromptList []string

func (p *PromptList) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*p = nil
		return nil
	}

	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		return p.setSinglePrompt(single)
	}

	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	return p.setPromptList(many)
}

func (p *PromptList) UnmarshalTOML(data interface{}) error {
	if data == nil {
		*p = nil
		return nil
	}
	switch typed := data.(type) {
	case string:
		return p.setSinglePrompt(typed)
	case []interface{}:
		items := make([]string, 0, len(typed))
		for _, entry := range typed {
			value, ok := entry.(string)
			if !ok {
				return fmt.Errorf("prompt entries must be strings")
			}
			items = append(items, value)
		}
		return p.setPromptList(items)
	default:
		return fmt.Errorf("prompt must be a string or array")
	}
}

func (p *PromptList) setSinglePrompt(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		*p = nil
		return nil
	}
	*p = PromptList{value}
	return nil
}

func (p *PromptList) setPromptList(values []string) error {
	cleaned := make(PromptList, 0, len(values))
	for _, entry := range values {
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

// Agent defines a terminal profile loaded from config/agents/*.json or *.toml.
type Agent struct {
	Name        string                 `json:"name" toml:"name"`
	Shell       string                 `json:"shell,omitempty" toml:"shell,omitempty"`
	Prompts     PromptList             `json:"prompt,omitempty" toml:"prompt,omitempty"`
	Skills      []string               `json:"skills,omitempty" toml:"skills,omitempty"`
	OnAirString string                 `json:"onair_string,omitempty" toml:"onair_string,omitempty"`
	UseWorkflow *bool                  `json:"use_workflow,omitempty" toml:"use_workflow,omitempty"`
	CLIType     string                 `json:"cli_type,omitempty" toml:"cli_type,omitempty"`
	LLMModel    string                 `json:"llm_model,omitempty" toml:"llm_model,omitempty"`
	CLIConfig   map[string]interface{} `json:"cli_config,omitempty" toml:"cli_config,omitempty"`
	ConfigHash  string                 `json:"-" toml:"-"`
}

type agentJSON struct {
	Name        string                 `json:"name"`
	Shell       string                 `json:"shell,omitempty"`
	Prompts     PromptList             `json:"prompt,omitempty"`
	Skills      []string               `json:"skills,omitempty"`
	OnAirString string                 `json:"onair_string,omitempty"`
	UseWorkflow *bool                  `json:"use_workflow,omitempty"`
	CLIType     string                 `json:"cli_type,omitempty"`
	LLMType     string                 `json:"llm_type,omitempty"`
	LLMModel    string                 `json:"llm_model,omitempty"`
	CLIConfig   map[string]interface{} `json:"cli_config,omitempty"`
}

func (a *Agent) UnmarshalJSON(data []byte) error {
	var payload agentJSON
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	cliType := strings.TrimSpace(payload.CLIType)
	if cliType == "" {
		cliType = strings.TrimSpace(payload.LLMType)
	}
	*a = Agent{
		Name:        payload.Name,
		Shell:       payload.Shell,
		Prompts:     payload.Prompts,
		Skills:      payload.Skills,
		OnAirString: payload.OnAirString,
		UseWorkflow: payload.UseWorkflow,
		CLIType:     cliType,
		LLMModel:    payload.LLMModel,
		CLIConfig:   payload.CLIConfig,
	}
	return nil
}

// Validate ensures required fields are present and values are supported.
func (a *Agent) Validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("agent name is required")
	}
	if len(a.CLIConfig) > 0 {
		if strings.TrimSpace(a.CLIType) == "" {
			return fmt.Errorf("agent cli_type is required when cli_config is set")
		}
		if err := ValidateAgentConfig(a.CLIType, a.CLIConfig); err != nil {
			return err
		}
		command := BuildShellCommand(a.CLIType, a.CLIConfig)
		if strings.TrimSpace(command) == "" {
			return fmt.Errorf("agent cli_type %q cannot build shell command", a.CLIType)
		}
		if strings.TrimSpace(a.Shell) != "" {
			log.Printf("agent %q: both shell and cli_config provided, cli_config takes precedence", a.Name)
		}
		a.Shell = command
	}
	if strings.TrimSpace(a.Shell) == "" {
		return fmt.Errorf("agent shell is required")
	}

	for i, prompt := range a.Prompts {
		if strings.TrimSpace(prompt) == "" {
			return fmt.Errorf("agent prompt %d is empty", i)
		}
	}

	return nil
}
