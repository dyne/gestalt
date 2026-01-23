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

// CodexNotify accepts a string or a list of strings.
type CodexNotify struct{}

func (CodexNotify) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{Type: "string"},
			{
				Type:  "array",
				Items: &jsonschema.Schema{Type: "string"},
			},
		},
	}
}

// CodexShellEnvironmentPolicy holds shell environment policy configuration.
type CodexShellEnvironmentPolicy map[string]interface{}

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

// CodexAnalyticsConfig captures analytics config (ConfigToml analytics table).
type CodexAnalyticsConfig struct {
	Enabled *bool `json:"enabled,omitempty"`
}

// CodexFeedbackConfig captures feedback config (ConfigToml feedback table).
type CodexFeedbackConfig struct {
	Enabled *bool `json:"enabled,omitempty"`
}

// CodexToolsConfig captures tools config (ConfigToml tools table).
type CodexToolsConfig struct {
	WebSearch        *bool `json:"web_search,omitempty"`
	WebSearchRequest *bool `json:"web_search_request,omitempty"`
	ViewImage        *bool `json:"view_image,omitempty"`
}

// CodexTuiConfig captures TUI config (ConfigToml tui table).
type CodexTuiConfig struct {
	Notifications                *CodexNotifications `json:"notifications,omitempty"`
	Animations                   *bool               `json:"animations,omitempty"`
	ShowTooltips                 *bool               `json:"show_tooltips,omitempty"`
	ScrollEventsPerTick          *uint16             `json:"scroll_events_per_tick,omitempty"`
	ScrollWheelLines             *uint16             `json:"scroll_wheel_lines,omitempty"`
	ScrollTrackpadLines          *uint16             `json:"scroll_trackpad_lines,omitempty"`
	ScrollTrackpadAccelEvents    *uint16             `json:"scroll_trackpad_accel_events,omitempty"`
	ScrollTrackpadAccelMax       *uint16             `json:"scroll_trackpad_accel_max,omitempty"`
	ScrollMode                   *string             `json:"scroll_mode,omitempty"`
	ScrollWheelTickDetectMaxMs   *uint64             `json:"scroll_wheel_tick_detect_max_ms,omitempty"`
	ScrollWheelLikeMaxDurationMs *uint64             `json:"scroll_wheel_like_max_duration_ms,omitempty"`
	ScrollInvert                 *bool               `json:"scroll_invert,omitempty"`
	AlternateScreen              *string             `json:"alternate_screen,omitempty" jsonschema:"enum=auto,enum=always,enum=never"`
}

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
// Field names follow the Codex ConfigToml struct in codex-config-mod.rs. All fields
// are optional to allow partial overrides in CLI config.
type CodexConfig struct {
	// Optional override of model selection.
	Model *string `json:"model,omitempty"`

	// Model used specifically for review sessions.
	ReviewModel *string `json:"review_model,omitempty"`

	// Size of the context window for the model, in tokens.
	ModelContextWindow *int64 `json:"model_context_window,omitempty"`

	// Token usage threshold triggering auto-compaction of conversation history.
	ModelAutoCompactTokenLimit *int64 `json:"model_auto_compact_token_limit,omitempty"`

	// Provider key to use from the model_providers map.
	ModelProvider *string `json:"model_provider,omitempty"`

	// Approval policy for executing commands.
	ApprovalPolicy *string `json:"approval_policy,omitempty" jsonschema:"enum=never,enum=on-failure,enum=untrusted,enum=on-request"`

	// Sandbox mode to use.
	SandboxMode *string `json:"sandbox_mode,omitempty" jsonschema:"enum=read-only,enum=workspace-write,enum=danger-full-access"`

	// Sandbox configuration to apply if sandbox_mode is workspace-write.
	SandboxWorkspaceWrite map[string]interface{} `json:"sandbox_workspace_write,omitempty"`

	// Shell environment policy configuration.
	ShellEnvironmentPolicy *CodexShellEnvironmentPolicy `json:"shell_environment_policy,omitempty"`

	// Optional external notifier command (argv without JSON payload).
	Notify *CodexNotify `json:"notify,omitempty"`

	// Base instructions.
	Instructions *string `json:"instructions,omitempty"`

	// Developer instructions override injected as a separate message.
	DeveloperInstructions *string `json:"developer_instructions,omitempty"`

	// Compact prompt override.
	CompactPrompt *string `json:"compact_prompt,omitempty"`

	// The directory treated as the current working directory.
	Cwd *string `json:"cwd,omitempty"`

	// Preferred store for CLI auth credentials.
	CliAuthCredentialsStore *string `json:"cli_auth_credentials_store,omitempty" jsonschema:"enum=file,enum=keyring,enum=auto"`

	// Definition for MCP servers that Codex can reach out to for tool calls.
	McpServers map[string]CodexMcpServerConfig `json:"mcp_servers,omitempty"`

	// Preferred store for MCP OAuth credentials.
	McpOAuthCredentialsStore *string `json:"mcp_oauth_credentials_store,omitempty" jsonschema:"enum=keyring,enum=file,enum=auto"`

	// Combined provider map (defaults merged with user-defined overrides).
	ModelProviders map[string]CodexModelProviderInfo `json:"model_providers,omitempty"`

	// The active profile name used to derive this config (if any).
	Profile *string `json:"profile,omitempty"`

	// Named profile definitions.
	Profiles map[string]interface{} `json:"profiles,omitempty"`

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

	// When true, AgentReasoning events are suppressed from output.
	HideAgentReasoning *bool `json:"hide_agent_reasoning,omitempty"`

	// When true, AgentReasoningRawContentEvent events are shown in output.
	ShowRawAgentReasoning *bool `json:"show_raw_agent_reasoning,omitempty"`

	// Path to the codex-linux-sandbox executable.
	CodexLinuxSandboxExe *string `json:"codex_linux_sandbox_exe,omitempty"`

	// Value to use for reasoning.effort.
	ModelReasoningEffort *string `json:"model_reasoning_effort,omitempty"`

	// Value to use for reasoning.summary.
	ModelReasoningSummary *string `json:"model_reasoning_summary,omitempty"`

	// Verbosity control for GPT-5 models.
	ModelVerbosity *string `json:"model_verbosity,omitempty"`

	// Force-enable reasoning summaries for the configured model.
	ModelSupportsReasoningSummaries *bool `json:"model_supports_reasoning_summaries,omitempty"`

	// Base URL for requests to ChatGPT.
	ChatgptBaseURL *string `json:"chatgpt_base_url,omitempty"`

	// When set, restricts ChatGPT login to a specific workspace identifier.
	ForcedChatgptWorkspaceID *string `json:"forced_chatgpt_workspace_id,omitempty"`

	// When set, restricts the login mechanism users may use.
	ForcedLoginMethod *string `json:"forced_login_method,omitempty"`

	// Named project configuration blocks.
	Projects map[string]CodexProjectConfig `json:"projects,omitempty"`

	// Settings for ghost snapshots (used for undo).
	GhostSnapshot *CodexGhostSnapshotConfig `json:"ghost_snapshot,omitempty"`

	// Centralized feature flags; source of truth for feature gating.
	Features CodexFeaturesConfig `json:"features,omitempty"`

	// Project root detection markers.
	ProjectRootMarkers []string `json:"project_root_markers,omitempty"`

	// TUI settings.
	Tui *CodexTuiConfig `json:"tui,omitempty"`

	// Tools settings.
	Tools *CodexToolsConfig `json:"tools,omitempty"`

	// Analytics settings.
	Analytics *CodexAnalyticsConfig `json:"analytics,omitempty"`

	// Feedback settings.
	Feedback *CodexFeedbackConfig `json:"feedback,omitempty"`

	// Collection of notices shown to the user.
	Notice *CodexNoticeConfig `json:"notice,omitempty"`

	// Checks for updates on startup when true.
	CheckForUpdateOnStartup *bool `json:"check_for_update_on_startup,omitempty"`

	// Disable burst-paste detection for typed input entirely.
	DisablePasteBurst *bool `json:"disable_paste_burst,omitempty"`

	// Tracks whether the Windows onboarding screen has been acknowledged.
	WindowsWslSetupAcknowledged *bool `json:"windows_wsl_setup_acknowledged,omitempty"`

	// OSS provider for local models.
	OssProvider *string `json:"oss_provider,omitempty"`

	// Optional experimental legacy flags.
	ExperimentalInstructionsFile      *string `json:"experimental_instructions_file,omitempty"`
	ExperimentalCompactPromptFile     *string `json:"experimental_compact_prompt_file,omitempty"`
	ExperimentalUseUnifiedExecTool    *bool   `json:"experimental_use_unified_exec_tool,omitempty"`
	ExperimentalUseFreeformApplyPatch *bool   `json:"experimental_use_freeform_apply_patch,omitempty"`

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
