# Gestalt CLI

`gestalt` is the small manager that keeps the Agents plugin profile and Mobile
relay together without merging their state. Run `gestalt help` at any time.

## Commands

| Command | Effect |
| --- | --- |
| `gestalt install` | Install or reconcile Agents, context mode, profiles, and Mobile |
| `gestalt update` | Checksum-update the manager, upgrade Agents, rerun setup, and update Mobile |
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

The command exports `CODEX_HOME=~/.codex-gestalt` only for the child process.
Your current shell and default Codex profile are unchanged.

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

To opt into the curated third-party skill set maintained by Gestalt Agents:

```sh
gestalt update --extra-skills
```
