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

Playwright MCP is an optional managed package for Codex clients. Install it
explicitly with `gestalt playwright install`, or pin a stable release with
`gestalt playwright install 0.0.83`. Node 20 or newer and npm are prerequisites.
`gestalt playwright update [VERSION]` stages and validates a release before
atomically activating it. Previous releases stay available to running sessions;
failed or interrupted staging preserves the active release. `gestalt update`
updates Playwright only when already installed; `gestalt update playwright`
explicitly installs or updates it. Installation disables all dependency lifecycle
scripts and browser download hooks, verifies registry integrity through a locked
`npm ci`, and checks the upstream CLI and actual MCP catalog before activation.

Chrome belongs to the operator: install and maintain Google Chrome or Chromium
outside Gestalt. `gestalt playwright doctor --json` reports package, Node and
Chrome readiness separately, without downloading, repairing or creating state.
Supported discovery locations are `/opt/google/chrome/chrome`, the usual
`/usr/bin/google-chrome[-stable]` and `/usr/bin/chromium[-browser]` paths on Linux,
and Google Chrome or Chromium under `/Applications` on macOS. Use
`--executable-path /absolute/path` on doctor to inspect another installation
(including paths with spaces). The executable must identify itself as Chrome or
Chromium, version 120 or newer; browser launch compatibility is also checked by
the browser smoke gate. Missing or incompatible Chrome never triggers a download.

`gestalt playwright version --json` is the read-only schema/contract version 1
installation contract: package version, registry integrity, release path, locked
dependency digest, executable digest, Node requirement and the standard tool
catalog. Clients must use the manager launcher rather than invoking npm/npx or
resolving `@latest` during session startup. Private package metadata lives under
`$GESTALT_HOME/playwright` (directories 0700, metadata 0600). A concurrent installer
fails with a bounded diagnostic; after a killed installer, verify that its
processes have exited before removing a stale `.install-lock` directory.

The host starts a client connection through `gestalt playwright mcp --cwd
/absolute/project` (optionally `--executable-path /absolute/chrome`). This is a
stdio child-process entry point for the trusted host, not an agent-shell fallback.
Profile consent and supervisor/plan activation controls belong to the client
integration; installing this component does not enable access. Standard navigation, form,
snapshot and screenshot tools are available. The managed catalog and call
handler both exclude server JavaScript execution, page evaluation, browser
installation and page-supplied WebMCP tools. The upstream release's core group
contains unsafe execution tools, so capability flags alone are insufficient;
the manager enforces its standard tool allowlist before forwarding any call.

The launcher speaks stdio only and does not create an HTTP MCP or CDP listener.
Chrome uses Playwright's browser pipe and retains its native Chromium sandbox.
It accepts ordinary MCP calls without Codex-specific metadata and never launches
a nested `codex sandbox`. The trusted host owns this MCP process and its
filesystem authority: shell sandbox roots do not constrain host MCP access.
Clients must enforce MCP tool approvals, profile consent and session policy.
Chrome's sandbox protects browser execution; it does not provide project-scoped
filesystem permissions for MCP tools. Upstream environment overrides and Node
injection variables are not forwarded.

`gestalt playwright doctor --cwd /absolute/project --json` also returns the
canonical workspace and its managed `state`, `userDataDir` and `lease` paths
without creating them. State lives at
`$GESTALT_HOME/playwright/workspaces/<sha256-of-canonical-workspace-path>/`.
The host process needs write access to that state path and read/execute access
to the managed release and Chrome. This launcher does not alter host permissions.
Different project paths have
separate cookies, including projects with identical basenames. Workspace
aliases resolve to one identity. The personal Chrome profile is never used.

A private atomic lease admits one browser owner per workspace. Contention
returns a bounded browser-busy error and leaves the existing session running.
A lifetime-pipe supervisor reaps the owned process group on EOF, signals or
proxy crashes. Updates and shutdown retain Chrome state. Stale leases are
recovered only after both the recorded owner and its process group are gone;
partial/unknown ownership remains busy. Live Chromium locks are never removed.
After an exceptional runtime crash, investigate a persistent busy record before
manually removing it. Browser stderr is not relayed, so paths and
credentials do not appear in launcher diagnostics.

Run `npm run test:playwright:real` in an authorized integration environment with
`GESTALT_PLAYWRIGHT_REAL_HOME` pointing to an explicitly prepared managed test
installation and `GESTALT_PLAYWRIGHT_REAL_CHROME` pointing to externally
installed Chrome. This separate integration gate never installs prerequisites.
It checks actual stdio tools, localhost forms and screenshots, persistent cookies,
workspace isolation and unchanged Chrome executable bytes. Unit suites use a
fixture MCP server and do not establish real browser behavior. The real gate
retains Chrome's sandbox; it never disables sandboxing or installs prerequisites.

The [CLI browser integration](cli/index.md#optional-managed-browser) uses native
named-profile consent and root-supervisor requests. Enabling a profile alone
never launches Chrome. `npm run test:playwright:cli` exercises the actual pinned
native terminal, profile precedence, lifecycle, resume/fork, child authority and
bounded failure fallback. Its terminal fixture requires Linux `script`/`stty`.
Ordinary persisted threads without managed controls remain browser-unavailable;
native resume/fork cannot attach a new dynamic tool. Compatible managed sessions
retain their controls, with private schema/workspace records under
`$GESTALT_HOME/playwright/cli-sessions` (0700 directory, 0600 files). These records
contain no cookies or credentials and do not authorize access without profile
consent. See the CLI reference for the fresh-session fallback.
`npm run test:playwright:cli:plan` additionally requires
`GESTALT_PLAYWRIGHT_AGENTS_CORE` pointing to the matching accepted source Git
checkout's `org-plan-core.mjs` and its relative modules; it consumes that dependency
read-only and records its revision/hash.

Run `npm run test:playwright:cli:real` with the same prepared package/Chrome
environment variables as the launcher real gate. It navigates a local page,
fills a form and takes a screenshot through the actual CLI controller, checks
workspace contention and teardown, and verifies unavailable Chrome preserves
chat. Set `GESTALT_PLAYWRIGHT_CLI_ARTIFACTS` to an authorized evidence directory
to retain the fixture screenshot. It never installs or updates prerequisites.
