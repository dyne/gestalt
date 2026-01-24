# Frontend Data Flow and Store Ownership

This document summarizes how data moves through the frontend and which modules own which responsibilities.

## Data flow overview
- Views (App.svelte, Dashboard.svelte, PlanView.svelte, LogsView.svelte, FlowView.svelte) fetch initial data with `apiFetch`.
- Event WebSockets fan out through event stores, which dispatch typed payloads to subscribed views.
- Terminal UI uses `terminalStore` for per-terminal state, which owns xterm setup, socket lifecycle, and history replay.

## Store ownership
- `frontend/src/lib/terminalStore.js`
  - Owns per-terminal state keyed by terminal ID.
  - Delegates to `terminal/xterm.js` for xterm setup, `terminal/socket.js` for WS/history, and `terminal/input.js` for input + touch scroll helpers.
  - Exposes status, history status, bell count, reconnect, scroll state, and input helpers.
- `frontend/src/lib/eventStore.js`
  - Filesystem event stream (`/ws/events`) with subscription message support.
- `frontend/src/lib/agentEventStore.js`
  - Agent lifecycle events (`/api/agents/events`).
- `frontend/src/lib/terminalEventStore.js`
  - Terminal lifecycle events (`/api/terminals/events`).
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

## Common patterns
- Fetch-on-mount for baseline state; subscribe to event stores for updates.
- When adding a new long-lived stream, introduce a store that owns its socket lifecycle and exposes a `subscribe` API.
- When adding a terminal-facing feature, keep xterm and socket changes inside `terminalStore` or its helper modules.
