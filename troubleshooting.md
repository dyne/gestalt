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
Mobile, Agents, context-mode, Codex, Kimi, and xerj versions. The first line
remains the manager version. Mobile's header menu includes the xerj version
detected by its launcher; restart Mobile after updating components.

Doctor also probes xerj readiness without starting a backend or indexing files.
Unavailable xerj is reported as optional: direct source search still works.
`gestalt xerj -V` reports the installed binary version without writing runtime
configuration. A version alone does not prove that retrieval is ready or that
the current workspace has been indexed.

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

## Passkey origin errors

Check that `--public-origin` exactly matches the address in the browser,
including `https://` and any non-default port. Only `http://localhost` is valid
without HTTPS. Do not change the RP-ID hostname after devices are enrolled.

## Update fails halfway

Rerun `gestalt update`. Both the Agents runtime publication and the manager
binary installation are designed to replace prepared artifacts atomically.
Existing Codex and Mobile state is stored separately from install artifacts.

## XERJ startup fails with `unsafe-path` after indexing

If Mobile reports `XERJ startup failed. Optional retrieval unavailable`, first
check the installed manager and its read-only readiness result:

```sh
gestalt version
gestalt xerj status
```

Manager 0.9.3 rejects all hard-linked files in managed XERJ storage. Autoindex
legitimately shares immutable snapshot blobs and prepared documents between
generations using hard links, so a healthy backend can become unavailable to new
sessions after indexing. This was reproduced with XERJ 1.0.0-rc.99. The snapshot
exception shipped in manager 0.9.4; symlinks and hard-linked credentials,
configuration, executables and snapshot metadata remain rejected.

When the installed manager predates 0.9.4, update the core components from a
terminal, then check readiness in a new command invocation:

```sh
gestalt update gestalt
gestalt version
gestalt xerj status
gestalt xerj probe
```

The `gestalt` update group replaces the manager, Agents, Mobile and context-mode
without running the old manager's XERJ updater over existing snapshot files.
Updating only XERJ cannot repair the manager's validator. Pulling source
repositories also leaves the installed manager unchanged.

After the updated manager reports `ready`, restart Mobile through your normal
launcher and start a new session so it can negotiate retrieval again. Existing
sessions may retain the unavailable capability selected at startup. This
recovery preserves indexes and does not require deleting snapshots, credentials
or lifecycle records.

An `unsafe-path` result from an updated manager needs separate investigation;
the status code alone does not establish that snapshot hard links caused it.
Preserve the trace reference and investigate the rejected path before changing
managed storage.

### Rechecking XERJ upstream issues

Use `gestalt xerj update [VERSION]` to install an official checksum-verified
release, then `gestalt xerj test` (or `test --json`) to exercise synthetic indexes
in disposable storage. Exit 1 means upstream issues remain; exit 2 means the
harness could not complete its controls. This never tests against your real index.
A newer binary is not automatically certified for Mobile/MCP readiness; see the
[update and test workflow](start/install.md#updating-and-checking-upstream-fixes).

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
