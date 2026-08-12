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
