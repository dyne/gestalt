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
- Sessions appear in `/api/sessions` and are opened from Dashboard agent controls.

## Accepted differences
- External sessions are backed by backend-managed tmux windows (one per session)
  inside `Gestalt <workdir>`.
- A single backend PTY "agents hub" can attach to that tmux session for the UI.
- External sessions remain non-interactive in per-session GUI terminal tabs.

## Enforced by
- API contract for `/api/sessions`, `/api/sessions/:id/input`, and `/api/sessions/:id/activate`.
- Backend tests that compare external and server session behavior for IDs,
  prompt file lists, and Codex notify injection.
