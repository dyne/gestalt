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

## Optional Dyne.org collection

`gestalt install` and `gestalt update` maintain the independent
[`dyne/gestalt-skills`](https://github.com/dyne/gestalt-skills) collection by
default. Its initial release, `0.1.0`, contains `nuklear-gui` for Nuklear C/C++
GUI work. Use its ordinary name, without a `gestalt:` prefix.

```sh
gestalt skills install
gestalt skills update
gestalt skills status
gestalt skills status --json
```

These targeted commands maintain only the collection. The stable content root is
`$CODEX_HOME/gestalt-skills`, with discovery links under `$CODEX_HOME/skills`.
`CODEX_HOME` defaults to `~/.codex-gestalt`. Versions never appear in these paths.
Status and `gestalt doctor` show the installed version, immutable revision,
integrity, and a recovery command; status and help make no filesystem changes.

Installation makes these skills available for normal triggering. Select them in
Mobile's existing skill profiles; they are optional and saved disabled choices
survive updates. Existing complete selections keep newly added skills disabled.
The collection does not change infrastructure advertisement or provider readiness.
Start a new session to refresh its catalog after maintenance.

The installer checks the manifest and every bundled file before promotion. It
preserves `.system`, unrelated skills and profiles, and refuses conflicting names
or local collection edits. Preserve those edits or move the conflicting entry
before retrying the recovery command. An interrupted transaction is rolled back;
a later maintenance invocation recovers a proven dead owner. Unknown lock
ownership requires inspection rather than deleting another operation's state.

The receipt at `$CODEX_HOME/gestalt-skills/receipt.json` records actual source,
revision, manifest and file digests. Current canonical content can establish
collection provenance; the receipt alone cannot. `GESTALT_SKILLS_SOURCE` supports
an HTTPS mirror or absolute local test repository, but any explicit override is
unverified. `GESTALT_SKILLS_REF` selects a ref or immutable commit (default `main`).
Neither option changes activation. Downloaded validators and skill scripts never
execute during installation; no Nuklear, SDL, font or application build is fetched.

`skills-export` and `skills-import` retain archive format 1. Managed collection
links export as ordinary skill directories, without installer receipts; canonical
profile paths relocate to the receiving Codex skill directory. Imported content
remains an ordinary skill. Import refuses an existing symlink destination, and a
later collection install refuses a colliding imported directory. Resolve the
collision explicitly rather than overwriting maintained or local content.

Maintainers intentionally update `MAJOR.MINOR.PATCH`, regenerate the sorted
version-1 manifest, and publish the immutable collection revision before releasing
a manager that defaults to it. The manifest has exact root/skill/file fields,
nonempty inventories, names shorter than 64 characters, canonical relative paths,
and SHA-256 digests; duplicate JSON keys, links, extra files and drift are rejected.
Checksums describe integrity, not resistance to someone replacing both files and
receipts or an independent security audit.

## Optional curated third-party skills

The marketplace also maintains an opt-in third-party set. Install or refresh it
with:

```sh
gestalt update --extra-skills
```

These are separate packages. Gestalt never silently disables conflicting
skills from another enabled plugin.
