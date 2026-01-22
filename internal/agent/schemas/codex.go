package schemas

import "github.com/invopop/jsonschema"

// CodexConfig defines the CLI-specific configuration for the codex CLI.
// Full field coverage is added in the codex schema implementation task.
type CodexConfig struct {
	Model          string       `json:"model,omitempty"`
	ApprovalPolicy string       `json:"approval_policy,omitempty"`
	SandboxMode    string       `json:"sandbox_mode,omitempty"`
	Profile        string       `json:"profile,omitempty"`
	Features       *CodexFeature `json:"features,omitempty"`
}

type CodexFeature struct {
	Experimental bool `json:"experimental,omitempty"`
}

func CodexSchema() *jsonschema.Schema {
	return GenerateSchema(CodexConfig{})
}
