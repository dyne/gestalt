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
- `singleton` (bool, optional): Allow only one running instance (default true).
- `llm_model` (string, optional): Model hint for UI/API.

Prompt names resolve against `.gestalt/config/prompts`, trying `.tmpl`, `.md`, then `.txt`.

Any additional top-level keys (outside the base fields) are treated as CLI config and validated. A legacy `[cli_config]` table is still accepted, but no longer required.

## gestalt-agent CLI

`gestalt-agent <agent-id>` runs Codex in a standalone TUI with the agent prompt pre-rendered.

- The `agent-id` is the filename in `config/agents/*.toml` (or `.gestalt/config/agents/*.toml` if extracted). `coder` and `coder.toml` are equivalent.
- Config precedence: local `./config/**` overrides extracted `./.gestalt/config/**`. If `.gestalt/config` is missing, it is extracted from embedded defaults first.
- Prompt rendering matches server behavior. Supported directives: `{{include filename}}`, `{{port <service>}}`, `{{session id}}`.
  - Directives may appear inline or as the only content on a line.
  - If the trimmed line is only a directive, unresolved values skip the entire line.
  - Inline unresolved directives render as empty strings, preserving the rest of the line.
  - Escape a directive with `\{{...}}` to render it literally.
  - Standalone tooling may not provide a session ID; inline `{{session id}}` renders empty in that case.
- Port directive mapping in standalone mode:
  - `frontend`: `GESTALT_PORT` or `57417`
  - `backend`: `GESTALT_BACKEND_PORT` or `frontend`
  - `temporal`: port from `GESTALT_TEMPORAL_HOST` or `7233`
  - `otel`: port from `GESTALT_OTEL_HTTP_ENDPOINT` or `4318`
- Standalone port values are best-effort defaults and may not match a running Gestalt instance with dynamic ports.
- Extremely large prompts can hit OS argv length limits.
- `--dryrun` prints the full `codex` command (including `developer_prompt`) without launching it.

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

The generated command replaces any explicit `shell` value when agents are loaded.

If no CLI config keys are set, `shell` is used as-is.

### Codex notify vs tui.notifications

`notify` is a **Codex root key** that defines an argv array for notifier hooks.
`tui.notifications` is separate and only controls OSC 9 popups in the TUI.

Gestalt injects a notify hook for Codex sessions at runtime (overriding any
existing `notify` value) so Temporal notary events are always recorded:

```toml
notify = ["gestalt-notify", "--session-id", "<session-id>", "--agent-id", "<agent-id>", "--agent-name", "<agent-name>"]
```

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
sandbox_mode = "workspace-write"
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

- `analytics.enabled`
- `approval_policy`
- `chatgpt_base_url`
- `check_for_update_on_startup`
- `cli_auth_credentials_store`
- `codex_home`
- `codex_linux_sandbox_exe`
- `compact_prompt`
- `cwd`
- `developer_instructions`
- `disable_paste_burst`
- `experimental_compact_prompt_file`
- `experimental_instructions_file`
- `experimental_use_freeform_apply_patch`
- `experimental_use_unified_exec_tool`
- `features`
- `feedback.enabled`
- `file_opener`
- `forced_chatgpt_workspace_id`
- `forced_login_method`
- `ghost_snapshot`
- `hide_agent_reasoning`
- `history`
- `instructions`
- `mcp_oauth_credentials_store`
- `mcp_servers`
- `model`
- `model_auto_compact_token_limit`
- `model_context_window`
- `model_provider`
- `model_providers`
- `model_reasoning_effort`
- `model_reasoning_summary`
- `model_supports_reasoning_summaries`
- `model_verbosity`
- `notice`
- `notify`
- `oss_provider`
- `otel`
- `profile`
- `profiles`
- `project_doc_fallback_filenames`
- `project_doc_max_bytes`
- `project_root_markers`
- `projects`
- `review_model`
- `sandbox_mode`
- `sandbox_workspace_write`
- `shell_environment_policy`
- `show_raw_agent_reasoning`
- `tool_output_token_limit`
- `tools.web_search`
- `tools.view_image`
- `tui.alternate_screen`
- `tui.animations`
- `tui.notifications`
- `tui.scroll_events_per_tick`
- `tui.scroll_invert`
- `tui.scroll_mode`
- `tui.scroll_trackpad_accel_events`
- `tui.scroll_trackpad_accel_max`
- `tui.scroll_trackpad_lines`
- `tui.scroll_wheel_like_max_duration_ms`
- `tui.scroll_wheel_lines`
- `tui.scroll_wheel_tick_detect_max_ms`
- `tui.show_tooltips`
- `windows_wsl_setup_acknowledged`
