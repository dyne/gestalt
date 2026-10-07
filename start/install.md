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

`gestalt xerj probe` is a read-only readiness operation for startup consumers.
It prints one JSON object on stdout and returns zero for optional absence or
unavailability as well as readiness. Schema version 1 has `status` equal to
`absent`, `unavailable` (with a bounded `reason`), or `ready`. Ready includes
the pinned `version`, normalized loopback `endpoint`, managed `binary` and
`dataRoot`, `protocolVersion`, and authentication references (`auth.environment`
and `auth.keyFile`), never a key or authorization header. Consumers must inspect
`status`; an exit code alone does not indicate usable retrieval. Invalid command
usage or a missing Node prerequisite still returns a nonzero manager error.

The probe checks the managed executable's exact version, the native MCP
`2025-03-26` initialize response and retrieval tool schemas, then authenticated
`GET /_cluster/health` and `GET /_cat/indices?format=json&h=index`. Those pinned
rc.87 handlers read engine state and accept an empty index list. MCP discovery
alone does not reach the backend. The probe deliberately avoids `_search`,
which appends an audit entry, and `xerj_map`, which refuses a missing autoindex
catalog. Empty readiness does not imply any repository has been indexed.

`XERJ_READY_TIMEOUT_MS` sets the whole probe budget, including detection, MCP
startup, HTTP reads and cleanup (default 5000; allowed range 100–60000). Probe
processes are reaped on timeout, EOF failure or interruption. The operation
creates no managed files, starts no backend, installs nothing, and never
indexes or downloads a model. Native diagnostics are suppressed to keep
credentials off both output streams. It uses the manager's existing Node
runtime and preserves the single-file checksum-verified distribution.

For a shared backend, use `gestalt xerj ensure-ready`, `gestalt xerj status`,
and `gestalt xerj stop` with the same `GESTALT_HOME` and `XERJ_URL`. Ensure uses
the same readiness schema and deadline, including time waiting for another
startup. A ready result also reports `ownership`: `managed` for a backend
retained by this manager, or `adopted` for a compatible existing endpoint.
Status rechecks readiness and ownership without changing managed files. Stop
returns schema-1 `status: "stopped"` only after its owned server is reaped;
otherwise it reports bounded unavailability, such as `not-managed`.

Ensure serializes startup and explicit stop using private records under
`runtime/xerj/.lifecycle`, scoped to the normalized endpoint. It reuses a
verified endpoint, never replaces an unrelated listener, and starts only the
installed pinned binary. Automatic server launches bind to loopback, retain
the native authentication default and managed admin key, and use lexical
embedding mode so readiness cannot trigger model downloads. No installation,
indexing, project configuration, profile edits or service registration occurs.

A private manager owner retains the actual server child and accepts
authenticated local control requests. CLI or Mobile exit and MCP proxy EOF
leave that shared backend running. Explicit managed stop closes the owned
server; it never signals a PID read from a saved record or stops an adopted
instance. Startup records include the lock owner's process birth identity;
only a proven dead or reused owner permits stale-lock recovery. Unknown or
incomplete lock ownership remains unavailable rather than being deleted.
An attempted launch expires and is reaped unless readiness succeeds and the
caller retains it. A later explicit ensure can restart a stopped backend.
This is an on-demand process lifetime, with no login autostart or system service.

Lifecycle directories have mode 0700; records, control sockets and event logs
have mode 0600. Event logs contain only manager event codes, are capped at
32 KiB per endpoint, and exclude native output and credentials. The default
five-second budget remains a readiness limit; an explicit longer warmup is
separate work. Managed `gestalt cli` startup also binds retrieval tools and
`gestalt:xerj` to the same verified capability. It reads native Codex
configuration for the selected working directory, reads a selected native
profile file without writing it, calls
ensure-ready, and supplies ephemeral `-c` session overrides. No persistent
Codex or project configuration is written. The installed plugin must contain
the canonical xerj skill; native configuration is verified against Codex
`0.160.0`.
The self-contained manager embeds the BSD-licensed `smol-toml` parser for
profile-v2 files (`$CODEX_HOME/<name>.config.toml`). The adapter follows the
native base-user, profile, project, then session precedence for retrieval
settings; Codex loads the original profile normally for the launch. Native
skill selectors and hook trust remain in their original user/profile layers.
Only existing session selectors/hooks and the managed capability are passed as
session overrides. Profile project trust is checked through native discovery.
The parser bundle can be regenerated with `node scripts/bundle-profile-parser.mjs`.

The `gestalt-xerj` stdio entry uses the absolute manager executable, native
`required = true`, and `startup_readiness = "connection"`, so a cached catalog
or preliminary probe cannot bypass the runtime's live MCP startup before the
first turn. One deadline covers prerequisite native config/profile/hook
discovery, shared backend readiness, and the actual MCP connection. Each step
uses only the remaining budget; exhaustion launches without xerj. The deadline
does not limit the interactive session after connection. If native xerj startup
fails, the launcher retries once with both its MCP entry and skill disabled.
Authentication stays in the managed key file/inherited environment, not argv.
Only the three audited retrieval tools (`xerj_search`, `xerj_map`, and
`xerj_code_search`) are exposed and approved through native MCP tool policy;
this keeps them callable under `approval_policy = "never"` without approving
indexing or other native xerj tools.
Existing unrelated MCP entries and skill selectors are preserved; duplicate
entries invoking this same managed proxy are disabled for the session.
Conflicting reserved-server authentication or tool filters make xerj unavailable
rather than supplying misleading guidance.

A ready runtime enables xerj even when its saved skill selector disables it;
absence or failure disables it even when a saved selector enables it. Native
SessionStart and SubagentStart hooks supply the canonical skill's concise
instructions, including when catalog limits omit its discovery entry. The
launcher reads the generated handlers' native trust identities and trusts only
those two pure-output commands through ephemeral session configuration. Existing
hooks and trust settings remain intact. Unavailable runtimes supply a bounded
notice that earlier retrieval guidance is inactive. Resume retains conversation
history; the current catalog, tool registry, and capability notice reflect
current availability. Codex hooks must already be enabled (`features.hooks =
true`); disabled hooks or an explicit global skill-instruction-disable setting
make this optional capability unavailable.
Native child agents inherit the effective configuration and tool catalog. Codex
opens a separate stdio client when a child first calls inherited retrieval; the
managed backend stays shared. Live first-turn connection gating applies to the
managed CLI launch; child connection errors use the inherited direct-source
fallback guidance. A new
CLI invocation, including resume, repeats readiness. A tool failure after a
turn begins uses direct-source fallback; supplied instructions are not
retroactively removed. Explicit remote Codex endpoints use their own server's
configuration and capability policy; the manager does not inject its local
xerj connection there. Help and configuration-management commands do not start
retrieval. Mobile receives the absolute `GESTALT_MANAGER_BIN` discovery
reference and must recheck capability at its own runtime boundaries.

The native startup regression suite uses a pinned Codex development dependency,
private plugin/configuration fixtures, and a local fake model endpoint. It
checks the first request, a real MCP call, and a separately observed child
request without calling a production model service. The configuration and
inheritance audit uses OpenAI's
[Codex 0.160.0 source](https://github.com/openai/codex/tree/rust-v0.160.0),
including `config/src/skills_config.rs`, `core/src/agent/child_config.rs`, and
`codex-mcp/src/connection_manager/required.rs` under `codex-rs`.

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
