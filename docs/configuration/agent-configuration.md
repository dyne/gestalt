# Agent configuration (TOML)

Gestalt agent profiles live in `.gestalt/config/agents/*.toml`. JSON agent configs are not supported. Each file defines a single agent profile, keyed by filename (agent ID).

## Base fields

All agent files support the following fields:

- `name` (string, required): Human-readable name shown in the UI.
- `shell` (string, optional): Explicit shell command. Required if no CLI config keys are set.
- `interface` (string, optional): `cli` or `mcp` (default `cli`). `mcp` is only supported for `cli_type="codex"`.
- `cli_type` (string, optional): CLI type (e.g., `codex`, `copilot`). Required when CLI config keys are set.
- `codex_mode` (string, optional, legacy): For `cli_type="codex"`, deprecated alias for `interface` when `interface` is unset (`mcp-server` maps to `interface="mcp"`, `tui` maps to `interface="cli"`).
- `prompt` (string or array, optional): Prompt names (no extension) to inject (Codex renders these into `developer_instructions`).
- `skills` (array, optional): Skill names to inject (Codex renders these into `developer_instructions`).
- `gui_modules` (array, optional): UI module flags for sessions (e.g., `["plan-progress"]`). Known modules: `console` (session view) and `plan-progress` (sidebar). Legacy `terminal` is accepted and normalized to `console`. Defaults to `["console"]` for server sessions and `["console","plan-progress"]` for external sessions when unset.
- `onair_string` (string, optional): Wait for this string before prompt injection (non-Codex only).
- `use_workflow` (bool, optional): Override workflow default.
- `singleton` (bool, optional): Allow only one running instance (default true).
- `llm_model` (string, optional): Model hint for UI/API.

Prompt names resolve against `.gestalt/config/prompts`, trying `.tmpl`, `.md`, then `.txt`.
Set `gui_modules` in the agent TOML to override the default module selection for that agent.

Any additional top-level keys (outside the base fields) are treated as CLI config and validated. A legacy `[cli_config]` table is still accepted, but no longer required.

The runtime can force Codex agents back to `interface="cli"` by setting `GESTALT_CODEX_FORCE_TUI=true`.

## Migration notes

- `gui_modules = ["terminal"]` is supported as a legacy alias and is normalized to `["console"]`.
- External `gestalt-agent` sessions no longer use a runner websocket bridge. The CLI now launches tmux and exits.
- For rollback during incident response, restore the previous runner bridge route/handlers and `gestalt-agent` bridge entrypoint in a single revert commit.

## gestalt-agent CLI

`gestalt-agent <agent-id>` connects to a running Gestalt server, registers an external session, and runs the agent in tmux.

- Requires a running server (`--host`, `--port`, `--token`/`GESTALT_TOKEN`).
- Requires `tmux` on `PATH`.
- The `agent-id` is the filename in `.gestalt/config/agents/*.toml` (`coder` and `coder.toml` are equivalent).
- Prompt rendering and session defaults come from the server response.
- After session creation, the CLI attaches tmux (`tmux attach` or `tmux switch-client`).
- `--dryrun` prints the resolved tmux attach command without executing it.

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

### Prompt injection (Codex)

For `cli_type="codex"`, Gestalt renders skills + prompt files into a single
`developer_instructions` value before the session starts. Prompt text is not
typed into the terminal stream (CLI or MCP). Any `developer_instructions`
provided in the CLI config is overwritten by the rendered content.

For non-Codex CLIs, prompt files are still typed into the terminal after the
`onair_string` (or a short delay if none is provided).

### Codex MCP mode

When `interface = "mcp"` (or legacy `codex_mode = "mcp-server"` with no `interface` set),
Gestalt runs `codex mcp-server` over stdio and renders a simple transcript in the terminal output:

- User prompts are echoed with a `> ` prefix.
- Assistant responses are plain text blocks (newlines preserved).
- Errors are prefixed with `! error:`.

In MCP mode, Gestalt emits workflow notify events directly (`source="codex-notify"`)
and does not inject Codex `notify=...` hooks.

Set `GESTALT_CODEX_FORCE_TUI=true` to force the legacy TUI path globally.

### Codex notify vs tui.notifications

`notify` is a **Codex root key** that defines an argv array for notifier hooks.
`tui.notifications` is separate and only controls OSC 9 popups in the TUI.

Gestalt injects a notify hook for Codex sessions at runtime (overriding any
existing `notify` value) so Flow automations can react to notify events:

```toml
notify = ["gestalt-notify", "--session-id", "<session-id>"]
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
interface = "cli"
prompt = ["coder"]
gui_modules = ["plan-progress"]
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
