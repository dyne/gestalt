# Gestalt documentation

The Dyne-styled VitePress documentation hub for Gestalt Agents and Gestalt
Mobile. It includes the onboarding journey, operational guides, copied source
documentation, a one-line installer, and the `gestalt` manager CLI.

```sh
npm ci
npm test
npm run build
```

Use `BASE_PATH=/gestalt/ npm run build` for the intended subpath deployment.

The manager installs a `workspace-git` Codex permission profile for development
sessions. It keeps writes scoped to the workspace (including Git metadata),
adds `/tmp` for test artifacts, grants read-only access to the isolated Codex,
Gestalt runtime, and user skill roots, and permits network access and loopback
listeners needed by local HTTP servers and Playwright.

Pushes to `main` deploy through `.github/workflows/deploy-pages.yml`. In the
GitHub repository settings, set **Pages → Build and deployment → Source** to
**GitHub Actions**. The workflow obtains the repository's actual Pages base
path from `actions/configure-pages`, runs the shell tests, builds VitePress, and
deploys the generated artifact.

Managed Serena places clangd compilation-database caches under
`<workspace>/.gestalt/serena/clangd/<source-name>-<path-hash>/cache/`.
The path hash separates databases from equally named source directories.
Workspace preparation refreshes managed copies of `compile_commands.json`
and selects them through clangd's isolated user configuration on Linux and
macOS; relative working directories are resolved against the original database.
Project `.clangd` database selections and their path conditions are retained.
This applies to managed Serena sessions and indexing commands after restarting
them with the updated manager. Existing source-tree caches are left in place.
Explicit `CompilationDatabase: Ancestors` fragments and manually maintained
user-config overrides retain clangd's native lookup/cache behavior; these are
not rewritten into a fixed build selection.
