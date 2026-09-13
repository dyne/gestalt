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

## Supervision compatibility

Before launching Codex or Mobile, `gestalt doctor` reads two small local release
manifests: the Agents marketplace manifest and Mobile's installed package
manifest. The probe is offline and read-only. It checks supervision contract
version 1 and the capabilities needed for `supervision-start`, wait and
checkpoint tools, stuck-agent capacity recovery, controller status, canonical
agent identity, session verdicts, acknowledgement-safe drafts, and the Org Plan
contract. It never opens a session, prompt, Org Plan body, transcript,
credential store, or lease ID.

`ready (v1; offline manifests)` means the installed components agree. `UNAVAILABLE`
means a release did not publish its manifest; `INCOMPATIBLE` means a manifest is
malformed, stale, or lacks a named capability. Both have one recovery action:

```sh
gestalt update
# then restart Gestalt Mobile
```

The manager continues its normal checksum-verified update path; doctor never
downloads or silently changes a component.
