# Gestalt LLM Orientation

Go backend + Svelte frontend for a multi-session dashboard with optional agent profiles, skills, and a small CLI.
Use this as the minimum context to start any plan task.

## Project shape
- Backend entry: `cmd/gestalt/main.go` loads config/env, skills (`.gestalt/config/skills`), agents (`.gestalt/config/agents`), builds `terminal.Manager`, registers REST/WS routes in `internal/api/routes.go`.
- Core packages:
  - `internal/terminal`: PTY sessions, manager, history/logging.
  - `internal/api`: REST/WS handlers, auth middleware, JSON errors.
  - `internal/agent`: agent profile parsing/validation.
  - `internal/skill`: skill metadata/loader, prompt XML.
  - `internal/watcher`: fsnotify watcher, event bus helpers, git branch monitoring.
  - `internal/logging`: structured logs + buffer.
- Frontend: `frontend/src/App.svelte` tabs; `frontend/src/views/Dashboard.svelte` agents/sessions; `frontend/src/lib/terminalStore.js` text stream + WS; `frontend/src/lib/eventStore.js` `/ws/events`.
- CLI: `cmd/gestalt-send` pipes stdin to agent sessions over REST.

## Runtime flow (high level)
- REST: `/api/sessions` create/list/delete; `/api/agents` list profiles.
- WS: `/ws/session/:id` PTY stream; `/ws/events` filesystem events.
- Agents: ID = filename in `.gestalt/config/agents/*.toml` (no `.toml`); name = `name` field (unique); session ids derive from agent names with per-run counters; session tab label = session id.

## Agent profiles (TOML only)
- Files: `.gestalt/config/agents/*.toml` (JSON not supported).
- Prompts: `.tmpl`, `.md`, `.txt` in `.gestalt/config/prompts`.
- `skills` lists optional skills (available, not auto-applied).
- `cli_type` + `cli_config` enable CLI-specific settings (schema-validated).
- Base fields: `name`, `shell`, `prompt`, `skills`, `onair_string`, `use_workflow`, `llm_model`.
- Shell commands generated at session start from `cli_config` (codex `-c key:value`, copilot `--flag`/`--no-flag`).
- Full reference: `docs/agent-configuration.md`.

## Key endpoints
- `/api/status`
- `/api/sessions` (GET/POST), `/api/sessions/:id` (DELETE)
- `/api/sessions/:id/output`, `/api/sessions/:id/input` (POST), `/api/sessions/:id/activate` (POST), `/api/sessions/:id/input-history` (GET/POST)
- `/api/agents`, `/api/agents/:name/send-input`
- `/api/flow/activities`, `/api/flow/event-types`, `/api/flow/config` (GET/PUT)
- `/api/flow/config/export` (GET), `/api/flow/config/import` (POST)
- WS: `/api/agents/events`, `/api/sessions/events`, `/api/config/events`, `/api/workflows/events`, `/ws/events`

## CLI (gestalt-send)
- `gestalt-send <agent-name-or-id>` resolves the agent and posts stdin to `/api/sessions/:id/input`.
- `gestalt-send --session-id <id>` writes directly to `/api/sessions/:id/input`.
- Resolves name/id case-insensitively; errors on ambiguity.
- `--start` auto-creates agent via `/api/sessions` using agent ID.
- Server flags are `--host` + `--port` (no `--url`).
- Completions: `gestalt-send completion bash|zsh`.

## Prompt templating
- Prompt files render at agent start and process directives (`.tmpl`, `.md`, `.txt`).
- Include syntax: `{{include filename}}` on its own line.
- Port syntax: `{{port <service>}}` on its own line; resolves to the runtime port number.
- Available services: `backend`, `frontend`, `temporal`, `otel`.
- Unknown services or missing port resolver skip silently (line removed).
- Scope: directives resolve in prompt files only; skill XML does not substitute ports yet.
- Resolve: absolute/relative path loads from workdir root; otherwise search `.gestalt/config/prompts` (`.tmpl`, `.md`, `.txt`), then `.gestalt/prompts`.
- Text-only includes, depth limit 3, de-dup by canonical path; failed includes are silent.

## Event-driven architecture
- Core type: `internal/event.Bus[T]` (sync fan-out, optional history).
- Buses: `watcher_events`, `agent_events`, `terminal_events`, `terminal_output`, `workflow_events`, `config_events`, `logs`.
- WS mappings: `/ws/events` (filesystem), `/api/agents/events`, `/api/sessions/events`, `/api/config/events`, `/api/workflows/events`.
- Filesystem events via `watcher.WatchFile` into `watcher_events`.
- Debug: `GESTALT_EVENT_DEBUG=true` logs all published events.
- History: `BusOptions.HistorySize`, `ReplayLast`, `DumpHistory`.
- Event payloads: `FileEvent`, `TerminalEvent`, `AgentEvent`, `ConfigEvent`, `WorkflowEvent`, `LogEvent`.
- Flow:
```
filesystem -> watcher_events -> /ws/events -> frontend eventStore -> UI
agent/session/workflow/config -> Manager/handlers -> /api/*/events -> frontend stores -> UI
terminal output -> Session output bus -> /ws/session/:id -> frontend text view
```
- Testing: `internal/event/testing.go` helpers (`MockBus`, `EventCollector`, `ReceiveWithTimeout`, `MatchEvent`).

## Filesystem events
- Uses `fsnotify` + `event.Bus[watcher.Event]` to publish `/ws/events`.
- Watches `.gestalt/plans/` and git branch changes on startup.
- `GESTALT_MAX_WATCHES` caps watches (default 100).

## Conventions + tooling
- Prefer minimal changes and dependencies; ASCII-only edits unless file already uses non-ASCII.
- Avoid destructive git commands unless explicitly requested.
- Tests: backend `GOCACHE=/tmp/gocache /usr/local/go/bin/go test ./...`; frontend `cd frontend && npm test`.

## Temporal quick reference
| Area | Defaults/Flags | Key APIs / Behavior |
| --- | --- | --- |
| Temporal HITL | Workflows on by default; disable via `workflow=false` / `use_workflow=false`. Dev server: `GESTALT_TEMPORAL_DEV_SERVER=true` or `--temporal-dev-server` (runs in `.gestalt/temporal`). | `GET /api/workflows`, `GET /api/sessions/:id/workflow/history`, `POST /api/sessions/:id/workflow/resume` (`continue`/`abort`), `GET /api/metrics/summary`. |

## OpenTelemetry observability
- Collector lifecycle lives in `internal/otel/collector.go`; config `.gestalt/otel/collector.yaml`, data file `.gestalt/otel/otel.json`.
- SDK setup in `internal/otel/sdk.go`; env: `GESTALT_OTEL_SDK_ENABLED`, `GESTALT_OTEL_SERVICE_NAME`, `GESTALT_OTEL_RESOURCE_ATTRIBUTES`.
- APIs: `/api/otel/logs` (POST ingest only), `/api/otel/traces`, `/api/otel/metrics`; `/api/logs` GET proxies to OTel when collector is active; `/api/metrics/summary` exposes API stats.
- Remote export: `GESTALT_OTEL_REMOTE_ENDPOINT` + `GESTALT_OTEL_REMOTE_INSECURE` adds otlpexporter to collector pipelines.
- Collector self-metrics are disabled in generated config; set `GESTALT_OTEL_SELF_METRICS=true` to keep default telemetry readers.
- Limits: `GESTALT_OTEL_MAX_RECORDS` caps records read from `otel.json`; `/api/otel/*` limit is capped at 1000.
- Runtime checklist:
  - `/api/status` shows `otel_collector_running=true` with PID and HTTP endpoint.
  - TCP dial to the `otel_collector_http_endpoint` succeeds.
  - Logs do not show repeated "connection refused" from OTLP exporters.
  - `otel_collector_last_exit` is empty or has a recent error with `otel_collector_restart_count` incremented.

## WebSocket consolidation notes
- Backend WS streaming now uses a shared write-loop helper (`internal/api/ws_helpers.go`) with per-handler read logic; logs/events/session handlers were updated to use it and have close-handling tests.
- Frontend WS helper tests live in `frontend/src/lib/wsStore.test.js` and cover reconnect, subscription payloads, and listener error handling.

## MCP/CLI stabilization notes
- Runner bridge websocket support was removed (`/ws/runner/session/:id` is no longer registered).
- External CLI session creation in the server now creates tmux windows (`Gestalt <workdir>`, window=`session.id`) and a lazy shared agents hub attach session.
- `gestalt-agent` now creates the external session via API, then runs tmux attach/switch-client (`--dryrun` prints the tmux command).
- External runner sessions are treated as non-interactive in `/ws/session/:id` and frontend terminal state (no reconnect loop).
- GUI module normalization maps legacy `terminal` to `console`; default server modules are `["console"]`.

## Frontend store simplification notes
- Dashboard orchestration (agent/config/git event handling, config extraction counts, git context) lives in `frontend/src/lib/dashboardStore.js`; Dashboard view now just binds store state.
- Terminal text stream behavior is covered by `frontend/src/lib/terminal/segments.test.js` and `frontend/src/components/TerminalTextView.test.js`.

## Frontend chunking notes
- Vite manual chunks split `@xterm/*` into `vendor-xterm` (`frontend/vite.config.js`).
- Non-default tab views (Plan/Flow/Terminal) lazy-load in `frontend/src/App.svelte`; avoid eager terminal imports at the root.

## Plan UI notes
- Plans are served via `/api/plans` (metadata + headings) from `.gestalt/plans/`; PlanView renders PlanCard details/summary and refreshes on file change events.
- `frontend/src/views/PlanView.svelte` debounces plan refreshes from file watcher events to avoid request floods.
- Event store regression tests cover malformed payloads and burst events (`frontend/tests/eventStore.test.js`, `frontend/src/lib/wsStore.test.js`).

## Docs and README notes
- Documentation site uses VitePress with `docs/` as the source root and config at `docs/.vitepress/config.mts`.
- Root scripts:
  - `npm run docs` (dev server)
  - `npm run docs:build`
  - `npm run docs:preview`
- `README.md` is intentionally short (quick setup, build, testing, license) and links to `docs/` for full reference material.
- Historical plan artifacts were moved to `docs/legacy/` (for example `docs/legacy/old-plan.org`) and are not part of the published docs nav.

## Logs UI notes
- Log entries are normalized via `frontend/src/lib/logEntry.js`; Dashboard and LogsView use inline `<details>/<summary>` disclosures with context tables and optional raw JSON.
- Log disclosures include Copy JSON actions, and the Dashboard intel section shows logs + API metrics side-by-side with agent sessions above.
- Clipboard controls should use `frontend/src/lib/clipboard.js` to gate copy actions on secure contexts (HTTPS or secure localhost) and hide when unavailable.
- Dashboard status pills (workdir, git remote, git branch) are clickable and copy their values when clipboard is allowed.

## Session logging notes
- Async file logger behavior is covered by `internal/terminal/async_file_logger_test.go` to validate flush-on-close and drop behavior.
- Session metadata boundaries are covered in `internal/terminal/session_test.go` (Info metadata + workflow identifier defaults).
- Log context fields (output_tail, bell context, stderr) are cleaned (ANSI escape sequences, control codes, repeated characters removed; text, tabs, and newlines preserved).
