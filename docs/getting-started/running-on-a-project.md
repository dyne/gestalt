# Running on a project

Run `gestalt` from your project root so the working directory is the context for sessions and prompt includes.

## `.gestalt/` extraction

Gestalt uses `.gestalt/` in the project root for runtime data and extracted defaults. This includes plans under `.gestalt/plans/*.org`.

## Authentication

Set `GESTALT_TOKEN` to enforce authentication for REST, WebSocket, and SSE routes.

- REST: send `Authorization: Bearer <token>`.
- WS/SSE: use `?token=<token>` when headers are unavailable.

## Plans

Place plan files in `.gestalt/plans/*.org`.
