# Agent configuration (TOML)

Gestalt agent profiles live in `.gestalt/config/agents/*.toml`. JSON agent configs are not supported. Each file defines a single agent profile, keyed by filename (agent ID).

## Base fields

All agent files support the following fields:

- `name` (string, required): Human-readable name shown in the UI.
- `shell` (string, optional): Explicit shell command. Required if no CLI config keys are set.
- `cli_type` (string, optional): CLI type (e.g., `codex`, `copilot`). Required when CLI config keys are set.
- `prompt` (string or array, optional): Prompt names (no extension) to inject.
- `skills` (array, optional): Skill names to inject.
- `onair_string` (string, optional): Wait for this string before prompt injection.
- `use_workflow` (bool, optional): Override workflow default.
- `llm_model` (string, optional): Model hint for UI/API.

Prompt names resolve against `.gestalt/config/prompts`, trying `.tmpl`, `.md`, then `.txt`.

Any additional top-level keys (outside the base fields) are treated as CLI config and validated. A legacy `[cli_config]` table is still accepted, but no longer required.

## CLI config validation

- CLI config keys are validated against a per-CLI JSON Schema.
- Validation errors include file name and (when possible) the line/field.
- Invalid agent files are skipped with a warning.

Schemas live in `internal/agent/schemas/`:
- `codex` schema: `internal/agent/schemas/codex.go`
- `copilot` schema: `internal/agent/schemas/copilot.go`

## Shell command generation

When CLI config keys are present, Gestalt generates the shell command at session creation:

- **Codex:** `codex -c key=value` for each config entry.
  - Nested tables flatten to dot notation (e.g., `tui.scroll_mode`).
  - Arrays repeat `-c key=value` for each entry.
- **Copilot:** `copilot --flag value` for each entry.
  - Boolean flags use `--flag` or `--no-flag`.
  - Arrays repeat `--flag value` for each entry.

If no CLI config keys are set, `shell` is used as-is.

## Examples

Example files live in `config/agents/`:
- `codex-full-example.toml`
- `copilot-example.toml`
- `simple-shell-example.toml`

### Codex (TOML)

```toml
name = "Codex"
cli_type = "codex"
prompt = ["coder"]
model = "o3"
approval_policy = "on-request"
sandbox_policy = "workspace-write"
```

### Copilot (TOML)

```toml
name = "Copilot"
cli_type = "copilot"
model = "gpt-5"
allow_all_tools = true
```

### Simple shell

```toml
name = "Shell"
shell = "/bin/bash"
```

## Codex CLI config reference (schema keys)

All fields are optional. Some keys live inside nested tables (e.g., `active_project.trust_level`); use TOML tables to nest as needed.

- `active_profile`
- `active_project`
- `analytics_enabled`
- `animations`
- `approval_policy`
- `base_instructions`
- `chatgpt_base_url`
- `check_for_update_on_startup`
- `cli_auth_credentials_store_mode`
- `codex_home`
- `codex_linux_sandbox_exe`
- `compact_prompt`
- `config_layer_stack`
- `cwd`
- `developer_instructions`
- `did_user_set_custom_approval_policy_or_sandbox_mode`
- `disable_paste_burst`
- `disable_warnings`
- `features`
- `feedback_enabled`
- `file_opener`
- `forced_auto_mode_downgraded_on_windows`
- `forced_chatgpt_workspace_id`
- `forced_login_method`
- `ghost_snapshot`
- `hide_agent_reasoning`
- `history`
- `ignore_large_untracked_dirs`
- `ignore_large_untracked_files`
- `include_apply_patch_tool`
- `mcp_oauth_credentials_store_mode`
- `mcp_servers`
- `model`
- `model_auto_compact_token_limit`
- `model_context_window`
- `model_provider`
- `model_provider_id`
- `model_providers`
- `model_reasoning_effort`
- `model_reasoning_summary`
- `model_supports_reasoning_summaries`
- `model_verbosity`
- `notices`
- `notify`
- `otel`
- `profile`
- `project_doc_fallback_filenames`
- `project_doc_max_bytes`
- `review_model`
- `sandbox_policy`
- `shell_environment_policy`
- `show_raw_agent_reasoning`
- `show_tooltips`
- `tool_output_token_limit`
- `tools_web_search_request`
- `trust_level`
- `tui_alternate_screen`
- `tui_notifications`
- `tui_scroll_events_per_tick`
- `tui_scroll_invert`
- `tui_scroll_mode`
- `tui_scroll_trackpad_accel_events`
- `tui_scroll_trackpad_accel_max`
- `tui_scroll_trackpad_lines`
- `tui_scroll_wheel_like_max_duration_ms`
- `tui_scroll_wheel_lines`
- `tui_scroll_wheel_tick_detect_max_ms`
- `use_experimental_unified_exec_tool`
- `user_instructions`
- `windows_wsl_setup_acknowledged`
