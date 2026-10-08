# Contributing

This documentation site is built with the shared `dyne-vitepress` theme.

## Local preview

```sh
npm ci
npm run dev
```

## Production build

```sh
npm run build
```

For the project site at `https://dyne.github.io/gestalt/`, verify the subpath build too:

```sh
BASE_PATH=/gestalt/ npm run build
```

## Refresh source snapshots

Copy the current public documentation from the sibling `gestalt-agents` and
`gestalt-mobile` repositories into `reference/upstream/`, then build and repair
any links that no longer resolve. Internal Org plans, agent-only repository
instructions, generated results, and protocol fixture dumps are not published
as user documentation.

## Validate scripts

```sh
npm test
```

The focused Bash tests use isolated temporary homes and mocked external
commands. They do not alter a developer's Codex profile or global npm packages.

## GitHub Pages deployment

The Pages workflow runs on pushes to `main` and can also be started manually.
It derives `BASE_PATH` from GitHub Pages configuration, so project-site routes
and assets work under the repository subpath. Repository administrators must
select **GitHub Actions** as the Pages source once before the first deployment.

The same workflow versions the manager using Conventional Commits and the
`ietf-tools/semver-action`. `feat` and `feature` commits increment the minor
version; `fix`, `bugfix`, `perf`, `refactor`, `test`, and `tests` commits
increment the patch version. When a bump is due, the workflow sets `GESTALT_CLI_VERSION` to the calculated
release version, commits the manager if it changed, and creates the matching `vMAJOR.MINOR.PATCH` tag. The versioned manager
continues to be distributed only through the latest GitHub Pages deployment.

The manager checksum is not tracked. The installer and updater download one
manager file and validate its Bash syntax and version declaration. Pages generates
a compatibility checksum only in the build output for older installed wrappers.

Feature PRs do not need to bump the manager version. The Pages workflow owns
the published version and accepts source versions that differ from the previous tag.

The Pages job enables unprivileged user namespaces on its disposable hosted
Linux runner before native Codex tests, matching OpenAI's Codex Action setup.
A sandbox smoke check runs first so runner restrictions fail with the original
Codex diagnostic rather than a generic Serena backend error. This setup does
not change developer installations or self-hosted runners.
