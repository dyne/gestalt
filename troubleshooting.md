# Troubleshooting

## Start with the manager

```sh
gestalt doctor
```

It reports Node, npm, Codex, configured directories, installed plugins and the
component state. Some fields establish presence without probing a version or
starting a service. Check the specific diagnostic, including missing Agents
marketplace or skills state, before retrying. On native Windows x64, Serena,
Playwright and Mobile show as deferred. An optional deferral alone needs no setup.

`gestalt version` (also `-V` or `--version`) lists the manager and detected
component versions or status. The first line is the manager version.
On supported Unix Mobile installations, restart Mobile after updating
components so it discovers the new capabilities.

## `gestalt: command not found`

On Windows, check npm's global prefix with `npm prefix -g` and check command
resolution with `where.exe gestalt` or `Get-Command gestalt -All`. The generated
`gestalt.cmd` command can run from either PowerShell or cmd. If npm reports
`EBADPLATFORM`, check that the selected version lists `win32` before retrying.
Public version `0.16.1` lists Linux and macOS.

For a standalone Unix install, add the default user binary directory to `PATH`:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Put the same line in your shell profile, then open a new terminal.

## Context mode is not prepared

If `ctx-doctor` or startup reports `CONTEXT_MODE_NOT_PREPARED`:

```sh
gestalt agents update
```

Confirm Node.js and Codex are available. If the npm lifecycle script was blocked,
run `gestalt agents install` after confirming the selected Agents marketplace
contains the compatible setup. For npm 12 installation approval syntax see
[Install with npm](start/install.md#install-with-npm). Check whether a second
context-mode marketplace variant is enabled before changing plugin state.

## Context-mode plugin says disabled

That is the expected Gestalt configuration. Do not enable
`context-mode@dyne-gestalt-agents` by hand: `gestalt install` and
`gestalt update` register its runtime as a native required MCP and disable the
plugin's duplicate manifest launcher. Run `gestalt doctor` to check both sides
of that bridge. If it reports drift, run:

```sh
gestalt update
```

## Mobile will not start

These checks apply to the existing Linux and macOS Mobile routes.

```sh
codex --version
gestalt mobile --version
npm view gestalt-mobile version
```

Codex must be authenticated in the same user account. If Mobile reports an
incompatible Codex protocol, update Gestalt or install the Codex version
supported by that Mobile release.

## A Mobile option is rejected

```sh
gestalt mobile --help
```

The installed executable is authoritative. The manager forwards arguments
unchanged.

## Processes remain after stopping Mobile

Console Ctrl+C and the menu's Quit action both call Mobile's graceful close.
Repeated Ctrl+C or SIGTERM signals remain intercepted while cleanup finishes.
Owned Codex, browser and host-test processes must stop; subprocess
supervisors retain their own process groups until resistant descendants have
been terminated. Embedded supervisor programs travel over source pipes, so
process listings show short helper names instead of JavaScript bodies.

Each Unix account has separate managed state and process ownership. Avoid
system-wide process-name kills on hosts with several Gestalt users. Processes
left by an older installation or an interrupted test need separate inspection;
updating the launcher does not retroactively attach ownership to them.

## Passkey origin errors

Check that `--public-origin` exactly matches the address in the browser,
including `https://` and any non-default port. Only `http://localhost` is valid
without HTTPS. Do not change the RP-ID hostname after devices are enrolled.

## An update fails or retains state

Check the first reported stage that failed. In an npm install, update or restore
the command through npm. See [npm upgrade and rollback](start/install.md#upgrade-or-roll-back-an-npm-installation).
In a standalone Unix update, an invalid replacement leaves the existing
executable available. Completed component steps may persist after a later failure.

If the diagnostic reports unknown ownership or retention, keep the named
snapshot, lock and scratch records. Windows performs managed process cleanup
through the owning Job. Linux follows its existing Unix supervision and
sandbox rules. macOS provides best-effort cleanup and detached work can
outlive its anchor. Treat an unknown result as unresolved before retrying
setup or using the offline scratch recovery commands. Inspection tokens
identify retained records and require separate operator confirmation that
the owner is quiescent. See [scratch inspection](cli/index.md#inspect-retained-scratch-state).

## Serena installation

This route applies to Linux and macOS. Native Windows core setup omits Serena.

Use `gestalt serena install` to prepare missing tooling and
`gestalt serena update` to repair or refresh the supported release. An update
reuses a healthy installation of the requested version. Download,
checksum, Python, package and MCP validation failures leave the prior active
installation intact. Check network access to official GitHub releases, Python
distribution downloads and the Python package index. No system uv is required.

The install lock prevents concurrent promotion. Interrupts clean up the candidate
and lock. After an uncatchable kill, verify no installer is still running before
removing `$GESTALT_HOME/serena/.install-lock` and retrying. Unactivated candidate
directories can then be removed; retain active and in-use releases.

After stopping Gestalt Mobile, Codex, and any standalone Serena processes that
use this Gestalt home, run `gestalt serena update VERSION --prune-offline` to
keep the active release and remove the other managed release directories. Use
the output of `gestalt serena version` to clean up without changing versions.
Some workspace uv environments may still link to an interpreter in a removed
release. If a workspace's language server then fails, set aside its
`.gestalt/serena/uv-tools`, `uv-bin`, and `uv-cache` directories and let Serena
recreate them. Keep the rest of the workspace's Serena state.

Run `gestalt serena doctor --cwd /absolute/project/path` to distinguish missing
installation, a failed MCP connection, and a failed project language service.
`--json` returns `installed`, `connected`, `projectReady` and `reason`. Without
`--cwd`, `projectReady` is null because no project was checked. A connected
transport with a failed language service returns failure; inspect the diagnostic
on stderr and install the language's prerequisites or allow the required cache
preparation/downloads through the existing permission workflow. The default
connection/language deadline is 30 seconds;
`GESTALT_SERENA_DOCTOR_TIMEOUT_MS` accepts 100–60000 ms for diagnostics.

Use `gestalt serena index --cwd /absolute/project/path` only when you want
explicit native cache warmup. Normal sessions rely on native lazy language
services. The doctor avoids Serena's native project health-check command, which
in the tested release writes logs to legacy `.serena` even with relocated state.
Legacy `.serena` configuration is preserved in place; existing project settings
can initialize managed config, while memories and caches are never migrated
automatically.

A restrictive native network profile has been observed to time out during real
MCP startup. This remains a closed failure; the manager does not change network
authority. Collaboration plan mode is an agent instruction: the tested Codex
0.160.0 supplies the same filesystem metadata in default and plan modes, so
plan-mode edit prohibitions are not a separate OS permission boundary.
