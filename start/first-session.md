# Your first session

## Terminal route

```sh
gestalt cli
```

This is equivalent to launching Codex with
`CODEX_HOME="$HOME/.codex-gestalt"`. In the new session:

1. Run `ctx-doctor` to verify the prepared context-mode runtime.
2. Describe the outcome you want.
3. For substantial, review-gated work, ask for an Org Plan.
4. Review the director's concise milestone updates and final evidence.

## Mobile route

```sh
gestalt mobile --cwd "$HOME/devel"
```

Open the loopback URL printed by the command. Create the first passkey before
placing the relay behind a network endpoint. In the Sessions tab, select a
workspace and model, then start a session.

::: warning Network use requires HTTPS
Gestalt Mobile does not terminate TLS. Do not expose `--host 0.0.0.0` until a
trusted HTTPS reverse proxy or tunnel is configured with the exact
`--public-origin`.
:::

## Stop and resume

Press Ctrl-C to stop the relay cleanly. Durable sessions remain in the relay
database and can be reopened after restart. The isolated Codex profile and
Agents installation remain available to both interfaces.
