# Choose your Gestalt journey

Gestalt combines an agent workflow and a browser relay. Install both once, then
choose the interface that fits the moment.

## 1. Check the requirements

- macOS or Linux with Bash, `curl`, and Git-capable network access.
- Node.js 24 or newer and npm.
- The Codex CLI installed, on `PATH`, and authenticated.
- `python3`, `make`, and a C/C++ compiler for the context-mode native runtime.

## 2. Install

```sh
curl -fsSL https://dyne.github.io/gestalt/install.sh | bash
```

The installer places `gestalt` in `~/.local/bin`, prepares the Agents plugins in
`~/.codex-gestalt`, and installs Mobile under `~/.gestalt/mobile`.

## 3. Pick an interface

::: code-group

```sh [Terminal]
gestalt cli
```

```sh [Browser relay]
gestalt mobile --cwd "$PWD"
```

:::

## 4. Make the first check

In a new terminal session, run:

```sh
gestalt doctor
```

Then start Codex and ask it to run `ctx-doctor`. Continue with [your first
session](./first-session.md).
