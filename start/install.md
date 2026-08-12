# Install Gestalt

The installer is a small Bash bootstrapper. It downloads the versioned manager
script, verifies its SHA-256 checksum, installs it atomically in a user-owned
directory, and runs `gestalt install`.

## One-line install

```sh
curl -fsSL https://dyne.github.io/gestalt/install.sh | bash
```

If `~/.local/bin` is not on `PATH`, the installer prints the exact export to add
to your shell profile.

## What changes on disk

| Path | Purpose |
| --- | --- |
| `~/.local/bin/gestalt` | Manager CLI |
| `~/.codex-gestalt` | Isolated Codex home, plugins, and generated agent profiles |
| `~/.gestalt/runtime` | Prepared context-mode runtime |
| `~/.gestalt/mobile` | User-local Gestalt Mobile npm installation |
| `~/.gestalt/skill-profiles` | Optional Mobile skill profiles |

Your normal `~/.codex` profile is not modified.

## Inspect before running

```sh
curl -fsSL https://dyne.github.io/gestalt/install.sh -o /tmp/gestalt-install.sh
less /tmp/gestalt-install.sh
bash /tmp/gestalt-install.sh
```

The published assets are also available directly: [installer](/install.sh)
and [manager script](/gestalt).

## Customize locations

```sh
GESTALT_BIN_DIR="$HOME/bin" \
CODEX_HOME="$HOME/.codex-gestalt" \
GESTALT_HOME="$HOME/.gestalt" \
  bash /tmp/gestalt-install.sh
```

`CODEX_HOME` and `GESTALT_HOME` must be absolute, non-root paths. The manager
refuses unsafe values.

## Install without running setup

Use this when you only want to stage the manager:

```sh
bash /tmp/gestalt-install.sh --no-setup
```

Later, run `gestalt install`.
