package schemas

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

// CodexNotifications accepts a boolean or a list of strings.
type CodexNotifications struct{}

func (CodexNotifications) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{Type: "boolean"},
			{
				Type:  "array",
				Items: &jsonschema.Schema{Type: "string"},
			},
		},
	}
}

// CodexShellEnvironmentPolicy holds shell environment policy configuration.
type CodexShellEnvironmentPolicy map[string]interface{}

// CodexConfigLayerStack describes the merged config layer provenance.
type CodexConfigLayerStack map[string]interface{}

// CodexModelProvider describes the selected provider (string) or full config object.
type CodexModelProvider struct{}

func (CodexModelProvider) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		AnyOf: []*jsonschema.Schema{
			{Type: "string"},
			{
				Type:                 "object",
				AdditionalProperties: &jsonschema.Schema{},
			},
		},
	}
}

// CodexModelProviderInfo captures provider configuration details.
type CodexModelProviderInfo map[string]interface{}

// CodexMcpServerConfig captures MCP server configuration details.
type CodexMcpServerConfig map[string]interface{}

// CodexHistoryConfig captures history persistence configuration details.
type CodexHistoryConfig map[string]interface{}

// CodexUriBasedFileOpener captures file opener configuration details.
type CodexUriBasedFileOpener map[string]interface{}

// CodexNoticeConfig captures notice configuration details.
type CodexNoticeConfig map[string]interface{}

// CodexOtelConfig captures OTEL configuration details.
type CodexOtelConfig map[string]interface{}

// CodexFeaturesConfig stores feature flags by name.
type CodexFeaturesConfig map[string]bool

// CodexProjectConfig captures project trust configuration.
type CodexProjectConfig struct {
	// Trust level for the project (trusted/untrusted).
	TrustLevel *string `json:"trust_level,omitempty"`
}

// CodexGhostSnapshotConfig captures undo snapshot configuration.
type CodexGhostSnapshotConfig struct {
	// Exclude untracked files larger than this many bytes from snapshots.
	IgnoreLargeUntrackedFiles *int64 `json:"ignore_large_untracked_files,omitempty"`
	// Ignore untracked directories containing this many files or more.
	IgnoreLargeUntrackedDirs *int64 `json:"ignore_large_untracked_dirs,omitempty"`
	// Disable all ghost snapshot warning events.
	DisableWarnings *bool `json:"disable_warnings,omitempty"`
}

// CodexConfig defines the CLI-specific configuration for the codex CLI.
//
// Field names follow the Codex Config struct in codex-config-mod.rs. All fields
// are optional to allow partial overrides in CLI config.
type CodexConfig struct {
	// Provenance for how this Config was derived (merged layers + enforced requirements).
	ConfigLayerStack CodexConfigLayerStack `json:"config_layer_stack,omitempty"`

	// Optional override of model selection.
	Model *string `json:"model,omitempty"`

	// Model used specifically for review sessions.
	ReviewModel *string `json:"review_model,omitempty"`

	// Size of the context window for the model, in tokens.
	ModelContextWindow *int64 `json:"model_context_window,omitempty"`

	// Token usage threshold triggering auto-compaction of conversation history.
	ModelAutoCompactTokenLimit *int64 `json:"model_auto_compact_token_limit,omitempty"`

	// Key into the model_providers map that specifies which provider to use.
	ModelProviderID *string `json:"model_provider_id,omitempty"`

	// Info needed to make an API request to the model.
	ModelProvider *CodexModelProvider `json:"model_provider,omitempty"`

	// Approval policy for executing commands.
	ApprovalPolicy *string `json:"approval_policy,omitempty" jsonschema:"enum=never,enum=on-failure,enum=untrusted,enum=on-request"`

	// Sandbox policy for executing commands.
	SandboxPolicy *string `json:"sandbox_policy,omitempty" jsonschema:"enum=read-only,enum=workspace-write,enum=danger-full-access"`

	// True if the user set approval_policy or sandbox_mode explicitly.
	DidUserSetCustomApprovalPolicyOrSandboxMode *bool `json:"did_user_set_custom_approval_policy_or_sandbox_mode,omitempty"`

	// On Windows, indicates a workspace-write sandbox was coerced to read-only.
	ForcedAutoModeDowngradedOnWindows *bool `json:"forced_auto_mode_downgraded_on_windows,omitempty"`

	// Shell environment policy configuration.
	ShellEnvironmentPolicy *CodexShellEnvironmentPolicy `json:"shell_environment_policy,omitempty"`

	// When true, AgentReasoning events are suppressed from output.
	HideAgentReasoning *bool `json:"hide_agent_reasoning,omitempty"`

	// When true, AgentReasoningRawContentEvent events are shown in output.
	ShowRawAgentReasoning *bool `json:"show_raw_agent_reasoning,omitempty"`

	// User-provided instructions from AGENTS.md.
	UserInstructions *string `json:"user_instructions,omitempty"`

	// Base instructions override.
	BaseInstructions *string `json:"base_instructions,omitempty"`

	// Developer instructions override injected as a separate message.
	DeveloperInstructions *string `json:"developer_instructions,omitempty"`

	// Compact prompt override.
	CompactPrompt *string `json:"compact_prompt,omitempty"`

	// Optional external notifier command (argv without JSON payload).
	Notify []string `json:"notify,omitempty"`

	// TUI notifications preference (bool or list).
	TuiNotifications *CodexNotifications `json:"tui_notifications,omitempty"`

	// Enable ASCII animations and shimmer effects in the TUI.
	Animations *bool `json:"animations,omitempty"`

	// Show startup tooltips in the TUI welcome screen.
	ShowTooltips *bool `json:"show_tooltips,omitempty"`

	// Override the events-per-wheel-tick factor for TUI2 scroll normalization.
	TuiScrollEventsPerTick *uint16 `json:"tui_scroll_events_per_tick,omitempty"`

	// Override the number of lines applied per wheel tick in TUI2.
	TuiScrollWheelLines *uint16 `json:"tui_scroll_wheel_lines,omitempty"`

	// Override the number of lines per tick-equivalent used for trackpad scrolling in TUI2.
	TuiScrollTrackpadLines *uint16 `json:"tui_scroll_trackpad_lines,omitempty"`

	// Trackpad acceleration: approximate number of events required to gain +1x speed in TUI2.
	TuiScrollTrackpadAccelEvents *uint16 `json:"tui_scroll_trackpad_accel_events,omitempty"`

	// Trackpad acceleration: maximum multiplier applied to trackpad-like streams in TUI2.
	TuiScrollTrackpadAccelMax *uint16 `json:"tui_scroll_trackpad_accel_max,omitempty"`

	// Control how TUI2 interprets mouse scroll input (wheel vs trackpad).
	TuiScrollMode *string `json:"tui_scroll_mode,omitempty"`

	// Override the wheel tick detection threshold (ms) for TUI2 auto scroll mode.
	TuiScrollWheelTickDetectMaxMs *uint64 `json:"tui_scroll_wheel_tick_detect_max_ms,omitempty"`

	// Override the wheel-like end-of-stream threshold (ms) for TUI2 auto scroll mode.
	TuiScrollWheelLikeMaxDurationMs *uint64 `json:"tui_scroll_wheel_like_max_duration_ms,omitempty"`

	// Invert mouse scroll direction for TUI2.
	TuiScrollInvert *bool `json:"tui_scroll_invert,omitempty"`

	// Controls whether the TUI uses the terminal's alternate screen buffer.
	TuiAlternateScreen *string `json:"tui_alternate_screen,omitempty" jsonschema:"enum=auto,enum=always,enum=never"`

	// The directory treated as the current working directory.
	Cwd *string `json:"cwd,omitempty"`

	// Preferred store for CLI auth credentials.
	CliAuthCredentialsStoreMode *string `json:"cli_auth_credentials_store_mode,omitempty" jsonschema:"enum=file,enum=keyring,enum=auto"`

	// Definition for MCP servers that Codex can reach out to for tool calls.
	McpServers map[string]CodexMcpServerConfig `json:"mcp_servers,omitempty"`

	// Preferred store for MCP OAuth credentials.
	McpOAuthCredentialsStoreMode *string `json:"mcp_oauth_credentials_store_mode,omitempty" jsonschema:"enum=keyring,enum=file,enum=auto"`

	// Combined provider map (defaults merged with user-defined overrides).
	ModelProviders map[string]CodexModelProviderInfo `json:"model_providers,omitempty"`

	// The active profile name used to derive this config (if any).
	Profile *string `json:"profile,omitempty"`

	// Maximum number of bytes to include from an AGENTS.md project doc file.
	ProjectDocMaxBytes *uint64 `json:"project_doc_max_bytes,omitempty"`

	// Additional filenames to try when looking for project-level docs.
	ProjectDocFallbackFilenames []string `json:"project_doc_fallback_filenames,omitempty"`

	// Token budget applied when storing tool/function outputs in the context manager.
	ToolOutputTokenLimit *uint64 `json:"tool_output_token_limit,omitempty"`

	// Directory containing all Codex state.
	CodexHome *string `json:"codex_home,omitempty"`

	// Settings that govern if and what will be written to history.jsonl.
	History *CodexHistoryConfig `json:"history,omitempty"`

	// Optional URI-based file opener.
	FileOpener *CodexUriBasedFileOpener `json:"file_opener,omitempty"`

	// Path to the codex-linux-sandbox executable.
	CodexLinuxSandboxExe *string `json:"codex_linux_sandbox_exe,omitempty"`

	// Value to use for reasoning.effort.
	ModelReasoningEffort *string `json:"model_reasoning_effort,omitempty"`

	// Value to use for reasoning.summary.
	ModelReasoningSummary *string `json:"model_reasoning_summary,omitempty"`

	// Force-enable reasoning summaries for the configured model.
	ModelSupportsReasoningSummaries *bool `json:"model_supports_reasoning_summaries,omitempty"`

	// Verbosity control for GPT-5 models.
	ModelVerbosity *string `json:"model_verbosity,omitempty"`

	// Base URL for requests to ChatGPT.
	ChatgptBaseURL *string `json:"chatgpt_base_url,omitempty"`

	// When set, restricts ChatGPT login to a specific workspace identifier.
	ForcedChatgptWorkspaceID *string `json:"forced_chatgpt_workspace_id,omitempty"`

	// When set, restricts the login mechanism users may use.
	ForcedLoginMethod *string `json:"forced_login_method,omitempty"`

	// Include the apply_patch tool for models that benefit from it.
	IncludeApplyPatchTool *bool `json:"include_apply_patch_tool,omitempty"`

	// Whether to request web search tool support.
	ToolsWebSearchRequest *bool `json:"tools_web_search_request,omitempty"`

	// Use the experimental unified exec tool.
	UseExperimentalUnifiedExecTool *bool `json:"use_experimental_unified_exec_tool,omitempty"`

	// Settings for ghost snapshots (used for undo).
	GhostSnapshot *CodexGhostSnapshotConfig `json:"ghost_snapshot,omitempty"`

	// Centralized feature flags; source of truth for feature gating.
	Features CodexFeaturesConfig `json:"features,omitempty"`

	// The active profile name used to derive this config (if any).
	ActiveProfile *string `json:"active_profile,omitempty"`

	// The currently active project config.
	ActiveProject *CodexProjectConfig `json:"active_project,omitempty"`

	// Tracks whether the Windows onboarding screen has been acknowledged.
	WindowsWslSetupAcknowledged *bool `json:"windows_wsl_setup_acknowledged,omitempty"`

	// Collection of notices shown to the user.
	Notices *CodexNoticeConfig `json:"notices,omitempty"`

	// Checks for updates on startup when true.
	CheckForUpdateOnStartup *bool `json:"check_for_update_on_startup,omitempty"`

	// Disable burst-paste detection for typed input entirely.
	DisablePasteBurst *bool `json:"disable_paste_burst,omitempty"`

	// Disables analytics across Codex product surfaces.
	AnalyticsEnabled *bool `json:"analytics_enabled,omitempty"`

	// Disables feedback collection across Codex product surfaces.
	FeedbackEnabled *bool `json:"feedback_enabled,omitempty"`

	// OTEL configuration.
	Otel *CodexOtelConfig `json:"otel,omitempty"`
}

func (CodexConfig) MarshalJSONSchema() ([]byte, error) {
	schema := CodexSchema()
	return json.Marshal(schema)
}

func CodexSchema() *jsonschema.Schema {
	return GenerateSchema(CodexConfig{})
}
