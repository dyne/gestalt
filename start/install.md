# Install Gestalt

Use npm for the core CLI. Linux and macOS users can also use the existing Bash
installer. The native Windows x64 candidate has been validated separately from
the public npm release. The published `0.16.1` package supports Linux and macOS.

## Install with npm

Agents-only setup needs Node.js 24 or newer, npm and a compatible Codex CLI in
the same account. Linux and macOS also need Bash. Native Windows x64 uses
PowerShell or cmd and requires no Bash, WSL, Developer Mode, administrator
privileges or compiler for core setup. On Windows, full `gestalt install` and
the default `gestalt update` also require Git on `PATH` for the Git-backed skill
collection. Use Git to clone or update a local Agents checkout.

On Windows, check the registry before installing from `latest`. Use a release
whose `os` includes `win32`, or an explicitly supplied and verified Windows-enabled
tarball.

```powershell
npm view @jaromil/gestalt@latest version os
```

On Linux or macOS, or once the registry lists a Windows-enabled release, use the
following command with npm 11.

```sh
npm install -g @jaromil/gestalt@latest --foreground-scripts
```

For an existing local dependency, run `npm install @jaromil/gestalt@latest --foreground-scripts`
from its project. The explicit tag selects the current release even when the project's
saved version range excludes it. Foreground scripts show the installed version report
when npm runs the install hook. If npm reports that everything is already up to date,
use `npm exec -- gestalt version` in that project to see the report again.

With npm 12, approve the package's lifecycle script. Use a Node version supported
by your npm client. npm 12.2.2 requires at least Node 24.15.0 on the Node 24 line.

```sh
npm install -g @jaromil/gestalt@latest --foreground-scripts --allow-scripts=@jaromil/gestalt
```

For a verified local tarball, approve the resolved file instead. In PowerShell,
substitute the supplied tarball filename.

```powershell
$tarball = (Resolve-Path .\gestalt-candidate.tgz).Path
npm install -g "--allow-scripts=file:$tarball" $tarball
```

When lifecycle scripts are allowed, a global install reconciles Agents and
context-mode in the isolated Codex profile. Use `--ignore-scripts` to install
only the command, then run `gestalt agents install` when ready. Local dependency
installs and `npx @jaromil/gestalt --help` leave profile setup explicit. If setup fails,
its diagnostic names the next action. Retry Agents with `gestalt agents install`
or refresh them with `gestalt agents update`.

Check which command PowerShell selects with `Get-Command gestalt -All`. If
PowerShell blocks the npm-generated script, invoke the `gestalt.cmd` shim.
Use `where gestalt` in cmd or `command -v gestalt` on Unix.

```powershell
gestalt.cmd --version
gestalt.cmd doctor
gestalt.cmd cli
```

Native Windows defers Serena, Playwright/Chrome and Mobile hosting. Core
setup and updates report these omissions. An explicit request for a deferred
component fails before profile or install state changes. Linux and macOS keep
their existing optional routes. Use `gestalt install` there to add Mobile and
the optional skill collection.

### Upgrade or roll back an npm installation

Use npm to upgrade the command. On Windows, verify that the requested release
lists `win32` first. On npm 12, supply the lifecycle approval again unless
already configured.

```sh
npm install -g @jaromil/gestalt@latest --foreground-scripts
```

`gestalt update` updates managed components and preserves the npm executable
and package files. To return to a previously validated compatible npm version,
install it explicitly with `npm install -g @jaromil/gestalt@VERSION --ignore-scripts`
after substituting the version. This restores the executable and package
files only. Keep the profile and managed data, review the reported component
state, and reconcile Agents only after confirming the older command and
Agents version work together.

You can inspect the command without installing it globally:

```sh
npx @jaromil/gestalt --help
```

## One-line Bash install

On Linux and macOS, the Bash bootstrapper downloads the versioned manager,
validates its syntax and version declaration, installs it atomically in a
user-owned directory, and runs `gestalt install`.

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
| `~/.codex-gestalt/gestalt-skills` | Stable optional collection, manifest, and installer receipt |
| `~/.codex-gestalt/skills/<name>` | Discovery links to optional collection skills |
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

## Optional skill collection

Ordinary installation and update include the independent
[`dyne/gestalt-skills`](https://github.com/dyne/gestalt-skills) collection. Its
initial version `0.1.0` provides the ordinary optional skill `nuklear-gui`.
This is independent of the third-party bundle selected by `--extra-skills`.

```sh
gestalt skills install          # collection only
gestalt skills update           # refresh collection only
gestalt skills status           # read-only version, revision and integrity
gestalt skills status --json
```

Choose activation in existing Mobile skill profiles. Updates preserve selections,
including disabled skills and complete profiles that omit newly added skills.
The canonical content and discovery paths stay stable across releases; do not add
versioned paths to profiles or duplicate the collection under `~/.agents/skills`.
The collection does not install GUI toolchains or execute downloaded validators.

`gestalt doctor` reports missing or damaged collection state and a recovery command.
Maintenance refuses user-skill name collisions or edited managed content. Preserve
local changes and resolve the indicated conflict before retrying; unrelated skills,
`.system` and saved profiles remain untouched. A failed collection step fails the
default install/update and leaves the previous usable collection and receipt.

See [the collection and transfer rules](../agents/skills.md#optional-dyneorg-collection)
for source overrides, manifests, portable archives and intentional release updates.

## Retiring older Xerj installations

The former Xerj integration is retired. Ordinary core upgrades remove only
positively identified manager-owned entries. Unrelated configuration and
legacy binaries, indexes, sessions, credentials and profiles are preserved.
The [CLI contract](../docs/native-cli-contract.md) records the retirement
behavior for older configurations and commands.

## Install Serena

This route applies to Linux and macOS.

Run `gestalt serena install` once to install private uv 0.12.23, managed Python
3.13 and the tested Serena 1.7.0 release. This explicit command downloads verified
official uv tooling and installs the released Python package without sudo,
questions, shell profile changes or client configuration rewrites. Ordinary
agent sessions never install or upgrade Serena. Linux and macOS x86_64/aarch64
artifacts are supported; real installation has been verified on Linux only.

`gestalt serena update [VERSION]` prepares a fresh environment at its permanent
path, verifies package metadata and the Codex editing MCP tool contract, then
activates it atomically. With no VERSION, update resolves the latest stable official PyPI release. An
explicit VERSION selects that release. Every candidate must pass native storage
preparation, Python/package identity, CLI options and Codex editing tool checks;
incompatible releases are rejected before activation. Failed
updates preserve the active installation and workspace data. Previous releases
remain available to running sessions and to existing workspace uv caches.
Only exact validated managed interpreter targets are allowed for uv venv Python
links; other project-state links that escape the workspace are rejected. Shared prepared tooling lives below
`$GESTALT_HOME/serena`; project data belongs to the session workspace's
`.gestalt/serena`. The installer never moves a constructed uv environment.

`gestalt serena version` (also `gestalt serena -V`) reads installed package
metadata without importing Serena or creating user state. `gestalt version`
also reports Serena, its private uv and Python versions. The manager exports
`GESTALT_SERENA_VERSION`, `GESTALT_UV_VERSION` and
`GESTALT_SERENA_PYTHON_VERSION` to Mobile, alongside existing component versions.

`gestalt serena doctor` checks installation identity and live MCP tools in
disposable storage. Add `--cwd ROOT` to prepare that canonical project's native
`.gestalt/serena` state and read symbols from one supported source file. Output
separates installed, connected and project-ready; `--json` exposes the same
states. A successful connection alone does not prove a working language server.
The bounded source probe checks at most 1,000 files/directories and never follows
project symlinks. A project with no supported file is reported as not ready.
The probe does not edit source. Global `gestalt doctor` includes the connection
check when Serena is installed.

`gestalt serena index --cwd ROOT` explicitly runs Serena's native project index
to warm symbol caches; it does not run during session startup. Native indexing
uses a ten-second per-file timeout and a five-minute overall limit. Doctor and
index are explicit operator commands running under the caller's existing OS
authority. Session MCP requests still require actual native Codex sandbox
metadata, and fail closed when that policy cannot start the service. Neither
command expands filesystem or network permissions. All managed MCP launches
include `--context codex --mode editing`.

First use of a language may require its upstream language-service packages or
compiler tools. Python's language service may use workspace-local uv tools and
cache downloads; C/C++ needs an available `clangd`. Network-restricted or
read-only policies can prevent first-use startup or cache preparation. Install
required tools or prepare permitted caches under the intended policy; a health
check never widens that policy. Runtime config, caches, memories, logs and
language-tool state remain below `<workspace>/.gestalt/serena`; shared uv, Python
and Serena environments remain under `$GESTALT_HOME/serena`.
