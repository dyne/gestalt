# Notify event envelope

All notify sources POST the same envelope to `POST /api/terminals/:id/notify`.
The backend treats `payload` as opaque JSON, while extracting top-level metadata
for validation and Temporal notarization.

## Envelope fields

- `terminal_id` (string, required)
- `agent_id` (string, required; agent config id)
- `agent_name` (string, optional; display name)
- `source` (string, required; `codex-notify` or `manual`)
- `event_type` (string, required)
- `occurred_at` (RFC3339 string, optional)
- `payload` (object, optional)
- `raw` (string, optional; raw JSON argument from Codex)
- `event_id` (string, optional; stable id for dedupe)

## Example: Codex notify payload

```json
{
  "terminal_id": "term-7",
  "agent_id": "codex",
  "agent_name": "Codex",
  "source": "codex-notify",
  "event_type": "agent-turn-complete",
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
  "terminal_id": "term-7",
  "agent_id": "architect",
  "agent_name": "Architect",
  "source": "manual",
  "event_type": "plan-L1-wip",
  "occurred_at": "2026-01-28T20:17:42Z",
  "payload": {
    "plan_file": ".gestalt/plans/gestalt-notify-temporal.org",
    "heading": "Notarize session events via gestalt-notify",
    "state": "wip",
    "level": 1
  },
  "event_id": "manual:plan-L1-wip:gestalt-notify-temporal"
}
```
