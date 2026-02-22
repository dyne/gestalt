# Notify event envelope

All notify clients POST the same envelope to `POST /api/sessions/:id/notify`.
The backend treats `payload` as opaque JSON, while extracting top-level metadata
for validation and OTel-backed notification records.
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
    "plan_file": ".gestalt/plans/gestalt-notify.org",
    "heading": "Route session events via gestalt-notify",
    "state": "wip",
    "level": 1
  },
  "event_id": "manual:plan-L1-wip:gestalt-notify"
}
```

## Flow event mapping

Notify payloads are normalized into Flow trigger events.
The canonical event type is derived from `payload.type`:

- `new-plan` -> `notify_new_plan`
- `progress` -> `notify_progress`
- `finish` -> `notify_finish`
- everything else -> `notify_event`

Flow fields include:

- `type`: canonical event type
- `timestamp`: RFC3339 time (from `occurred_at` or server time)
- `session_id` and `session.id`
- `notify.type`: original `payload.type`
- `notify.event_id`: `event_id` when provided
- `notify.<key>` for scalar payload keys (strings/bools/numbers)
- top-level aliases for the same payload keys (for example `summary`, `plan_file`, `task_title`)

Template tokens follow the same keys, for example:
<div v-pre>

`{{summary}}`, `{{plan_file}}`, `{{plan_summary}}`, `{{task_title}}`, `{{task_state}}`,
`{{git_branch}}`, `{{session_id}}`, `{{timestamp}}`,
`{{event_id}}`, `{{notify.summary}}`, `{{notify.type}}`, `{{notify.event_id}}`.
</div>

## OTel log attribute contract

For every successfully parsed notify request (after session and agent validation), the backend emits one `INFO` log entry with message `notify event accepted`.
This entry is visible through `/api/logs` and `/api/logs/stream`.

Stable attributes:

- `gestalt.category=notification`
- `gestalt.source=notify`
- `type` (canonical notify event type)
- `notify.type` (original payload `type`)
- `notify.event_id` (when provided)
- `session.id` and `session_id`
- `notify.dispatch` (`queued`, `flow_unavailable`, `temporal_unavailable`, or `failed`)
- scalar payload aliases as both top-level keys and `notify.<key>`

Guarantee:

- Logging is additive and best-effort (no API response changes).
- If a notify request is accepted but dispatch later degrades (for example Temporal unavailable), the notify log entry is still emitted with `notify.dispatch` set to the outcome.

## Example: Flow automation for a new plan

```json
{
  "version": 1,
  "triggers": [
    {
      "id": "plan-created",
      "label": "New plan created",
      "event_type": "notify_new_plan",
      "where": {
        "session.id": "Codex 1"
      }
    }
  ],
  "bindings_by_trigger_id": {
    "plan-created": [
      {
        "activity_id": "toast_notification",
        "config": {
          "level": "info",
          "message_template": "{{summary}}"
        }
      },
      {
        "activity_id": "spawn_agent_session",
        "config": {
          "agent_id": "coder",
          "message_template": "New plan summary: {{notify.plan_summary}}"
        }
      }
    ]
  }
}
```
