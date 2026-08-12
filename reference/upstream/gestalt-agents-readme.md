<h1 align="center">
  Gestalt Agents Orchestrator Methodology<br/><br/>
  <sub>We invite you to stop assembling the pieces and start perceiving the whole.</sub>
</h1>

<p align="center">
  <a href="https://dyne.org">
    <img src="https://img.shields.io/badge/%3C%2F%3E%20with%20%E2%9D%A4%20by-Dyne.org-blue.svg" alt="Dyne.org">
  </a>
</p>

<br><br>

This methodology is based on Emacs org-mode and concepts by Ludwig Wittgenstein

### 📖 More info on [dyne.org/gestalt](https://dyne.org/gestalt) <!-- omit in toc -->


## Org-mode plan and development

This method optimizes on token usage and quality of code by leveraging
org-mode as planning format and context-mode as token saving memory
system. It adopts a light multi-agent setup to keep the workflow and
avoid stall. The prepared agents default to:

```text
director (depth 0, org-plan-reviewer, Sol or Terra, read-only)
└── executor (depth 1, org-plan-executor, Terra, only code writer)
```

The root director also performs the supervisor and reviewer duties. It directly
launches one fresh executor for each L1, supervises its evidence gates, and
reviews its uncommitted result. Rejected work returns to the same executor;
accepted work is committed once before that executor closes.
This keeps the root active with only one subagent below it. Evidence flows
upward as concise summaries; raw test and inspection logs stay outside
conversational context. The root gives brief user-facing updates such as
`L1 2/5 — Validate release metadata: in review`.

Context-mode transports evidence; it does not spawn agents.

## 🎮 Quick setup

### Requirements

Gestalt setup requires Node.js 22.5 or newer and npm. It uses Bun for dependency
installation when a working Bun executable is available. Building context-mode
also requires network access, `python3`, `make`, and a C/C++ compiler.

### Fresh Codex install

Gestalt uses an isolated Codex home at `~/.codex-gestalt`. Add the marketplace
to that profile and locate its checkout:

```sh
export CODEX_HOME="$HOME/.codex-gestalt" && mkdir -p "$CODEX_HOME"
codex plugin marketplace add dyne/gestalt-agents
"$CODEX_HOME/.tmp/marketplaces/dyne-gestalt-agents/gestalt-setup.sh"
```

You may change 'add' to 'upgrade' in the middle of the second line.

If you use `codex-profile` then add `alias gestalt='codex-profile cli gestalt'` else invoke `CODEX_HOME="$HOME/.codex-gestalt" codex`.


### Developer's installl

You may also run `gestalt-setup.sh` from a development checkout. If Codex has a
different marketplace snapshot configured, the script continues from that
snapshot automatically and preserves its arguments.

The script defaults `CODEX_HOME` to `~/.codex-gestalt`, installs both plugins,
and prepares context-mode under `~/.gestalt`. Setup generates `org-plan-reviewer` and `org-plan-executor`,
then removes the obsolete
`~/.codex-gestalt/agents/org-plan-supervisor.toml`. It does not create,
validate, or rewrite `config.toml`; `codex plugin add` only records the installed
plugins.
Current Codex already defaults the V1 agent depth to one and enables stable
lifecycle hooks. The former `features.plugin_hooks` flag has been removed, so
Gestalt needs no configuration override. On an older installation, remove an
`agents.max_depth = 2` override; setup does not create, validate, or rewrite
Codex configuration.

Pass `--extra-skills` to opt into the marketplace's curated third-party skill
set. This uses `npx skills` in project scope and keeps its canonical skill
payloads and lock metadata under `${GESTALT_HOME:-$HOME/.gestalt}`, then links
each non-conflicting entry into `CODEX_HOME/skills`. The option requires network
access and is intentionally disabled during normal setup. Use
`--extra-skills-only` to install or refresh only that curated set without
repeating runtime preparation or plugin installation.

Start or restart Codex with that home, verify the effective installation, and
run `ctx-doctor` in a new session:

```sh
export CODEX_HOME="$HOME/.codex-gestalt"
codex plugin list --marketplace dyne-gestalt-agents --json
codex
```

### Runtime preparation details

Run `./gestalt-setup.sh` again after a marketplace upgrade. Use
`./gestalt-setup.sh --prepare-only` to install the external runtime without
installing plugins or changing the isolated Codex home, and `--force` to replace
an invalid prepared runtime. Runtime versions are isolated by operating system,
CPU architecture, and Node ABI under
`${GESTALT_HOME:-$HOME/.gestalt}/runtime/context-mode/`. Set `CODEX_HOME`
explicitly only to test or install an additional isolated Gestalt profile.
Use `./gestalt-setup.sh --force` to rebuild and atomically replace that runtime.

Marketplace installation does not execute setup automatically. On an existing
installation, upgrade the marketplace and rerun its setup script:

```sh
export CODEX_HOME="$HOME/.codex-gestalt"
codex plugin marketplace upgrade dyne-gestalt-agents
"$CODEX_HOME/.tmp/marketplaces/dyne-gestalt-agents/gestalt-setup.sh"
```

Do not add duplicate MCP or hook configuration. If startup still fails after
forced preparation, confirm that another context-mode marketplace variant is
not also enabled. `CONTEXT_MODE_NOT_PREPARED` identifies a missing, incompatible,
or damaged external runtime; rerun setup with `--force` and restart Codex. MCP
and hook startup are side-effect free: only setup installs, builds, or repairs
the external runtime.

## 🧪 Testing (only for developers of this repo)

Run the same complete validation used by CI before publishing changes:

```
bash tests/ci.sh
```

The validation covers repository and Gestalt contracts, plugin and skill
ingestion, context-mode integrity and Codex-focused tests, skill discovery,
nested MCP startup, shell linting, release versioning, and release-workflow
contracts. It also installs both plugins through the current Codex CLI in an
isolated home. GitHub runs it on Linux and macOS with Node.js 22.12.0. The
release job starts only after both operating-system jobs pass.

Releases use conventional commits with `ietf-tools/semver-action@v1`. The
stable release line starts at `v2.0.0`; historical `v0.x` tags are excluded
from future version calculations. Each release assigns the same version to the
Gestalt plugin and the adapted context-mode plugin/runtime. `feat` advances the
minor version, supported fix-oriented commit types advance the patch version,
and breaking changes advance the major version.

# 📃 Plan

Each L1 starts unreviewed. After implementation and test gates make it
DONE, the director/reviewer audits only requested DONE + UNREVIEWED
milestones. Accepted L1s remain reviewed as the plan grows, so later
refinements review only new or materially changed L1s. Final
acceptance still requires a current full-suite pass and clean intended
scope.

Each L1 also declares a non-empty `:SKILLS:` property containing exact
`$skill` references selected from the planner's complete
available-skill catalog. Do not list `$gestalt:context-mode`; it
is an implicit baseline for every role. A fresh executor loads that
baseline plus exactly the declared task-specific list before
inspecting or implementing the L1 and stops without edits when either
is unavailable.

## 💼 License

Copyright (C) 2025-2026 Dyne.org foundation

Designed and written by Denis "[Jaromil](https://jaromil.dyne.org/)" Roio.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful, but
WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public
License along with this program. If not, see
https://www.gnu.org/licenses/.
