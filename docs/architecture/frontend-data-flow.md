# Frontend Data Flow and Store Ownership

This document summarizes how data moves through the frontend and which modules own which responsibilities.

## Data flow overview
- Views (App.svelte, Dashboard.svelte, ChatView.svelte, PlanView.svelte, LogsView.svelte, FlowView.svelte) fetch initial data with `apiFetch`.
- Event WebSockets fan out through event stores, which dispatch typed payloads to subscribed views.
- Session UI uses `terminalStore` for per-session state, which owns xterm setup, socket lifecycle, and history replay.
- Director prompt flow uses `sendDirectorPrompt` to start-or-reuse the Director session, send input, then emit notify trigger metadata (`prompt-text` or `prompt-voice`).
- Director dialogue bubbles are owned by `directorChatStore`, which aggregates streamed terminal output into assistant messages and suppresses echoed prompt text.

## Store ownership
- `frontend/src/lib/terminalStore.js`
  - Owns per-session state keyed by session ID.
  - Delegates to `terminal/xterm.js` for xterm setup, `terminal/socket.js` for WS/history, and `terminal/input.js` for input + touch scroll helpers.
  - Exposes status, history status, bell count, reconnect, scroll state, and input helpers.
- `frontend/src/lib/directorChatStore.js`
  - Owns Director chat state (`sessionId`, `messages`, `streaming`, `error`) for the home Chat experience.
  - Appends user messages on successful sends and aggregates assistant chunks from the Director session stream.
  - Handles assistant finalization after output idle debounce and disconnect/dispose lifecycle for the stream connection.
- `frontend/src/lib/eventStore.js`
  - Filesystem event stream (`/ws/events`) with subscription message support.
- `frontend/src/lib/agentEventStore.js`
  - Agent lifecycle events (`/api/agents/events`).
- `frontend/src/lib/terminalEventStore.js`
  - Session lifecycle events (`/api/sessions/events`).
- `frontend/src/lib/configEventStore.js`
  - Config change events (`/api/config/events`).
- `frontend/src/lib/workflowEventStore.js`
  - Workflow events (`/api/workflows/events`).
- `frontend/src/lib/notificationStore.js`
  - Toast queue and dismissal logic for global notices.

## Component responsibilities
- Views own data shaping and local UI state, not transport logic.
- Stores own WebSocket lifecycle, reconnection, parsing, and fan-out.
- Components should avoid creating new WebSocket connections directly; use stores instead.
- Dashboard and Chat share `DirectorComposer.svelte`; App owns the submit handler so both views use the same send path.

## Common patterns
- Fetch-on-mount for baseline state; subscribe to event stores for updates.
- When adding a new long-lived stream, introduce a store that owns its socket lifecycle and exposes a `subscribe` API.
- When adding a session-facing feature, keep xterm and socket changes inside `terminalStore` or its helper modules.
