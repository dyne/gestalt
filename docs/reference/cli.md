# CLI reference

This page covers the primary binaries shipped in release archives.

## `gestalt` (server)

`gestalt` starts the dashboard and HTTP API.

```sh
gestalt
```

Common options:

- `--port` (`GESTALT_PORT`): frontend/dashboard port (default `57417`)
- `--backend-port` (`GESTALT_BACKEND_PORT`): API port (default random)
- `--token` (`GESTALT_TOKEN`): auth token for REST/WS/SSE
- `--dev` (`GESTALT_DEV_MODE`): skip config extraction and use existing config dir
- `--config-dir` (`GESTALT_CONFIG_DIR`): config root (default `.gestalt/config`)

Developer mode note:

- `--dev` does not extract embedded config; `.gestalt/config` must already exist.

## `gestalt-agent` (standalone Codex runner)

`gestalt-agent` runs Codex using an agent profile from `config/agents/*.toml` or `.gestalt/config/agents/*.toml`.

```sh
gestalt-agent <agent-id>
gestalt-agent <agent-id> --dryrun
```

- Agent IDs are filenames without `.toml` (for example `coder`).
- `--host` and `--port` select the server (defaults: `127.0.0.1`, `57417`).
- `--dryrun` prints the resolved tmux attach command without executing it.

## `gestalt-send` (session input client)

`gestalt-send` sends stdin to a running session.

```sh
gestalt-send [options] <agent-name-or-id>
gestalt-send --session-id <session-id>
```

- `--host` and `--port` select the server (defaults: `127.0.0.1`, `57417`).
- `--session-id` accepts both canonical (`Coder 1`) and shorthand (`Coder`) references.
- Agent sends are create-or-reuse: `gestalt-send <agent-name-or-id>` ensures the singleton agent session exists, waits until it is ready, then posts to `POST /api/sessions/:id/input`.

## Agent config and prompts

- Agent profiles are TOML only: `.gestalt/config/agents/*.toml`
- Prompt files resolve in this order: `.tmpl`, `.md`, `.txt`
- Prompt lookup roots: `.gestalt/config/prompts` then `.gestalt/prompts`
- Prompt directives are supported in prompt files:
  - ``&#123;&#123;include filename&#125;&#125;``
  - ``&#123;&#123;port <service>&#125;&#125;``

See [Agent configuration](../configuration/agent-configuration) for full schema and behavior.

## Other binaries

- `gestalt-notify`: send notify payloads to a session (`--session-id` required, `--host`/`--port` server selection)
- `gestalt-otel`: embedded OpenTelemetry collector binary (collector management/debug commands)

## Session API singleton cleanup contract

This compatibility contract defines the intended public behavior for this release line.

| Area | Legacy behavior | Current contract |
| --- | --- | --- |
| Session input API | `POST /api/agents/:name/send-input` and `POST /api/sessions/:id/input` | `POST /api/sessions/:id/input` only |
| Agent sessions | Multiple instances per agent could exist | Exactly one session per agent, canonical id `<AgentName> 1` |
| Agent config `singleton` | Runtime behavior changed when set to `false` | Parse-compatible only; runtime always singleton |
| `gestalt-send` start behavior | `--start` opt-in auto-create | Implicit create-or-reuse; no `--start` |
| Session-id normalization | Tool-specific behavior | Shared rule: explicit `<name> <number>` is honored, otherwise normalize to `<name> 1` |
| `gestalt-notify` server flags | `--url` | `--host` + `--port` |
| CLI non-zero exits | Partially documented | Every non-zero exit prints one actionable stderr message |
