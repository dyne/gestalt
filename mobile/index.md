# Gestalt Mobile

Gestalt Mobile is a mobile-first web relay for durable Codex development
sessions. It presents sessions, plans, approvals, skill profiles, workspace
selection, and bounded Git operations in a browser while Codex runs on your
machine.

<div class="mobile-shot-grid">
  <MobileScreenshot
    src="03-workspace-selection.png"
    alt="Sessions tab with the gestalt-mobile repository selected in the workspace tree."
    caption="Choose a bounded workspace, then configure the new Codex session."
    eager
  />
  <MobileScreenshot
    src="07-chat.png"
    alt="Chat tab with a user request and completed Gestalt Mobile response."
    caption="Continue the durable session from Chat, with commentary and final answers kept in one timeline."
  />
</div>

## The journey at a glance

1. Enroll the first trusted device with a passkey.
2. Select a workspace, skill profile, sandbox, and approval policy.
3. Work in Chat and answer approvals or bounded questions in context.
4. Follow supervised milestones in Plan.
5. Inspect safe repository actions in Git and keep local notes in Scratchpad.
6. Add, rename, or revoke authorized devices as your setup changes.

[See every Gestalt Mobile screen in the screenshot gallery →](./gallery)

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

## Primary navigation

The bottom navigation keeps four primary views within thumb reach:

- **Sessions** selects workspaces and starts or reopens Codex threads.
- **Git** inspects repositories and exposes only bounded pull, push, and clone
  operations.
- **Chat** carries the active prompt, response, approval, and user-input
  timeline.
- **Plan** presents supervised milestones and review status for the selected
  session.

Scratchpad, appearance, device management, and relay locking are available
from configuration.

<MobileScreenshot
  src="06-configuration.png"
  alt="Gestalt Mobile configuration popover over the Sessions tab."
  caption="Configuration keeps secondary tools and security actions available without crowding the primary tabs."
/>
