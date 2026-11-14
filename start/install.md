# Install Gestalt

The installer is a small Bash bootstrapper. It downloads the versioned manager
script, validates its Bash syntax and version declaration, installs it atomically
in a user-owned directory, and runs `gestalt install` for Agents, the optional
Dyne.org skill collection, and Mobile.

## Install with npm

On Linux or macOS with Node.js 24 or newer, Bash, and the Codex CLI:

```sh
npm install -g @jaromil/gestalt
gestalt cli
```

Global npm installation automatically installs or reconciles Agents and their
context-mode runtime in the isolated Codex profile when install scripts are allowed.
With npm 12, approve this package for the install:

```sh
npm install -g @jaromil/gestalt --allow-scripts=@jaromil/gestalt
```

The same approval is needed on npm 12 updates unless it is already configured
in npm. The installer respects blocked-script policies.

Run `gestalt install` to add
the optional skill collection and Mobile. Use `gestalt agents install` to retry
Agents setup, or `gestalt agents update` to refresh it independently.

`npm install -g @jaromil/gestalt --ignore-scripts` installs only the wrapper;
Agents setup remains explicit in that case. Local dependency installs and npx
inspection do not provision the profile. Setup failures fail npm installation
and print recovery instructions.
The executable lives in npm's configured global prefix; use `command -v gestalt`
to check which installation your shell selects.

Update the command and reconcile Agents with `npm install -g @jaromil/gestalt@latest`. For an npm
installation, `gestalt update` updates managed components and reminds you to
update the command through npm. It preserves npm's executable and package files.

You can inspect the command without installing it globally:

```sh
npx @jaromil/gestalt --help
```

## One-line Bash install

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

Gestalt no longer installs, launches, probes or updates Xerj. Use scoped `rg`
searches and current source reads for text discovery, verified Serena for
semantic operations, and context-mode for large output.

Update the manager through your existing installation method, then run:

```sh
gestalt update gestalt
gestalt doctor
```

For an npm installation, first update the command with
`npm install -g @jaromil/gestalt@latest`. The core update reconciles Agents,
Mobile and context-mode. Restart Mobile and start a new session afterward so
installed skills and runtime capabilities are rediscovered.

The update removes MCP entries only when their absolute executable resolves to
this manager and their complete arguments are exactly `["xerj", "mcp"]`.
Canonical names and aliases use the same ownership check. Ordinary tables in
`CODEX_HOME/config.toml` and named `*.config.toml` profiles are edited atomically;
unrelated settings and comments remain intact. User-defined servers, modified
arguments, bare executable names, linked configuration files and layouts that
cannot be safely localized are preserved with a manual-review diagnostic.
Inspect those entries before changing them; a name alone does not prove ownership.

Old explicit `gestalt xerj` and `gestalt update xerj` invocations now report
retirement before doing work. Remove `--xerj auto`, `--xerj manual` or `--xerj off`
from manually maintained Mobile launch commands. The explicit update/restart
worker removes recognized pairs from the arguments it consumes; it does not
rewrite unrelated or unproven restart files. An unknown legacy value fails
before the update or relay shutdown.

Binaries, index directories, repositories, sessions, credentials, passkeys and
saved skill profiles are retained. The update neither signals legacy processes
nor deletes their records. Existing process records and obsolete restart files
receive bounded review guidance because a PID or filename alone cannot establish
ownership. Stop any separately running old service through its original owner
after inspecting its current identity; do not use process-name kills.

Update the manager, Mobile and Agents together before relying on the removal.
An older Mobile can use its optional-capability fallback when the retired manager
command is unavailable. A new Mobile launched by an older manager must ignore
obsolete version metadata; an obsolete explicit launch option instead requires
removing that option and updating the launcher. No backend or feature switch is
retained for compatibility.

## Install Serena

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
