package schemas

import "github.com/invopop/jsonschema"

// CopilotResume accepts a boolean or session ID string.
type CopilotResume struct{}

func (CopilotResume) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{Type: "boolean"},
			{Type: "string"},
		},
	}
}

// CopilotConfig defines the CLI-specific configuration for the copilot CLI.
type CopilotConfig struct {
	// Add a directory to the allowed list for file access.
	AddDir []string `json:"add_dir,omitempty"`
	// Additional MCP servers configuration as JSON string or file path.
	AdditionalMcpConfig []string `json:"additional_mcp_config,omitempty"`
	// Specify a custom agent to use (prompt mode only).
	Agent *string `json:"agent,omitempty"`
	// Disable file path verification and allow access to any path.
	AllowAllPaths *bool `json:"allow_all_paths,omitempty"`
	// Allow all tools to run automatically without confirmation.
	AllowAllTools *bool `json:"allow_all_tools,omitempty"`
	// Allow specific tools.
	AllowTool []string `json:"allow_tool,omitempty"`
	// Show the startup banner.
	Banner *bool `json:"banner,omitempty"`
	// Resume the most recent session.
	Continue *bool `json:"continue,omitempty"`
	// Deny specific tools.
	DenyTool []string `json:"deny_tool,omitempty"`
	// Disable all built-in MCP servers.
	DisableBuiltinMcps *bool `json:"disable_builtin_mcps,omitempty"`
	// Disable a specific MCP server.
	DisableMcpServer []string `json:"disable_mcp_server,omitempty"`
	// Disable parallel execution of tools.
	DisableParallelToolsExecution *bool `json:"disable_parallel_tools_execution,omitempty"`
	// Prevent automatic access to the system temporary directory.
	DisallowTempDir *bool `json:"disallow_temp_dir,omitempty"`
	// Enable all GitHub MCP server tools.
	EnableAllGithubMcpTools *bool `json:"enable_all_github_mcp_tools,omitempty"`
	// Set log file directory.
	LogDir *string `json:"log_dir,omitempty"`
	// Set the log level.
	LogLevel *string `json:"log_level,omitempty" jsonschema:"enum=none,enum=error,enum=warning,enum=info,enum=debug,enum=all,enum=default"`
	// Set the AI model to use.
	Model *string `json:"model,omitempty" jsonschema:"enum=claude-sonnet-4.5,enum=claude-sonnet-4,enum=claude-haiku-4.5,enum=gpt-5"`
	// Disable all color output.
	NoColor *bool `json:"no_color,omitempty"`
	// Disable loading of custom instructions from AGENTS.md and related files.
	NoCustomInstructions *bool `json:"no_custom_instructions,omitempty"`
	// Execute a prompt directly without interactive mode.
	Prompt *string `json:"prompt,omitempty"`
	// Resume from a previous session (optionally specify session ID).
	Resume *CopilotResume `json:"resume,omitempty"`
	// Enable screen reader optimizations.
	ScreenReader *bool `json:"screen_reader,omitempty"`
	// Enable or disable streaming mode.
	Stream *string `json:"stream,omitempty" jsonschema:"enum=on,enum=off"`
}

func CopilotSchema() *jsonschema.Schema {
	return GenerateSchema(CopilotConfig{})
}
