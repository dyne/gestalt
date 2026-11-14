# Troubleshooting

## Start with the manager

```sh
gestalt doctor
```

It checks Node, npm, Codex, configured directories, installed plugins, and the
Mobile executable without printing prompts, model output, secrets, or arbitrary
environment values. It also verifies that the context-mode plugin source is
disabled as expected and that the native context-mode MCP bridge is enabled.

`gestalt version` (also `-V` or `--version`) lists the manager and detected
Mobile, Agents, context-mode, Codex, Kimi, Serena and Playwright versions.
The first line remains the manager version. Restart Mobile after updating
components so its installed capabilities are rediscovered.

For an older installation, follow the [Xerj retirement guidance](start/install.md#retiring-older-xerj-installations).
A retirement diagnostic requires reviewing the indicated legacy configuration;
reinstalling a binary or deleting index data is unnecessary.

## `gestalt: command not found`

Add the default user binary directory to `PATH`:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Put the same line in your shell profile, then open a new terminal.

## Context mode is not prepared

If `ctx-doctor` or startup reports `CONTEXT_MODE_NOT_PREPARED`:

```sh
gestalt update
```

Confirm Node.js, `python3`, `make`, and a C/C++ compiler are installed. Also
check that a second context-mode marketplace variant is not enabled.

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

## Update fails halfway

Rerun `gestalt update`. Both the Agents runtime publication and the manager
binary installation are designed to replace prepared artifacts atomically.
Existing Codex and Mobile state is stored separately from install artifacts.

## Serena installation

Use `gestalt serena install` to prepare missing tooling and
`gestalt serena update` to repair or refresh the supported release. Download,
checksum, Python, package and MCP validation failures leave the prior active
installation intact. Check network access to official GitHub releases, Python
distribution downloads and the Python package index. No system uv is required.

The install lock prevents concurrent promotion. Interrupts clean up the candidate
and lock. After an uncatchable kill, verify no installer is still running before
removing `$GESTALT_HOME/serena/.install-lock` and retrying. Unactivated candidate
directories can then be removed; retain active and in-use releases.

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
