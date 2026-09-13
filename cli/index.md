# Gestalt CLI

`gestalt` is the small manager that keeps the Agents plugin profile and Mobile
relay together without merging their state. Run `gestalt help` at any time.

## Commands

| Command | Effect |
| --- | --- |
| `gestalt install` | Install or reconcile Agents, context mode, profiles, and Mobile |
| `gestalt update` | Checksum-update the manager, upgrade Agents, rerun setup, and update Mobile |
| `gestalt update-restart` | From a live Mobile session, update everything and gracefully restart Mobile |
| `gestalt cli [args…]` | Launch Codex with the isolated Gestalt home |
| `gestalt mobile [args…]` | Launch Gestalt Mobile and forward its options |
| `gestalt doctor` | Check prerequisites, paths, Gestalt plugin version, and Mobile version |
| `gestalt version` | Print the manager version |
| `gestalt help` | Print command help |

## Launch Codex

```sh
gestalt cli
gestalt cli --help
```

Before launch, the command verifies that the prepared context-mode runtime and
native MCP bridge match the installed plugin version. It exports
`CODEX_HOME=~/.codex-gestalt` and `GESTALT_HOME=~/.gestalt` only for the child
process. Your current shell and default Codex profile are unchanged.

The `context-mode@dyne-gestalt-agents` plugin source is intentionally shown as
disabled. Gestalt Agents keeps that package as the implementation source and
registers one native `context-mode` MCP launcher instead. Enabling the plugin
by hand can create a second, incorrectly launched MCP server.

## Launch Mobile

```sh
gestalt mobile --cwd "$HOME/devel"
gestalt mobile --cwd "$HOME/devel" --port 3000
gestalt mobile --cwd "$HOME/devel" --skills focused
```

Every option after `mobile` is passed to `gestalt-mobile`. See [network
deployment](../mobile/deployment.md) before using a non-loopback listener.

## Refresh the installation

```sh
gestalt update
```

This upgrades `dyne/gestalt-agents`, reruns its required setup script, verifies
the context-mode runtime and complete `$gestalt:*` app-server skill catalog,
updates the stable `$CODEX_HOME/bin/org-plan` helper, and installs
`gestalt-mobile@latest` under the Gestalt home. Start a new session after the
update because running sessions retain their startup catalog.

When you are already working in a session opened through Gestalt Mobile, use:

```sh
gestalt update-restart
```

The command schedules the update outside the current Codex process. Mobile
continues running if any update step fails. After a successful update it shuts
down gracefully, closes its Codex children, and restarts with the same working
directory and command-line options. The browser reconnects to the relay and the
durable sessions remain available. Progress is written to
`$GESTALT_HOME/update-restart.log`.

The command is intentionally available only to sessions launched by
`gestalt mobile`; it will not guess at or terminate an unrelated process.

To opt into the curated third-party skill set maintained by Gestalt Agents:

```sh
gestalt update --extra-skills
```
