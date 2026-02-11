# HTTP API reference

This page summarizes REST, WebSocket, and SSE surfaces exposed by Gestalt.

## REST

- `GET /api/status`
- `GET|POST /api/sessions`
- `DELETE /api/sessions/:id`
- `GET /api/sessions/:id/output`
- `GET|POST /api/sessions/:id/input-history`
- `GET /api/agents`
- `POST /api/agents/:name/input`

## WebSocket

- `GET /ws/session/:id`
- `GET /ws/logs`
- `GET /ws/events`
- `GET /api/agents/events`
- `GET /api/sessions/events`
- `GET /api/config/events`
- `GET /api/workflows/events`

## SSE

- `GET /api/events/stream`
- `GET /api/logs/stream`
- `GET /api/notifications/stream`

## Workflow and metrics endpoints

- `GET /api/workflows`
- `GET /api/sessions/:id/workflow/history`
- `POST /api/sessions/:id/workflow/resume`
- `GET /api/metrics/summary`

## Auth

- REST: `Authorization: Bearer <token>`
- WS/SSE: `?token=<token>` as needed
