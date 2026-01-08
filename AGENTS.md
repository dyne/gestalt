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
