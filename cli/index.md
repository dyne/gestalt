# Gestalt CLI

`gestalt` is the small manager that keeps the Agents plugin profile and Mobile
relay together without merging their state. Run `gestalt help` at any time.

## Commands

| Command | Effect |
| --- | --- |
| `gestalt install` | Install or reconcile Agents, context mode, profiles, and Mobile |
| `gestalt update [COMPONENT]` | Update Gestalt core and installed optional components; optionally select one component group |
| `gestalt update-restart` | From a live Mobile session, update everything and gracefully restart Mobile |
| `gestalt cli [args…]` | Launch Codex with the isolated Gestalt home |
| `gestalt serena install` | Prepare the optional managed Serena release |
| `gestalt serena update [VERSION]` | Validate and activate a Serena update |
| `gestalt serena version` | Print the installed Serena version without startup |
| `gestalt serena doctor [--cwd ROOT] [--json]` | Check install, connection, and optional project language readiness |
| `gestalt serena index --cwd ROOT` | Explicitly warm native project caches |
| `gestalt serena mcp --cwd ROOT` | Start Serena's workspace MCP connection over stdin/stdout |
| `gestalt serena help` | Show Serena installation and startup commands |
| `gestalt mobile [args…]` | Launch Gestalt Mobile and forward its options |
| `gestalt path [NAME\|--json]` | Resolve managed executable, plugin, and runtime paths |
| `gestalt doctor` | Check prerequisites, paths, Gestalt plugin version, and Mobile version |
| `gestalt version` | Print the manager version |
| `gestalt skills-export [ARCHIVE]` | Export installed skills and skill profiles to a portable `.tar.gz` archive |
| `gestalt skills-import ARCHIVE` | Merge a validated skills archive into this installation |
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

## Optional managed browser

Install the package explicitly and provide an existing Chrome installation:

```sh
gestalt playwright install
gestalt playwright doctor --json
```

For native Codex profile-v2, create `~/.codex-gestalt/browser.config.toml`
(or `$CODEX_HOME/browser.config.toml` when using another Gestalt Codex home):

```toml
[mcp_servers.gestalt-playwright]
enabled = true
default_tools_approval_mode = "prompt"
```

Then select it with `gestalt cli -p browser -C /absolute/project`. This named
profile format is exercised against Codex 0.160.0. Gestalt supplies the managed
launcher for the session; this sparse example is intended for `gestalt cli`.
No selected profile, an absent enabled setting, legacy profile syntax, or an
effective explicit `enabled = false` leaves managed browser access off. Native
project/session override precedence remains in effect. Missing skills, package,
Chrome or supported runtime interfaces leave the capability unavailable.

Opt-in permits access; it does not launch Chrome. In an opted-in interactive
session the root supervisor uses the fixed `gestalt_managed_playwright` tool:

```json
{"action":"start"}
{"action":"status"}
{"action":"stop"}
```

`start` initially reports pending. At the next root turn, once all active turns
are idle, Gestalt restarts only its dedicated Codex backend and resumes the
same persistent thread with the stdio MCP configured. The native terminal
frontend remains attached through a private Unix socket. Read-only package and
Chrome checks and a connected expected tool catalog precede browser guidance.
Failed checks keep chat and unrelated MCPs usable. `stop` releases the owned
browser at the same supported boundary. Repeated requests are idempotent.
Fresh managed sessions and compatible managed resumptions/forks start idle;
children and native title-generation threads cannot take lifecycle ownership.
Codex 0.160.0 cannot attach new dynamic lifecycle tools when resuming/forking an
ordinary saved thread. Those conversations remain usable with browser access
unavailable; start a fresh opted-in interactive session for managed browser
controls. Older or incompatible managed tool schemas use the same safe fallback.
Gestalt records compatible controls only after native fresh-thread creation
accepts them; the record never replaces current profile consent.
Ending the CLI tears down its owned
runtime and MCP processes while preserving workspace Chrome state.

For another Chrome executable, put the operator-selected path in the profile:

```toml
[mcp_servers.gestalt-playwright]
enabled = true
default_tools_approval_mode = "prompt"
args = ["playwright", "mcp", "--executable-path", "/absolute/path/to/chrome"]
```

Runtime requests accept no command, environment or executable path. The launcher
uses the canonical CLI workspace and its private managed browser profile; it
never uses the personal Chrome profile. A busy workspace remains owned by its
existing browser session. Close or release that session; do not delete live
Chromium locks. Package updates and shutdown preserve cookies.

An Org L1 may declare `- Managed MCP :: playwright`. The root binds a validated
plan already cataloged under this workspace's `.gestalt/`, for example
`{"action":"bind_plan","plan":"browser-task"}` for `browser-task.org`.
`status` returns the current host catalog of bounded plan IDs. The persisted
tool schema does not freeze that catalog, so compatible resumed sessions can
bind newly created plans. The host derives the active WIP requirement from that
bound plan. Removing the
requirement or completing the milestone releases availability at the next
running root turn. Binding neither installs anything nor resumes paused work,
and a declaration never overrides profile consent. Unknown plan IDs are
rejected; arbitrary paths are not accepted.

Ordinary MCP approvals and the shell sandbox configuration are preserved.
The host MCP has the host user's filesystem authority; shell writable roots do
not confine browser tools. Chrome retains its own sandbox, uses a browser pipe,
and runs headless by default. The managed allowlist excludes server JavaScript,
page evaluation, installation and page-supplied WebMCP tools. No HTTP MCP/CDP
listener or nested `codex sandbox` is introduced.

Explicit user `--remote`, `--no-daemon`, `--worktree` and noninteractive
`exec`/`review` retain their native behavior and do not enter this managed
interactive runtime. Their managed browser capability stays unavailable. The
ordinary launch path still supports unrelated Serena/XERJ preparation.
`GESTALT_PLAYWRIGHT_READY_TIMEOUT_MS` bounds individual browser readiness
operations (100–60000 ms, default 5000), independently of retrieval readiness.

## Optional workspace semantics

```sh
gestalt serena install
gestalt cli -C /absolute/project/path
gestalt serena doctor --cwd /absolute/project/path --json
```

For an MCP client that needs an explicit launcher, use
`gestalt serena mcp --cwd /absolute/project/path`. This starts a stdio connection;
Codex and Mobile manage it automatically for sessions that select Serena.
Run `gestalt serena help` to see all commands. If an older installed manager
does not recognize `serena`, update the manager before installing the capability.

When the selected Gestalt plugin includes `gestalt:serena` and Serena is installed,
the CLI adds a session-only `gestalt-serena` connection bound to the selected
directory. Named profiles and skill exclusions apply. Sessions and child agents
receive the same conditional guidance; resume checks the current installation
again. Launch never installs or updates Serena and never rewrites global Codex
configuration.

The connection exposes a validated tool catalog. Verify a successful
`get_symbols_overview` on a current source file before relying on language
semantics or editing. The first native call starts the backend with that call's
effective sandbox policy. Missing tooling, a failed connection or language
service, denied approval, and read-only cache bootstrap all fall back to native
code tools. XERJ and Serena are independent optional capabilities.

Use XERJ to discover references, Serena for semantic work in the active project,
and context-mode to analyze large output. Serena always uses Codex context and
editing mode; session approvals and permissions still govern every call. The
manager defaults Serena tools to `approve` in `never` sessions, preserving
explicit server and per-tool overrides. Collaboration plan mode also
prohibits editing through agent instructions.

With `approval_policy = "never"`, explicit tools that require a prompt are denied.
Gestalt supplies this Serena default automatically; it can also be configured in
a named profile:

```toml
[mcp_servers.gestalt-serena]
command = "/home/you/.local/bin/gestalt"
args = ["serena", "mcp", "--cwd", "/absolute/project/path"]
default_tools_approval_mode = "approve"
```

Use your absolute manager and project paths. This approves tool dispatch,
including editing; it grants no filesystem or network permissions. Native
sandbox restrictions still apply. Existing per-tool approval overrides are
preserved: for example, `approval_mode = "prompt"` under
`[mcp_servers.gestalt-serena.tools.replace_symbol_body]` still denies that edit
when session approval is `never`, while approved semantic reads remain usable.

`gestalt doctor` checks Bubblewrap (system or Codex-bundled), Linux user-namespace
settings, and a bounded native Codex sandbox probe. `gestalt serena doctor`
includes this evidence in its `sandbox` JSON field and stops before MCP startup
if the probe fails, with the original sandbox diagnostic. The checks report
Ubuntu AppArmor restrictions without changing system settings.

Native Serena state stays in the selected project's `.gestalt/serena`. An
operator doctor result describes its diagnostic process, not the effective
permissions or language readiness of a later session. See
[Serena troubleshooting](../troubleshooting.md#serena-installation).

## Launch Mobile

```sh
gestalt mobile --cwd "$HOME/devel"
gestalt mobile --cwd "$HOME/devel" --port 3000
gestalt mobile --cwd "$HOME/devel" --skills focused
```

Every option after `mobile` is passed to `gestalt-mobile`. See [network
deployment](../mobile/deployment.md) before using a non-loopback listener.

## Resolve managed paths

Use the manager instead of reconstructing paths below versioned npm or Codex
plugin-cache directories:

```sh
gestalt path mobile
gestalt path bin
gestalt path context-mode-cli
gestalt path org-plan
gestalt path context-mode
gestalt path context-mode-plugin
gestalt path --json
```

`context-mode` resolves its prepared runtime; `context-mode-plugin` resolves
the installed plugin source. `bin` is the stable command directory,
`~/.gestalt/bin` by default; `context-mode-cli` and `org-plan` print the public
launchers in that directory. Setup refreshes these launchers on updates and
keeps one prepared context-mode runtime, without a version history.
Sessions launched by Mobile also receive
`GESTALT_MOBILE_BIN`, `GESTALT_CONTEXT_MODE_RUNTIME`, and the corresponding
plugin-root environment variables. The command remains the portable discovery
surface for both interactive shells and executors.

## Refresh the installation

```sh
gestalt update
gestalt update --help
```

This upgrades `dyne/gestalt-agents`, reruns its required setup script, verifies
the context-mode runtime and complete `$gestalt:*` app-server skill catalog,
updates the stable `$CODEX_HOME/bin/org-plan` helper, and installs
`gestalt-mobile@latest` under the Gestalt home. Start a new session after the
update because running sessions retain their startup catalog. By default it also
updates installed Serena and XERJ through their managed release installers.
Optional components that are not installed are reported as skipped.

Use one component name to limit the update:

| Component | Scope |
| --- | --- |
| `all` | Gestalt core plus installed Serena and XERJ; the default |
| `gestalt` | Manager, Agents, Mobile, context-mode, and generated profiles/helpers |
| `serena` | Latest stable Serena with its private managed uv and Python tooling |
| `xerj` | Latest official managed XERJ release |

```sh
gestalt update gestalt
gestalt update serena
gestalt update xerj
gestalt update --component xerj
```

Explicitly selecting `serena` or `xerj` installs that component if absent.
The `--component NAME` form is equivalent to a positional component name.
`--extra-skills` is supported only with `all` or `gestalt`. Node, npm, Codex,
Kimi, and system language servers remain managed by their own installers.
Serena's release validation and XERJ's checksum/version checks still apply;
existing XERJ servers and indexes are preserved.

When you are already working in a session opened through Gestalt Mobile, use:

```sh
gestalt update-restart
```

The GUI's **Upgrade** action uses this same all-components update path.
The command schedules the update outside the current Codex process. Mobile
continues running if any update step fails. After a successful update it shuts
down gracefully, closes its Codex children, and restarts with the same working
directory and command-line options. The browser reconnects to the relay and the
durable sessions remain available. Progress is written to
`$GESTALT_HOME/update-restart.log`.

`update-restart` also accepts the same optional component selector when called
from a managed Mobile session. Any failed component update prevents the restart;
completed component updates are retained and the error is recorded in the log.

The command is intentionally available only to sessions launched by
`gestalt mobile`; it will not guess at or terminate an unrelated process.

To opt into the curated third-party skill set maintained by Gestalt Agents:

```sh
gestalt update --extra-skills
```
