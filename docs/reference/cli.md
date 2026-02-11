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
- `--temporal-dev-server` (`GESTALT_TEMPORAL_DEV_SERVER`): auto-start Temporal dev server (default true)
- `--temporal-enabled` (`GESTALT_TEMPORAL_ENABLED`): enable workflow integration (default true)
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
- `--dryrun` prints the full command without executing it.

## Agent config and prompts

- Agent profiles are TOML only: `.gestalt/config/agents/*.toml`
- Prompt files resolve in this order: `.tmpl`, `.md`, `.txt`
- Prompt lookup roots: `.gestalt/config/prompts` then `.gestalt/prompts`
- Prompt directives are supported in prompt files:
  - ``&#123;&#123;include filename&#125;&#125;``
  - ``&#123;&#123;port <service>&#125;&#125;``

See [Agent configuration](../agent-configuration) for full schema and behavior.

## Other binaries

- `gestalt-send`: send stdin to an existing agent session (`--start` can auto-create the session)
- `gestalt-notify`: send notify payloads to a session workflow (`--session-id` required)
- `gestalt-otel`: embedded OpenTelemetry collector binary (collector management/debug commands)
