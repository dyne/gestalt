# CLI configuration

The manager has conservative defaults and uses environment variables for
repeatable overrides.

| Variable | Default | Purpose |
| --- | --- | --- |
| `CODEX_HOME` | `~/.codex-gestalt` | Isolated Codex profile |
| `GESTALT_HOME` | `~/.gestalt` | Runtime, Mobile package, and skill profile root |
| `GESTALT_MARKETPLACE` | `dyne/gestalt-agents` | Marketplace source passed to Codex |
| `GESTALT_MARKETPLACE_NAME` | `dyne-gestalt-agents` | Codex checkout directory name |
| `GESTALT_MOBILE_VERSION` | `latest` | npm version or tag installed by the manager |
| `GESTALT_BIN_DIR` | `~/.local/bin` | Installer destination for the manager |
| `GESTALT_INSTALL_BASE_URL` | `https://dyne.github.io/gestalt` | Manager install and self-update source |

## Alternate isolated profile

```sh
CODEX_HOME="$HOME/.codex-gestalt-lab" gestalt install
CODEX_HOME="$HOME/.codex-gestalt-lab" gestalt cli
```

Use the same override on later updates. Paths must be absolute and cannot be
`/` or your home directory.

## Pin Mobile

```sh
GESTALT_MOBILE_VERSION="0.1.0" gestalt update
```

The Agents marketplace follows the version selected by Codex marketplace
upgrade. Gestalt Agents and its adapted context-mode runtime share one release
version.
