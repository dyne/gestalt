# Gestalt CLI

`gestalt` is the small manager that keeps the Agents plugin profile and Mobile
relay together without merging their state. Run `gestalt help` at any time.

## Commands

| Command | Effect |
| --- | --- |
| `gestalt install` | Install or reconcile Agents, context mode, profiles, and Mobile |
| `gestalt update` | Checksum-update the manager, upgrade Agents, rerun setup, and update Mobile |
| `gestalt update-restart` | From a live Mobile session, update everything and gracefully restart Mobile |
| `gestalt cli [args…]` | Launch Codex with the isolated Gestalt home |
| `gestalt serena install` | Prepare the optional managed Serena release |
| `gestalt serena update [VERSION]` | Validate and activate a Serena update |
| `gestalt serena version` | Print the installed Serena version without startup |
| `gestalt serena doctor [--cwd ROOT] [--json]` | Check install, connection, and optional project language readiness |
| `gestalt serena index --cwd ROOT` | Explicitly warm native project caches |
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

## Optional workspace semantics

```sh
gestalt serena install
gestalt cli -C /absolute/project/path
gestalt serena doctor --cwd /absolute/project/path --json
```

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
manager does not auto-approve Serena edits. Collaboration plan mode also
prohibits editing through agent instructions.

With `approval_policy = "never"`, tools that require a prompt are denied. For a
trusted project, an operator can opt into Serena tool execution in their isolated
Codex configuration or named profile:

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
```

This upgrades `dyne/gestalt-agents`, reruns its required setup script, verifies
the context-mode runtime and complete `$gestalt:*` app-server skill catalog,
updates the stable `$CODEX_HOME/bin/org-plan` helper, and installs
`gestalt-mobile@latest` under the Gestalt home. Start a new session after the
update because running sessions retain their startup catalog.

When you are already working in a session opened through Gestalt Mobile, use:

```sh
gestalt update-restart
```

The command schedules the update outside the current Codex process. Mobile
continues running if any update step fails. After a successful update it shuts
down gracefully, closes its Codex children, and restarts with the same working
directory and command-line options. The browser reconnects to the relay and the
durable sessions remain available. Progress is written to
`$GESTALT_HOME/update-restart.log`.

The command is intentionally available only to sessions launched by
`gestalt mobile`; it will not guess at or terminate an unrelated process.

To opt into the curated third-party skill set maintained by Gestalt Agents:

```sh
gestalt update --extra-skills
```
