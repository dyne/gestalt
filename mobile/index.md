# Gestalt Mobile

Gestalt Mobile is a mobile-first web relay for durable Codex development
sessions. It presents sessions, plans, approvals, skill profiles, workspace
selection, and bounded Git operations in a browser while Codex runs on your
machine.

## Start it

```sh
gestalt mobile --cwd "$HOME/devel"
```

The relay listens on `127.0.0.1:3000` by default and prints its URL. Press
Ctrl-C to stop the HTTP server, active Codex subprocesses, and database cleanly.

## Prerequisites

- Node.js 24 or newer.
- An installed and authenticated Codex CLI.
- The Gestalt isolated profile created by `gestalt install`.

When `codex-profile` and the Gestalt profile both exist, Mobile uses
`codex-profile cli gestalt app-server --stdio`. Otherwise it falls back to the
Codex CLI available in the manager's isolated environment.

## Common options

| Option | Default | Purpose |
| --- | --- | --- |
| `--cwd <path>` | Current directory | Root of selectable workspaces |
| `--host <address>` | `127.0.0.1` | Relay listen address |
| `--port <number>` | `3000` | HTTP port |
| `--public-origin <origin>` | Loopback origin | Exact browser origin for passkeys |
| `--data-dir <path>` | XDG state path | Relay database directory |
| `--skills <profile>` | Project/native selection | Apply a saved global skill profile |
| `--disable-passkey-auth` | Off | Explicit unsafe, unprotected mode |

Run `gestalt mobile --help` for the installed release's authoritative option
list.
