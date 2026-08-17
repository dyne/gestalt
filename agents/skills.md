# Gestalt skill catalog

The Gestalt plugin distributes 13 skills. Codex exposes them with the `gestalt`
provider, so invoke them as `$gestalt:<skill-name>`. Every Gestalt skill is
session infrastructure: saved Mobile profiles cannot hide or disable it, and
an Agents update reconciles the catalog against the newly installed version.
Start a fresh session after `gestalt update`; an already-running Codex session
keeps the catalog captured when it started.

## Development workflows

| Skill | Use it for |
| --- | --- |
| `development-testing` | Keep implementation and observable automated tests coherent |
| `org-plan` | Author and execute supervised Org plans |
| `systematic-debugging` | Establish evidence, root cause, and regression verification |
| `verification-before-completion` | Match completion claims to current evidence |
| `writing-skills` | Create reliable, discoverable reusable skills |

`gestalt cli` and `gestalt mobile` also put `$CODEX_HOME/bin` on `PATH`, where
Agents setup installs the matching `org-plan` helper on every update.

## Context-mode skills

| Skill | Use it for |
| --- | --- |
| `context-mode` | Route large or uncertain output through compact derived findings |
| `ctx-doctor` | Diagnose runtime, hooks, dependencies, and registration |
| `ctx-index` | Persistently index local files for later focused retrieval |
| `ctx-insight` | Open the hosted engineering analytics dashboard |
| `ctx-purge` | Permanently delete selected context-mode data |
| `ctx-search` | Search indexed project content or session memory |
| `ctx-stats` | Inspect token savings and tool activity |
| `ctx-upgrade` | Upgrade or repair context-mode installation state |

## Optional curated skills

The marketplace also maintains an opt-in third-party set. Install or refresh it
with:

```sh
gestalt update --extra-skills
```

These are separate packages. Gestalt never silently disables conflicting
skills from another enabled plugin.
