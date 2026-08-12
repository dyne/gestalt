# Usage and architecture

This repository is a Codex marketplace containing the `gestalt` workflow plugin
and the `context-mode` runtime plugin.

## Codex setup

Context-mode requires Node.js 22.5 or newer, npm, network access during setup,
and the native toolchain needed by `better-sqlite3` (`python3`, `make`, and a
C/C++ compiler). A working Bun installation is used for dependencies when
available. Set `CONTEXT_MODE_PACKAGE_MANAGER=npm` or `bun` to override automatic
selection during setup.

```sh
export CODEX_HOME="$HOME/.codex-gestalt"
codex plugin marketplace add dyne/gestalt-agents
codex plugin marketplace list
cd <MARKETPLACE_ROOT>
./gestalt-setup.sh
```

When setup is launched from another checkout, it automatically continues from
the marketplace snapshot configured in `~/.codex-gestalt`.

Marketplace installation does not run the setup script automatically. The
script installs both plugins, prepares a stable context-mode runtime under
`${GESTALT_HOME:-$HOME/.gestalt}`, verifies its artifact manifest, generates the
reviewer and executor profiles, and removes the retired supervisor profile. It
does not create, validate, or rewrite `config.toml`. Run setup again after a
marketplace upgrade.

Useful modes:

```sh
./gestalt-setup.sh --prepare-only  # install only the external runtime
./gestalt-setup.sh --force         # reinstall dependencies and rebuild
./gestalt-setup.sh --dry-run       # print mutations without running them
./gestalt-setup.sh --extra-skills  # opt into curated third-party Codex skills
./gestalt-setup.sh --extra-skills-only  # update only the curated skill set
```

The curated list is maintained directly in the marketplace's
`gestalt-setup.sh`. It runs `npx skills` in project scope so canonical copies
and lock metadata remain under `${GESTALT_HOME:-$HOME/.gestalt}` rather than
the normal global skills directory. Setup exposes them through per-skill links
in `CODEX_HOME/skills` and preserves entries that already exist there.
`--extra-skills` cannot be combined with `--prepare-only`.
`--extra-skills-only` skips runtime preparation, marketplace registration,
plugin installation, and profile generation.

No configuration override is required: current Codex defaults V1 agent depth
to one and enables stable lifecycle hooks. The former `features.plugin_hooks`
flag has been removed. An older explicit `agents.max_depth = 2` override should
be removed or changed to `1`.

Start Codex with `CODEX_HOME="$HOME/.codex-gestalt"` after setup or
configuration changes. The plugin manifest registers the MCP server, and Codex
discovers the hooks from `hooks/hooks.json`. Do not add duplicate MCP or hook
configuration.

Verify the installation with:

```sh
codex plugin list --marketplace dyne-gestalt-agents --json
```

Then start a fresh Codex session and run `ctx-doctor`. If context-mode reports
`CONTEXT_MODE_NOT_PREPARED`, rerun `gestalt-setup.sh` and check the listed
runtime or build prerequisite.

## Distributed skills

The `gestalt` plugin distributes 13 Codex skills: five development workflows
and eight context-mode routing and command skills. Codex shows each one with
the `(gestalt)` provider and enables or disables it with the other
Gestalt-provided skills.

Each skill has one canonical package under
`plugins/gestalt/skills/<skill-name>/`. Cross-references use exact
`$gestalt:<skill-name>` names. Context-mode routing selects the appropriate
`ctx_*` operation; it does not grant permission to mutate files, install
software, use the network, push changes, or perform another external side
effect.

## Org Plan execution

Supervised Org Plan execution uses two roles:

```text
director (depth 0, org-plan-reviewer, Sol or Terra, read-only)
└── executor (depth 1, org-plan-executor, Terra, only code writer)
```

The root director also performs the supervisor and reviewer duties. It remains
active, communicates with the user, launches one fresh executor for each L1,
checks evidence, and returns ACCEPT or REJECT directly to that executor. There
is no intermediate supervisor and no separate reviewer subagent.

The executor owns all implementation and corrective edits for its assigned L1.
Rejected work stays uncommitted and returns to the same executor. After ACCEPT,
the executor creates one conventional commit, marks the L1 REVIEWED, and exits.
The director then starts a fresh executor for the next L1.

Prepare the recommended profiles with:

```sh
plugins/gestalt/skills/org-plan/scripts/org-plan prepare-supervision
```

This creates `org-plan-reviewer` for a newly launched read-only root and
`org-plan-executor` for depth-one implementation. An already-running root keeps
the model selected by its Codex session while adopting the same director
contract.

Potentially large inspections and test output stay in context-mode or another
context-preserving execution path. Executors report only commands, status,
pass/fail counts, affected scope, and the smallest useful diagnostic excerpt.

## Context-mode runtime

Normal Codex startup never installs dependencies, compiles code, repairs a
registry, or writes generated files. Startup follows this path:

```text
Codex MCP start
  -> start.mjs in the replaceable Codex plugin cache
  -> resolve version + platform + architecture + Node ABI
  -> runtime-preflight.mjs (read only)
  -> ~/.gestalt/runtime/context-mode/<version>/<target>/server.bundle.mjs
```

`runtime-preflight.mjs` verifies the package version and SHA-256 artifact
manifest, including the native `better-sqlite3` binding. An incomplete external
runtime exits with code 78,
`CONTEXT_MODE_NOT_PREPARED`, the invalid paths, and the setup command.

Preparation is explicit:

```text
gestalt-setup.sh
  -> install-runtime.mjs
     -> copy source into a versioned external staging directory
     -> locked dependency install
     -> TypeScript check and bundle build
     -> bundle assertions
     -> versioned SHA-256 manifest
     -> atomic publication under ~/.gestalt
  -> codex plugin add
```

Runtime directories use the package version and
`<platform>-<architecture>-node-<modules ABI>` as their identity. They survive
Codex plugin-cache replacement and can be shared by profiles running the same
Node ABI. `GESTALT_HOME` must be absolute when set. The setup lock prevents
concurrent installers from publishing a partial runtime; MCP and hook startup
are read-only and do not acquire it. Codex hooks use a cache-local launcher that
loads their implementations from the same external runtime.

`package.json` is a private dependency and build manifest. Codex marketplace
installation does not use npm publication metadata or npm lifecycle scripts.
The repository-level setup script is the required pre-flight because Codex has
no marketplace install-build hook, generated bundles are not committed, and
Codex may rematerialize its plugin cache when a session starts.

The context-mode plugin exposes only its Codex MCP and hook surfaces. Hooks use
Codex's auto-discovered `hooks/hooks.json` path, so the plugin manifest does not
declare a custom `hooks` field.

## Maintainer verification

Run the same complete validation used by CI before publishing:

```sh
bash tests/ci.sh
git diff --check
```

Startup coverage verifies that:

1. the launcher and preflight contain no install, build, child-process, or
   write path;
2. an incomplete runtime fails quickly with the stable diagnostic;
3. explicit preparation is concurrency-safe and produces a valid MCP
   `initialize` and `tools/list` exchange after the plugin cache is recreated;
4. Codex hooks do not invoke preparation;
5. plugin manifests, runtime artifacts, skill metadata, and skill discovery
   remain consistent.

The full suite also validates plugin layout, marketplace metadata, Org Plan
state transitions and generated profiles, ShellCheck results, context-mode
integrity, its Codex-focused Vitest suite, and nested MCP startup. GitHub runs
the canonical command on Linux and macOS with Node.js 22.12.0 and verifies both
plugins through the current Codex CLI's real marketplace installation path. A
push to `main` can enter the release job only after both validation jobs pass.

## Release versioning

The repository release line starts at `v2.0.0` and is calculated by
`ietf-tools/semver-action@v1` from conventional commits. Existing `v0.x` tags
remain as historical releases but are excluded from the new calculation
baseline. The Gestalt manifest, context-mode manifest and runtime package use
the same repository release version; context-mode's imported upstream version
remains recorded separately in `UPSTREAM.md`. Once `v2.0.0` exists, `feat` and
`feature` commits bump the minor version; `fix`, `bugfix`, `perf`, `refactor`,
`test`, and `tests` bump the patch version; and a conventional-commit
breaking-change marker bumps the major version. Other commit types do not
create a release.
