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
	UseWorkflow *bool                  `json:"use_workflow,omitempty" toml:"use_workflow,omitempty"`
	Singleton   *bool                  `json:"singleton,omitempty" toml:"singleton,omitempty"`
	Interface   string                 `json:"interface,omitempty" toml:"interface,omitempty"`
	CodexMode   string                 `json:"codex_mode,omitempty" toml:"codex_mode,omitempty"`
	CLIType     string                 `json:"cli_type,omitempty" toml:"cli_type,omitempty"`
	LLMModel    string                 `json:"llm_model,omitempty" toml:"llm_model,omitempty"`
	CLIConfig   map[string]interface{} `json:"cli_config,omitempty" toml:"cli_config,omitempty"`
	ConfigHash  string                 `json:"-" toml:"-"`
}

const (
	AgentInterfaceCLI = "cli"
	AgentInterfaceMCP = "mcp"
	CodexModeMCPServer = "mcp-server"
	CodexModeTUI       = "tui"
)

// Validate ensures required fields are present and values are supported.
func (a *Agent) Validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("agent name is required")
	}
	resolvedInterface, err := a.ResolveInterface()
	if err != nil {
		return err
	}
	a.Interface = resolvedInterface
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

func (a *Agent) ResolveInterface() (string, error) {
	cliType := strings.TrimSpace(a.CLIType)
	if strings.EqualFold(cliType, "codex") {
		mode := strings.TrimSpace(a.CodexMode)
		if mode != "" {
			normalizedMode := strings.ToLower(mode)
			switch normalizedMode {
			case CodexModeMCPServer, CodexModeTUI:
				a.CodexMode = normalizedMode
			default:
				return "", &ValidationError{
					Path:        "codex_mode",
					Expected:    "\"mcp-server\" or \"tui\"",
					Actual:      actualType(mode),
					ActualValue: mode,
				}
			}
		}
	}
	interfaceValue := strings.TrimSpace(a.Interface)
	if interfaceValue == "" {
		if strings.EqualFold(strings.TrimSpace(a.CodexMode), CodexModeMCPServer) {
			interfaceValue = AgentInterfaceMCP
		}
	}
	if interfaceValue == "" {
		interfaceValue = AgentInterfaceCLI
	}
	interfaceValue = strings.ToLower(interfaceValue)
	switch interfaceValue {
	case AgentInterfaceCLI, AgentInterfaceMCP:
	default:
		return "", &ValidationError{
			Path:    "interface",
			Message: fmt.Sprintf("expected \"cli\" or \"mcp\" (got %q)", interfaceValue),
		}
	}
	if interfaceValue == AgentInterfaceMCP && !strings.EqualFold(strings.TrimSpace(a.CLIType), "codex") {
		return "", &ValidationError{
			Path:    "interface",
			Message: "interface=\"mcp\" requires cli_type=\"codex\"",
		}
	}
	return interfaceValue, nil
}

func (a *Agent) RuntimeInterface(forceTUI bool) (string, error) {
	resolvedInterface, err := a.ResolveInterface()
	if err != nil {
		return "", err
	}
	if forceTUI && strings.EqualFold(strings.TrimSpace(a.CLIType), "codex") {
		return AgentInterfaceCLI, nil
	}
	return resolvedInterface, nil
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
		if strings.TrimSpace(a.CLIType) == "" {
			return "", fmt.Errorf("agent cli_type is required when CLI config is set")
		}
		if err := ValidateAgentConfig(a.CLIType, a.CLIConfig); err != nil {
			return "", err
		}
		command := BuildShellCommand(a.CLIType, a.CLIConfig)
		if strings.TrimSpace(command) == "" {
			return "", fmt.Errorf("agent cli_type %q cannot build shell command", a.CLIType)
		}
		return command, nil
	}
	command := strings.TrimSpace(a.Shell)
	if command == "" {
		return "", fmt.Errorf("agent shell is required")
	}
	return command, nil
}
