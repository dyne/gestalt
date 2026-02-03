# Notify event envelope

All notify clients POST the same envelope to `POST /api/sessions/:id/notify`.
The backend treats `payload` as opaque JSON, while extracting top-level metadata
for validation and Temporal notarization.
Requests that include `agent_id`, `agent_name`, `source`, or `event_type` are rejected.

## Envelope fields

- `session_id` (string, required)
- `occurred_at` (RFC3339 string, optional)
- `payload` (object, required)
- `raw` (string, optional; raw JSON argument from Codex)
- `event_id` (string, optional; stable id for dedupe)

Payload requirements:
- `type` (string, required)

## Example: Codex notify payload

```json
{
  "session_id": "Codex 1",
  "occurred_at": "2026-01-28T20:17:42Z",
  "payload": {
    "type": "agent-turn-complete",
    "turn_id": "turn-42",
    "model": "gpt-5"
  },
  "raw": "{\"type\":\"agent-turn-complete\",\"turn_id\":\"turn-42\",\"model\":\"gpt-5\"}",
  "event_id": "codex-notify:turn-42"
}
```

## Example: Plan event

```json
{
  "session_id": "Codex 1",
  "occurred_at": "2026-01-28T20:17:42Z",
  "payload": {
    "type": "plan-L1-wip",
    "plan_file": ".gestalt/plans/gestalt-notify-temporal.org",
    "heading": "Notarize session events via gestalt-notify",
    "state": "wip",
    "level": 1
  },
  "event_id": "manual:plan-L1-wip:gestalt-notify-temporal"
}
```
