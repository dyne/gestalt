# External Session Contract

This document defines what "external session" means for Gestalt and what must
match GUI-created sessions.

## Shared invariants
- Session creation uses the same agent TOML parsing and validation rules.
- Session IDs follow the same generation and singleton rules.
- Prompt rendering uses the same include, port, and session-id substitutions.
- Codex sessions inject `notify = ["gestalt-notify","--session-id",<id>]` the
  same way as GUI-created sessions.
- Workflow start/stop behavior is backend-owned and identical.
- Sessions appear in `/api/sessions` and open a UI tab.

## Accepted differences
- External sessions are not backed by a backend PTY/process; the agent process
  lifecycle is owned by `gestalt-agent` via tmux.
- The backend can only resize, send input, and receive output via the runner
  bridge for external sessions.

## Enforced by
- API contract for `/api/sessions` and the runner bridge endpoints.
- Backend tests that compare external and server session behavior for IDs,
  prompt file lists, and Codex notify injection.
