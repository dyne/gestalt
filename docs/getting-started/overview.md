# Gestalt

Gestalt is a local dashboard and API server for running multiple agent sessions in parallel. It gives you one place to start, monitor, and automate coding workflows with terminal streams, flow automations, and event feeds.

The project combines a Go backend, a Svelte frontend, and optional standalone binaries for agent-first workflows. Use this documentation as the source of truth for setup, operation, configuration, and API details.

## Dashboard and Chat Workflow

- The Dashboard home tab is a Director-first entry surface. It provides a shared multiline composer for prompts (typed or voice-transcribed).
- After a successful send, the app switches to the Chat home tab automatically.
- Chat renders the active Director dialogue as role-based bubbles:
  - `user` bubbles are appended immediately on successful send.
  - `assistant` bubbles are assembled from Director session output streaming over the session WebSocket.
- Every successful send also posts a notify event with `payload.type`:
  - `prompt-text` for typed submissions.
  - `prompt-voice` for voice submissions.
- The dedicated Agents tab remains available for direct session management and is not removed by this workflow.

## Quick Setup

```sh
# Install dependencies
npm i
make

# Start Gestalt dashboard
./gestalt
# Opens http://localhost:57417
```
