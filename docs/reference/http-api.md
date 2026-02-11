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

### Sessions

- `GET /api/sessions`
- `POST /api/sessions`
- `DELETE /api/sessions/:id`
- `GET /api/sessions/:id/output`
- `GET /api/sessions/:id/history`
- `GET /api/sessions/:id/input-history`
- `POST /api/sessions/:id/input-history`
- `POST /api/sessions/:id/bell`
- `POST /api/sessions/:id/notify`
- `POST /api/sessions/:id/workflow/resume`
- `GET /api/sessions/:id/workflow/history`

### Agents and skills

- `GET /api/agents`
- `POST /api/agents/:name/input`
- `POST /api/agents/:name/send-input`
- `GET /api/skills`

### Plans

- `GET /api/plans`

### Workflows

- `GET /api/workflows`

### Flow configuration

- `GET /api/flow/activities`
- `GET /api/flow/config`
- `PUT /api/flow/config`

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
- `GET /api/workflows/events`

## SSE endpoints

- `GET /api/events/stream`
- `GET /api/logs/stream`
- `GET /api/notifications/stream`

## Notes

- The canonical session namespace is `/api/sessions/*`.
- `/api/terminals/*` is not part of the current API surface.
