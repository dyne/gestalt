# HTTP API reference

This reference reflects the current backend routes in `internal/api/routes.go`.

## Authentication

If `GESTALT_TOKEN` is unset, auth is disabled.

If `GESTALT_TOKEN` is set:

- REST: send `Authorization: Bearer <token>`
- WebSocket/SSE: either `Authorization: Bearer <token>` or `?token=<token>`

## REST endpoints

### Status and metrics

- `GET /api/status`
- `GET /api/metrics/summary`
- `GET /api/git/log`

### Sessions

- `GET /api/sessions`
- `POST /api/sessions`
- `DELETE /api/sessions/:id`
- `GET /api/sessions/:id/output`
- `POST /api/sessions/:id/input`
- `POST /api/sessions/:id/activate`
- `GET /api/sessions/:id/history`
- `GET /api/sessions/:id/input-history`
- `POST /api/sessions/:id/input-history`
- `POST /api/sessions/:id/bell`
- `POST /api/sessions/:id/notify`

### Agents and skills

- `GET /api/agents`
- `POST /api/agents/:name/send-input`
- `GET /api/skills`

### Plans

- `GET /api/plans`

### Flow configuration

- `GET /api/flow/activities`
- `GET /api/flow/event-types`
- `GET /api/flow/config`
- `PUT /api/flow/config`
- `GET /api/flow/config/export`
- `POST /api/flow/config/import`

### OpenTelemetry

- `POST /api/otel/logs`
- `GET /api/otel/traces`
- `GET /api/otel/metrics`

## WebSocket endpoints

- `GET /ws/session/:id` (terminal stream)
- `GET /ws/logs`
- `GET /ws/events` (filesystem events)
- `GET /api/agents/events`
- `GET /api/sessions/events`
- `GET /api/config/events`

## SSE endpoints

- `GET /api/events/stream`
- `GET /api/logs/stream`
- `GET /api/notifications/stream`

## Notes

- The canonical session namespace is `/api/sessions/*`.
- `/api/terminals/*` is not part of the current API surface.

## Git log endpoint

`GET /api/git/log`

Query params:

- `limit` (optional): number of commits to return. Default `20`, max `50`.

Behavior:

- Returns `200` with `{"branch":"","commits":[]}` outside a git repository.
- Returns `503` when git is unavailable or the command times out.

Example response:

```json
{
  "branch": "git-log-dashboard",
  "commits": [
    {
      "sha": "0123456789abcdef0123456789abcdef01234567",
      "short_sha": "0123456789ab",
      "committed_at": "2026-02-18T00:00:00Z",
      "subject": "feat(dashboard): add git log panel",
      "stats": {
        "files_changed": 2,
        "lines_added": 45,
        "lines_deleted": 8,
        "has_binary": false
      },
      "files_truncated": false,
      "files": [
        {
          "path": "frontend/src/views/Dashboard.svelte",
          "added": 32,
          "deleted": 7,
          "binary": false
        }
      ]
    }
  ]
}
```
