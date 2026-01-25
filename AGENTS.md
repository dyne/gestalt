# Gestalt LLM Orientation

Go backend + Svelte frontend for a multi-terminal dashboard with optional agent profiles, skills, and a small CLI.
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
- Frontend: `frontend/src/App.svelte` tabs; `frontend/src/views/Dashboard.svelte` agents/skills/terminals; `frontend/src/lib/terminalStore.js` xterm + WS; `frontend/src/lib/eventStore.js` `/ws/events`.
- CLI: `cmd/gestalt-send` pipes stdin to agent terminals over REST.

## Config extraction (startup)
- Embedded config extracts to `.gestalt/config/` using `config/manifest.json` FNV-1a 64-bit hashes; mismatches backup to `.bck`.
- Dev mode (`GESTALT_DEV_MODE=true` or `--dev`) skips extraction/validation and reads from `config/` (or `GESTALT_CONFIG_DIR`).
- `.gestalt/version.json` tracks build version; `--force-upgrade` bypasses major mismatch.
- Agent/skill validation logs warnings and skips invalid entries; prompt files must be text.
- Metrics log: `config extraction metrics`.
- PLAN moved to `.gestalt/PLAN.org` (startup migrates from root `PLAN.org`); `--extract-config` is a no-op.

## Runtime flow (high level)
- REST: `/api/terminals` create/list/delete; `/api/agents` list profiles.
- WS: `/ws/terminal/:id` PTY stream; `/ws/events` filesystem events.
- Agents: ID = filename in `.gestalt/config/agents/*.toml` (no `.toml`); name = `name` field (unique); single instance per name; terminal tab label = agent name.

## Agent profiles (TOML only)
- Files: `.gestalt/config/agents/*.toml` (JSON not supported).
- Prompts: `.tmpl`, `.md`, `.txt` in `.gestalt/config/prompts`.
- `skills` lists optional skills (available, not auto-applied).
- `cli_type` + `cli_config` enable CLI-specific settings (schema-validated).
- Base fields: `name`, `shell`, `prompt`, `skills`, `onair_string`, `use_workflow`, `llm_model`.
- Shell commands generated at session start from `cli_config` (codex `-c key:value`, copilot `--flag`/`--no-flag`).
- Full reference: `docs/agent-configuration.md`.

## Key endpoints
- `/api/status`, `/api/events/debug`
- `/api/terminals` (GET/POST), `/api/terminals/:id` (DELETE)
- `/api/terminals/:id/output`, `/api/terminals/:id/input-history` (GET/POST)
- `/api/agents`, `/api/agents/:name/input`
- WS: `/api/agents/events`, `/api/terminals/events`, `/api/config/events`, `/api/workflows/events`, `/ws/events`

## CLI (gestalt-send)
- `gestalt-send <agent-name-or-id>` posts stdin to `/api/agents/:name/input`.
- Resolves name/id case-insensitively; errors on ambiguity.
- `--start` auto-creates agent via `/api/terminals` using agent ID.
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
- WS mappings: `/ws/events` (filesystem), `/api/agents/events`, `/api/terminals/events`, `/api/config/events`, `/api/workflows/events`.
- Filesystem events via `watcher.WatchFile` into `watcher_events`.
- Debug: `GESTALT_EVENT_DEBUG=true`; `/api/events/debug` shows subscriber counts.
- History: `BusOptions.HistorySize`, `ReplayLast`, `DumpHistory`.
- Event payloads: `FileEvent`, `TerminalEvent`, `AgentEvent`, `ConfigEvent`, `WorkflowEvent`, `LogEvent`.
- Flow:
```
filesystem -> watcher_events -> /ws/events -> frontend eventStore -> UI
agent/terminal/workflow/config -> Manager/handlers -> /api/*/events -> frontend stores -> UI
terminal output -> Session output bus -> /ws/terminal/:id -> xterm
```
- Testing: `internal/event/testing.go` helpers (`MockBus`, `EventCollector`, `ReceiveWithTimeout`, `MatchEvent`).

## Filesystem events
- Uses `fsnotify` + `event.Bus[watcher.Event]` to publish `/ws/events`.
- Watches `.gestalt/PLAN.org` and git branch changes on startup.
- `GESTALT_MAX_WATCHES` caps watches (default 100).

## Planning workflow (must follow)
- Work tracked in `.gestalt/PLAN.org` (Org). L1 = feature, L2 = steps.
- Exactly one WIP L1 and one WIP L2 at a time.
- For non-tiny work: update the plan first, then ask for confirmation before implementing.

## Conventions + tooling
- Prefer minimal changes and dependencies; ASCII-only edits unless file already uses non-ASCII.
- Avoid destructive git commands unless explicitly requested.
- Tests: backend `GOCACHE=/tmp/gocache /usr/local/go/bin/go test ./...`; frontend `cd frontend && npm test`.

## Versioning + flags
- Conventional commits drive semver via `scripts/get-next-version.js`.
- Backend version: `internal/version.Version` (exposed in `/api/status`); frontend version: `frontend/src/lib/version.js` with `__GESTALT_VERSION__`.
- Flag priority: CLI > env > default; `Config.Sources` records source.
- `gestalt --help/--version`, `gestalt completion bash|zsh`, `--verbose`/`--quiet`.
- `gestalt-send --url --token --start --verbose --debug --help --version`.

## Temporal + SCIP quick reference
| Area | Defaults/Flags | Key APIs / Behavior |
| --- | --- | --- |
| Temporal HITL | Workflows on by default; disable via `workflow=false` / `use_workflow=false`. Dev server: `GESTALT_TEMPORAL_DEV_SERVER=true` or `--temporal-dev-server` (runs in `.gestalt/temporal`). | `GET /api/workflows`, `GET /api/terminals/:id/workflow/history`, `POST /api/terminals/:id/workflow/resume` (`continue`/`abort`), `GET /api/metrics`. |
| SCIP | Enabled when `GESTALT_SCIP_INDEX_PATH` points to `index.db` (default `.gestalt/scip/index.db`). | `/api/scip/symbols?q=...` -> `/api/scip/symbols/<id>` -> `/api/scip/symbols/<id>/references`; `/api/scip/files/<path>`; `POST /api/scip/index` `{ "path": "/repo/path" }`. |

## SCIP CLI (2026-01-25)
- Offline CLI lives in `cmd/gestalt-scip` and builds with `make build-scip`.
- CLI commands: `symbols`, `definition`, `references`, `files`; default merges all `.scip` files, `--language` filters, `--format` supports `text|json|toon`.
- Symbol IDs in CLI output are base64url-encoded and safe for shells; `definition`/`references` accept encoded IDs and raw SCIP IDs.
- CLI output omits `kind` when it would be `UnspecifiedKind` and strips fenced code markers plus language tag lines from `documentation`.
- Backend async indexing: `/api/scip/status`, `/api/scip/reindex`, `/api/scip/events`; startup indexing runs unless `--noindex` or `GESTALT_SCIP_NO_INDEX=true`.
- `scip-typescript-finder` is reference-only; parsing logic is copied into `cmd/gestalt-scip/src/lib`.
- Prompt guidance: `config/prompts/scip.tmpl` prefers the API, while `config/prompts/scip-cli.md` documents offline CLI-first navigation.

## OpenTelemetry observability
- Collector lifecycle lives in `internal/otel/collector.go`; config `.gestalt/otel/collector.yaml`, data file `.gestalt/otel/otel.json`.
- SDK setup in `internal/otel/sdk.go`; env: `GESTALT_OTEL_SDK_ENABLED`, `GESTALT_OTEL_SERVICE_NAME`, `GESTALT_OTEL_RESOURCE_ATTRIBUTES`.
- APIs: `/api/otel/logs`, `/api/otel/traces`, `/api/otel/metrics`; `/api/logs` GET proxies to OTel when collector is active; `/api/metrics/summary` exposes API stats.
- Remote export: `GESTALT_OTEL_REMOTE_ENDPOINT` + `GESTALT_OTEL_REMOTE_INSECURE` adds otlpexporter to collector pipelines.
- Limits: `GESTALT_OTEL_MAX_RECORDS` caps records read from `otel.json`; `/api/otel/*` limit is capped at 1000.

## Recent changes (2026-01-10 to 2026-01-23)
- UI: relative time formatting in `frontend/src/lib/timeUtils.js` (ISO tooltips).
- UI: dashboard agent buttons are Start/Open only; running state from `terminals` prop.
- UI: terminal tabs have no close icons; close via header pill + native dialog.
- UI: terminal start success logs to console; header branding uses SVG assets.
- UI: workflow tracking always on; terminal resizing uses ResizeObserver; touch scrolling uses pointer events with 10px threshold + inertia; `scrollSensitivity` in `frontend/src/components/Terminal.svelte`.
- Dev: `make dev` runs backend + Vite dev server.
- Agents: TOML-only; loader rejects `.json`; `cli_config` schema-validated; shellgen skips empty values.
- Agents/skills: prompt resolution unified with prompt parser (.md support), skill XML includes license/compatibility/allowed tools with `xml.EscapeText`, loader path normalization shared in `internal/fsutil`, validation no longer mutates shell (use `NormalizeShell`), and agent registry centralizes reloads.
- Errors: REST errors include message + code (error alias retained), WS streams send error envelopes, and frontend error messages use `errorUtils`.
- Testing: added DSR/output backpressure, watcher restart, CLI error mapping, structured error e2e, and Terminal/CommandInput component tests; frontend coverage workflow documented.
- Agents: new sessions refresh configs via `agent.AgentCache`; sessions store `Command` + `ConfigHash` snapshots.
- Temporal: workflows store agent config + hash in memo (`internal/temporal/memo.go`); legacy JSON memos in `.gestalt/temporal` are warned/rejected.
- CLI: `gestalt config validate --agents-dir ...`.
- Backend: app wiring now lives in `internal/app` (`app.Build` loads skills/agents and constructs `terminal.Manager`).
- Backend: `cmd/gestalt` subcommands dispatch via `cmd/gestalt/commands.go`; execution tests live in `cmd/gestalt/commands_test.go`.
- Backend: server startup flow moved to `cmd/gestalt/server_command.go`; `cmd/gestalt/main.go` only resolves commands.
- CLI: `gestalt-send` split into `parse.go`, `http.go`, and `completion.go`; agent cache now JSON (`agents-cache.json`).
- CLI: shared HTTP helpers live in `internal/client`; `gestalt-send` uses them for agent lookups and input sends.
- CLI: common help/version flag wiring in `internal/cli`.

## WebSocket consolidation notes
- Backend WS streaming now uses a shared write-loop helper (`internal/api/ws_helpers.go`) with per-handler read logic; logs/events/terminal handlers were updated to use it and have close-handling tests.
- Frontend WS helper tests live in `frontend/src/lib/wsStore.test.js` and cover reconnect, subscription payloads, and listener error handling.

## Frontend store simplification notes
- Dashboard orchestration (agent/config/git event handling, config extraction counts, git context) lives in `frontend/src/lib/dashboardStore.js`; Dashboard view now just binds store state.
- Terminal input helpers have direct tests in `frontend/src/lib/terminal/input.test.js`.

## Terminal session logging notes
- Async file logger behavior is covered by `internal/terminal/async_file_logger_test.go` to validate flush-on-close and drop behavior.
- Session metadata boundaries are covered in `internal/terminal/session_test.go` (Info metadata + workflow identifier defaults).
- Log context fields (output_tail, bell context, stderr) are cleaned (ANSI escape sequences, control codes, repeated characters removed; text, tabs, and newlines preserved).
