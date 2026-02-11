# Running on a project

Run `gestalt` from the repository you want to work on. Gestalt uses the current working directory as the project context for plans, config, sessions, and filesystem events.

## `.gestalt/` extraction behavior

On startup, Gestalt automatically prepares `.gestalt/config` (unless `--dev` / `GESTALT_DEV_MODE=true` is set). In dev mode, config extraction is skipped and the config directory must already exist.

The project-local `.gestalt/` directory also contains runtime data such as:

- `.gestalt/plans/*.org`
- `.gestalt/sessions/`
- `.gestalt/input-history/`

## Authentication with `GESTALT_TOKEN`

Set `GESTALT_TOKEN` to protect API endpoints.

```sh
export GESTALT_TOKEN=\"replace-with-a-secret-token\"
gestalt
```

Token validation behavior:

- REST: `Authorization: Bearer <token>`
- WebSocket and SSE: `Authorization: Bearer <token>` or `?token=<token>`

Examples:

```sh
curl -H \"Authorization: Bearer $GESTALT_TOKEN\" http://localhost:57417/api/status
```

```txt
ws://localhost:57417/ws/events?token=<token>
http://localhost:57417/api/events/stream?token=<token>
```

## Plans directory

Place plans in `.gestalt/plans/*.org`. Gestalt watches this directory and emits filesystem events when plan files change.
