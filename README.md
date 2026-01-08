# Gestalt

We invite you to stop assembling the pieces and start perceiving the whole.

Welcome to Gestalt.

### More info on [dyne.org/gestalt](https://dyne.org/gestalt)

## Quick Start

Build (needs nodejs and npm):
```
make
```

Launch (needs golang)
```
go run cmd/gestalt/main.go
```

Default listens to 0.0.0.0 port 8080

When running local open browser at http://localhost:8080

## Testing

Backend:
```
go test ./...
```

Frontend:
```
cd frontend
npm run build
```

### Token authentication

If you don’t set GESTALT_TOKEN, auth is disabled.

- REST auth is Authorization: Bearer <token> when `GESTALT_TOKEN` is set (handled in frontend/src/lib/api.js).
- WS auth uses ?token=<token> in the URL (also handled in frontend/src/lib/api.js).
- Default port is 8080; override with `GESTALT_PORT`.

`GESTALT_TOKEN` is just an arbitrary shared secret you choose. The
server only checks that incoming REST/WS requests present the same
token. To generate a random token:

- macOS/Linux: `export GESTALT_TOKEN=$(openssl rand -hex 16)`
- Windows PowerShell `$env:GESTALT_TOKEN = -join ((48..57)+(97..102) | Get-Random -Count 32 | % {[char]$_})`

## Configuration

Environment variables:
- `GESTALT_PORT` (default 8080)
- `GESTALT_SHELL` (default: system shell)
- `GESTALT_TOKEN` (default: empty, disables auth)
- `GESTALT_SESSION_PERSIST` (default true)
- `GESTALT_SESSION_DIR` (default `.gestalt/sessions`)
- `GESTALT_SESSION_BUFFER_LINES` (default 1000)
- `GESTALT_SESSION_RETENTION_DAYS` (default 7)
- `GESTALT_MAX_WATCHES` (default 100)
- `GESTALT_INPUT_HISTORY_DIR` (default `.gestalt/input-history`)

Session logs and input history now live under `.gestalt/` by default. If you
previously stored data in `logs/`, move it manually if you want to keep it.

## Dashboard

- Working directory is shown prominently so you can confirm the server context.
- Agent cards show Start/Stop controls based on whether the agent is running.
- Logs are embedded on the dashboard; the Logs tab is removed.
- The Plan view auto-refreshes every 5s (manual refresh is still available).

## Embedded Resources

The `gestalt` binary embeds the frontend bundle and default config so it can run
from any directory without external files.

Overrides (per subdirectory, relative to the current working directory):
- `./gestalt/config/agents` overrides embedded agents
- `./gestalt/config/prompts` overrides embedded prompts
- `./gestalt/config/skills` overrides embedded skills
- `./gestalt/dist` overrides embedded frontend assets

Extract embedded defaults:
```
gestalt --extract-config
```
This creates `./gestalt/` with `config/` and `dist/`. Existing files are left in
place and reported as warnings.

Build from source with embedded assets:
```
make gestalt
```

## Filesystem watching

Gestalt uses `github.com/fsnotify/fsnotify` for filesystem events because it is
the de-facto, cross-platform watcher (inotify/kqueue/ReadDirectoryChangesW),
stable, widely used, and BSD 3-Clause licensed (compatible with AGPL).

Failure handling: watcher errors are logged at warning level and retried with
exponential backoff (up to 3 attempts). If watching remains unavailable, the
server emits `watch_error` events and the UI falls back to polling with a toast
("File watching unavailable").

## Filesystem Event System

Architecture: Watcher (fsnotify) → EventHub → `/ws/events` broadcaster.

Event types:
- `file_changed` (path + timestamp)
- `git_branch_changed` (path holds branch name)
- `watch_error` (watch failures)

WebSocket protocol:
- Connect to `/ws/events` (token in query when auth is enabled).
- Optional filter message: `{"subscribe":["file_changed","git_branch_changed","watch_error"]}`.
- Server messages: `{"type":"file_changed","path":"PLAN.org","timestamp":"..."}`.

Backend usage example:
```
hub.WatchFile("PLAN.org")
hub.Subscribe(watcher.EventTypeFileChanged, func(event watcher.Event) {
  // React to changes.
})
```

Debouncing:
- Per-path 100ms coalescing (configurable in code via `watcher.Options.Debounce`).
- Latest event data wins within the debounce window.

Limits and cleanup:
- `GESTALT_MAX_WATCHES` caps active watches (default 100).
- Watchers drop paths with no subscribers; a cleanup loop trims stale entries.

Frontend event store:
```
import { subscribe, eventConnectionStatus } from './lib/eventStore.js'

const unsubscribe = subscribe('file_changed', (event) => {
  // Refresh UI.
})
```
Use `eventConnectionStatus` to drive fallback polling if needed, and unsubscribe
on teardown to avoid leaks.

## API endpoints

API (development snapshot)
- GET /api/status - system status (terminal count, server time)
- GET /api/terminals - list active terminals
- POST /api/terminals - create a new terminal
- DELETE /api/terminals/:id - terminate a terminal
- GET /api/terminals/:id/output - recent output lines (buffered)
- GET /api/plan - read PLAN.org contents
- GET /api/logs - recent system logs (query: level, since, limit)

Auth
- REST endpoints expect `Authorization: Bearer <token>` when `GESTALT_TOKEN` is set.
- WebSocket connections accept `?token=<token>` for browser compatibility.

## Logging and notifications

Backend logging is structured and buffered in memory (ring buffer). Logs are
available via REST and WebSocket, and the UI shows toasts plus a Logs tab.

Log levels:
- debug
- info
- warning
- error

REST log retrieval:
- GET `/api/logs?level=warning&since=2025-01-01T12:00:00Z&limit=100`
  - `level` filters by minimum severity (warning includes warning+error)
  - `since` is RFC3339 UTC timestamp
  - `limit` returns the last N entries (default 100)

WebSocket log streaming:
- `/ws/logs` sends JSON log entries in real time.
- Clients can send `{"level":"warning"}` to adjust minimum severity.

Toast notifications:
- Automatically surface key events (API errors, terminal connection issues).
- Auto-dismiss defaults: info 5s, warning 7s, errors stay until dismissed.
- Preferences are available via the “Notifications” button and stored in localStorage.

Backend logging usage:
- Use the structured logger (`Logger.Info/Warn/Error`) with context fields.
- Avoid `log.Printf` in new code so logs remain visible in the UI.

## Agent profiles

Agent profiles live in `config/agents/*.json` and are loaded at startup.

Fields:
- `name` (required)
- `shell` (required)
- `prompt` (optional: string or array of strings)
- `llm_type` (required: `copilot`, `codex`, `promptline`)
- `llm_model` (optional; use `default`)

Example:
```
{
  "name": "Codex",
  "shell": "/bin/bash",
  "prompt": ["coder", "architect"],
  "llm_type": "codex",
  "llm_model": "default"
}
```

Prompt behavior:
- `prompt` accepts a single string or array of strings.
- Each string is a prompt name, resolved to `config/prompts/{name}.txt`.
- Prompts are injected in order, with a small delay between each.

## Agent Skills

Agent Skills follow the [agentskills.io](https://agentskills.io) structure and are loaded from `config/skills/`.

Structure:
- `config/skills/<skill-name>/SKILL.md` with YAML frontmatter + Markdown body.
- Optional folders: `scripts/`, `references/`, `assets/` (files are listed by the API).

Frontmatter fields:
- `name` (required, lowercase with hyphens; must match folder name)
- `description` (required, short summary)
- `license` (recommended)
- `compatibility` (recommended)
- `metadata` (optional map)
- `allowed_tools` (optional list)

Assign skills to agents by adding a `skills` array:
```
"skills": ["git-workflows", "code-review"]
```

Discovery and activation:
- On startup, skills are loaded and metadata is injected into agent terminals as XML.
- The XML format provides skill discovery information to the LLM:
  ```xml
  <available_skills>
    <skill>
      <name>terminal-navigation</name>
      <description>Terminal navigation shortcuts and safe command patterns.</description>
      <location>config/skills/terminal-navigation/SKILL.md</location>
    </skill>
  </available_skills>
  ```
- Skills are injected before agent prompts when the terminal starts.
- Agents can read the full `SKILL.md` at the provided `<location>` to activate a skill.
- Use scripts/references/assets only from trusted sources.

API:
- `GET /api/skills` (optional `?agent=<id>` filter)
- `GET /api/skills/:name`

Security considerations (future work):
- Scripts in skills are not sandboxed; plan for allowlists or user confirmation before execution.
- Log script execution for auditing and traceability.
- Consider signature verification for skills from external sources.
- Treat skills in `config/skills/` as trusted until stronger controls are added.

## CLI

Current commands:
- `gestalt validate-skill <path>`: Validate a skill directory or `SKILL.md` file.

### gestalt-send CLI Tool

Send stdin to a running agent terminal by agent name or id.

Install:
- `make gestalt-send`
- `make install` (installs to `/usr/local/bin` by default; override with `PREFIX=/path`)

Usage:
- `cat file.txt | gestalt-send Codex`
- `echo "status" | gestalt-send Architect`

Flags:
- `--url`: server URL (default `GESTALT_URL` or `http://localhost:8080`)
- `--token`: auth token (default `GESTALT_TOKEN`)
- `--start`: auto-start the agent if not running, then retry input
- `--verbose`: log request/response details to stderr (token masked)
- `--debug`: include payload preview and unmasked token

Shell completion:
- `gestalt-send completion bash > /etc/bash_completion.d/gestalt-send`
- `gestalt-send completion zsh > ~/.zfunc/_gestalt-send`

Exit codes:
- `0`: success
- `1`: usage error
- `2`: agent not running
- `3`: network or server error

Notes:
- Agent names must be unique and match the `name` field in `config/agents/*.json`.
- If auth is enabled, set `GESTALT_TOKEN` to the same token used by the server.

## License

Copyright (C) 2025-2026 Dyne.org foundation

Designed and written by Denis "[Jaromil](https://jaromil.dyne.org)"
Roio.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful, but
WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public
License along with this program.  If not, see
<https://www.gnu.org/licenses/>.

<p align="center">
  <a href="https://dyne.org">
    <img src="https://files.dyne.org/software_by_dyne.png" width="170">
  </a>
</p>
