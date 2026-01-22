package schemas

import "github.com/invopop/jsonschema"

// CopilotConfig defines the CLI-specific configuration for the copilot CLI.
// Full field coverage is added in the copilot schema implementation task.
type CopilotConfig struct {
	Model          string `json:"model,omitempty"`
	AllowAllTools  bool   `json:"allow_all_tools,omitempty"`
	DisableBuiltins bool  `json:"disable_builtins,omitempty"`
	Profile        string `json:"profile,omitempty"`
}

func CopilotSchema() *jsonschema.Schema {
	return GenerateSchema(CopilotConfig{})
}
