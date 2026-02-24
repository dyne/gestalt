package agent

import (
	"fmt"
	"strings"
)

// PromptList supports "prompt" as a string or array in TOML.
type PromptList []string

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

// Agent defines a terminal profile loaded from config/agents/*.toml.
type Agent struct {
	Name        string                 `json:"name" toml:"name"`
	Shell       string                 `json:"shell,omitempty" toml:"shell,omitempty"`
	Prompts     PromptList             `json:"prompt,omitempty" toml:"prompt,omitempty"`
	Skills      []string               `json:"skills,omitempty" toml:"skills,omitempty"`
	OnAirString string                 `json:"onair_string,omitempty" toml:"onair_string,omitempty"`
	Singleton   *bool                  `json:"singleton,omitempty" toml:"singleton,omitempty"`
	Interface   string                 `json:"-" toml:"-"`
	CLIType     string                 `json:"-" toml:"-"`
	CLIConfig   map[string]interface{} `json:"-" toml:"-"`
	CodexMode   string                 `json:"codex_mode,omitempty" toml:"codex_mode,omitempty"`
	Model       string                 `json:"model,omitempty" toml:"model,omitempty"`
	Hidden      bool                   `json:"hidden" toml:"hidden,omitempty"`
	ConfigHash  string                 `json:"-" toml:"-"`
	warnings    []string               `json:"-" toml:"-"`
}

const (
	AgentInterfaceCLI = "cli"
)

// Validate ensures required fields are present and values are supported.
func (a *Agent) Validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("agent name is required")
	}
	if strings.TrimSpace(a.CodexMode) != "" {
		return &ValidationError{
			Path:    "codex_mode",
			Message: "codex_mode is no longer supported",
		}
	}
	if _, err := a.resolveShell(); err != nil {
		return err
	}

	for i, prompt := range a.Prompts {
		if strings.TrimSpace(prompt) == "" {
			return fmt.Errorf("agent prompt %d is empty", i)
		}
	}

	return nil
}

func (a *Agent) RuntimeInterface(forceTUI bool) (string, error) {
	_ = forceTUI
	return AgentInterfaceCLI, nil
}

// NormalizeShell applies CLI config shell generation using the resolved shell command.
func (a *Agent) NormalizeShell() error {
	command, err := a.resolveShell()
	if err != nil {
		return err
	}
	a.Shell = command
	return nil
}

func (a *Agent) resolveShell() (string, error) {
	if len(a.CLIConfig) > 0 {
		cliType := strings.ToLower(strings.TrimSpace(a.CLIType))
		if cliType == "" {
			return "", fmt.Errorf("agent cli_type is required")
		}
		command := strings.TrimSpace(BuildShellCommand(cliType, a.CLIConfig))
		if command == "" {
			return "", fmt.Errorf("agent cli_type %q is not supported", cliType)
		}
		return command, nil
	}
	command := strings.TrimSpace(a.Shell)
	if command == "" {
		return "", fmt.Errorf("agent shell is required")
	}
	return command, nil
}

// RuntimeType returns a lightweight agent type inferred from the shell command.
func (a *Agent) RuntimeType() string {
	if a == nil {
		return ""
	}
	if trimmed := strings.ToLower(strings.TrimSpace(a.CLIType)); trimmed != "" {
		return trimmed
	}
	command := strings.TrimSpace(a.Shell)
	if command == "" {
		return ""
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	switch strings.ToLower(fields[0]) {
	case "codex":
		return "codex"
	case "github-copilot", "copilot":
		return "copilot"
	default:
		return ""
	}
}
