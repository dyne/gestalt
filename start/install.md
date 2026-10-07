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

## Managed xerj

Install xerj explicitly; the ordinary Gestalt installer does not start it or
modify shell, editor, agent, or project configuration:

```sh
gestalt xerj install
gestalt xerj --version
gestalt xerj --insecure --port 9300
gestalt xerj autoindex /absolute/path/to/project --url http://localhost:9300
gestalt xerj search 'query with spaces' --url http://localhost:9300
gestalt xerj mcp --url http://localhost:9300
```

The manager installs the official Linux static-musl or macOS release
`1.0.0-rc.87`, verifies its published SHA-256, and atomically replaces
`$GESTALT_HOME/xerj/xerj`. Defaults are `~/.gestalt/xerj` for installation and
`~/.gestalt/runtime/xerj` for persistent data, configuration, and caches.
Downloads and staging stay in a private install directory which is removed on
success or failure. A failed download, checksum or extraction preserves the
previous executable. An explicit install repeats the verified replacement;
other upstream versions require a new manager audit.

Forwarding supports the server, `index`, `autoindex` (including map/status),
`search`, `def`, `gain`, and the native `mcp` stdio proxy. The server and `index` receive `--data-dir` and a
managed config; autoindex receives its actual supported `--state-dir`. Automatic
neural-model cache paths are not placed in the user home: model-cache, XDG and
temporary paths point into the runtime root. `HOME` is preserved. Connection
settings `XERJ_URL`, `XERJ_API_KEY`, logging and feedback settings are preserved;
other inherited environment settings are excluded from the child process.
Argument boundaries, streams, signals and exit codes pass through directly.
Use the pinned CLI's split option form (`--port 9300`, not `--port=9300`);
an optional initial `gestalt xerj --` separates manager dispatch from arguments.
Input folders and NDJSON files can be outside the managed roots and are read
without writing project configuration.

Server listeners bind explicitly to `127.0.0.1`; `--bind ::1` selects IPv6
loopback. Public/LAN bindings are rejected. Client URLs must use HTTP and an
exact `127.0.0.1`, `localhost` or `[::1]` host, with an optional valid port and
trailing slash. `localhost` is converted to `127.0.0.1` before execution to avoid
DNS ambiguity. HTTPS, credentials in URLs, alternate host spellings, paths,
queries and fragments are refused. This service is intended for local agents.

`gestalt xerj mcp` keeps stdout exclusively for native JSON-RPC and exits at
stdin EOF. It proxies to the local HTTP node; the manager installs no agent
configuration or bridge. `XERJ_AUTH`/`--auth` carry the full Authorization header
for MCP, while ordinary clients use `XERJ_API_KEY`. Without an explicit key,
the wrapper reads only its managed `runtime/xerj/admin.key`; MCP never falls
back to guessed keys in the working directory or home. With no managed key,
it sends a fixed unavailable-key marker, so authenticated calls fail visibly
and an explicitly insecure local node can still serve them. MCP's code-cache
path is also routed under the managed runtime root.

Conflicting install/data/state paths, custom config files, diagnostic-output
and worker-executable overrides are rejected before execution. Managed trees
must contain no symlinks or multiply linked files; configured root aliases are resolved before checking
descendants. `init` writes agent/editor/project files; `brain` and `share` spawn
auxiliary services; `feedback` can write reports and open PRs; `code`/`corpus`
clone repositories and invoke Git. These commands, service/update/export
commands and unaudited flags are refused by this wrapper.

This is automatic state routing and path validation, not an OS filesystem
sandbox. In the pinned server, an administrator can supply a snapshot repository
destination through the HTTP API, which can write outside these roots. The
wrapper does not block that API. Concurrent filesystem changes and manually
replaced binaries are also outside this guarantee.
The managed config retains upstream's default transport (TLS disabled, API-key
authentication enabled) and routes certificate paths under the data root.
Custom server configuration is currently refused rather than accepted with
unverified persistent destinations.

Audit basis: [xerj rc.87 source](https://github.com/xerj-org/xerj/tree/fcb73c1c725cf6532cb73e556c51e0388a791533),
especially `xerj-server/src/main.rs`, `xerj-autoindex/src/cli.rs`, `state.rs`,
`init.rs`, `xc.rs`, `feedback.rs`, `xerj-ai/src/neural.rs`, and snapshot handling
in `xerj-api/src/es_compat.rs` under `engine/crates`.
