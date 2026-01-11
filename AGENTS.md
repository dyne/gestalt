# Gestalt LLM Orientation

This repo is a Go backend + Svelte frontend for a multi-terminal dashboard with optional agent profiles, skills, and a small CLI. Use this as the minimum context to start any plan task.

## Project shape
- Backend entrypoint: `cmd/gestalt/main.go` loads config/env, skills (`config/skills`), agents (`config/agents`), builds a `terminal.Manager`, and registers REST/WS routes in `internal/api/routes.go`.
- Core backend packages:
  - `internal/terminal`: PTY sessions (`session.go`), manager (`manager.go`), history buffers/loggers.
  - `internal/api`: REST/WS handlers, auth middleware, JSON errors.
  - `internal/agent`: agent profile parsing/validation (`loader.go`, `agent.go`).
  - `internal/skill`: skill metadata/loader and prompt XML.
  - `internal/watcher`: fsnotify watcher, EventHub, git branch monitoring.
  - `internal/logging`: structured logs + buffer.
- Frontend (Svelte, Vite): `frontend/src/App.svelte` orchestrates tabs; `frontend/src/views/Dashboard.svelte` shows agents/skills/terminals; `frontend/src/lib/terminalStore.js` owns xterm + WebSocket; `frontend/src/lib/eventStore.js` shares /ws/events.
- CLI: `cmd/gestalt-send` pipes stdin to agent terminals over REST.

## Runtime flow (high level)
- REST: `/api/terminals` create/list/delete sessions; `/api/agents` lists agent profiles.
- WS: `/ws/terminal/:id` streams PTY data to xterm; `/ws/events` streams filesystem events.
- Agents:
  - Agent ID = filename in `config/agents/*.json` (without `.json`).
  - Agent name = `name` field (must be unique).
  - Single-instance enforced: one running terminal per agent name.
  - Terminal tabs use agent name as label.
  - Event types: `file_changed`, `git_branch_changed`, `watch_error`.

## Agent profiles
- Prompt names in `config/agents/*.json` can reference `.tmpl` or `.txt` files (backward compatible).
- The `skills` field lists optional skills the agent may load later (Claude-style skills); treat them as available but not auto-applied at start.

## Key API endpoints
- `/api/status` system status
- `/api/terminals` (GET/POST), `/api/terminals/:id` (DELETE)
- `/api/terminals/:id/output`, `/api/terminals/:id/input-history` (GET/POST)
- `/api/agents` (GET)
- `/api/agents/:name/input` (POST raw bytes to agent terminal)
- `/ws/events` (WebSocket; subscribe message supported)

## CLI (gestalt-send)
- `gestalt-send <agent-name-or-id>` posts stdin to `/api/agents/:name/input`.
- Resolves agent name/id case-insensitively via `/api/agents` and errors on ambiguity.
- `--start` auto-creates agent via `/api/terminals` using agent ID.
- Completions: `gestalt-send completion bash|zsh` (uses cached agent list).

## Prompt templating
- Prompt files can be `.txt` (plain) or `.tmpl` (templated); templates render at agent start.
- Include syntax: `{{include filename}}` on its own line.
- Include resolution: paths are relative to the workdir root; for bare filenames also search `config/prompts` with `.tmpl`, `.txt`, and `.md` extensions.
- Includes are text-only (binary files are skipped) and depth-limited to 3.
- Failed includes are silent (the directive line is skipped).
- Use cases: shared fragments, DRY prompts, easier maintenance.

## Tests
- Backend: `GOCACHE=/tmp/gocache /usr/local/go/bin/go test ./...`
- Frontend: `cd frontend && npm test`

## Filesystem events
- Uses `github.com/fsnotify/fsnotify` + EventHub to publish updates to `/ws/events`.
- PLAN.org changes and git branch changes are watched on startup.
- `GESTALT_MAX_WATCHES` caps active watches (default 100).

## Planning workflow (must follow)
- Work is tracked in `PLAN.org` (Org-mode). L1 = feature, L2 = steps.
- Exactly one WIP L1 and one WIP L2 at a time.
- For non-tiny work: update the plan first, then ask for confirmation before implementing.

## Repo conventions
- Prefer minimal changes and minimal dependencies.
- Use ASCII-only edits unless the file already uses non-ASCII.
- Avoid destructive git commands unless explicitly requested.

## Versioning and releases
- Conventional commits drive semantic version bumps via `scripts/get-next-version.js` (semver).
- Backend version is `internal/version.Version`, exposed in `/api/status` and startup logs.
- Frontend version comes from `frontend/src/lib/version.js` with Vite define `__GESTALT_VERSION__`.
- Makefile supports `VERSION=...` for builds; CI workflows set VERSION and release tags.

## CLI flags and help
- gestalt and gestalt-send use stdlib flag parsing with priority: CLI flag > env var > default.
- gestalt flags mirror GESTALT_* env vars; `Config.Sources` records the source for each value.
- `gestalt --help`, `gestalt --version`, `gestalt completion bash|zsh`, plus `--verbose`/`--quiet` log level control.
- gestalt-send supports `--url`, `--token`, `--start`, `--verbose`, `--debug`, `--help`, `--version`.

## Temporal HITL integration notes
- Session workflows default on: omit `workflow` to enable, or set `workflow=false` / `use_workflow=false` to disable.
- Workflow list/detail: `GET /api/workflows`, `GET /api/terminals/:id/workflow/history`.
- Resume/abort: `POST /api/terminals/:id/workflow/resume` with `continue` or `abort`.
- Metrics: `GET /api/metrics` exposes workflow/activity counters and timings.
- Dev server auto-management: set `GESTALT_TEMPORAL_DEV_SERVER=true` or `--temporal-dev-server` to run Temporal in `.gestalt/temporal`.
- Temporal defaults: workflows and dev server are enabled unless flags/env disable them.
- Handoff is design-only: `session.handoff` is reserved; implementation deferred.

## UI/UX polish notes (2026-01-10)
- Relative time formatting lives in `frontend/src/lib/timeUtils.js` and is used across logs and workflow UI with ISO tooltips.
- Dashboard agent buttons are Start/Open only; running state is derived from the `terminals` prop for live updates.
- Terminal tabs no longer have close icons; terminal shutdown is via the Terminal header close pill with a native dialog.
- Terminal start success is logged to the console instead of a toast notification.
- Header branding uses imported SVG assets for the Gestalt logo and Dyne icon/logotype.
- Workflow tracking is always on; the dashboard toggle was removed and running state is synced from agent terminal IDs.
- Terminal resizing uses a ResizeObserver to refit xterm and send PTY resize updates on width changes.
- `make dev` runs the Go backend and Vite dev server together for live UI updates.
