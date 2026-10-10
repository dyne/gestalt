# Gestalt documentation

The Dyne-styled VitePress documentation hub for Gestalt Agents and Gestalt
Mobile. It includes the onboarding journey, operational guides, copied source
documentation, a one-line installer, and the `gestalt` manager CLI.

```sh
npm ci
npm test
npm run build
```

The managed Impeccable Live adaptation initially supports Linux x86_64. The
manager downloads the verified [CI-built prerelease](https://github.com/dyne/gestalt/releases/tag/impeccable-gestalt-live-1)
by default during shared setup or `gestalt impeccable install`. The archive,
binary and adaptation manifest hashes are pinned from runtime CI run 38046014336.
A matching downloaded archive can also be installed without network access:

```sh
gestalt impeccable install
gestalt impeccable install --artifact /absolute/path/impeccable-gestalt-live-1-x86_64-unknown-linux-musl.tar.gz
gestalt impeccable status
gestalt impeccable doctor --json
gestalt impeccable path
```

The installer design pins the archive, binary and adaptation manifest SHA256, verifies
source/patch identity and license notices, and probes native engine 0.1.11 and
CLI 4.0.0 (the upstream npm manifest is 4.1.0). It stages private immutable release
files below `$GESTALT_HOME/impeccable`, then atomically promotes `active.json`.
`gestalt impeccable update --artifact PATH` revalidates the same pinned release;
failed downloads, checksum checks and probes retain the old active descriptor.
Prior releases remain available to existing owners. No project hooks, first-run
downloads, global installs or sudo are involved.

`GESTALT_IMPECCABLE_ARTIFACT` supplies the same pinned archive to `gestalt install`
and `gestalt update`; a credential-free HTTPS URL is also accepted. Without an
override, shared setup uses the published pinned release. Set
`GESTALT_IMPECCABLE_ENABLED=0` to disable installation and report the runtime
disabled. Download and verification failures preserve the previous runtime;
the manager never substitutes a local candidate, unpatched upstream release or
executable found on `PATH`. `gestalt impeccable uninstall` removes private
runtime files and preserves project Live journals.

`gestalt impeccable doctor --json` reports `ready`, `reason`, native versions and
public-URL/chat-copy/poll-protocol capabilities. Each identity/help probe has a
three-second timeout; doctor never starts Live or prints probe output, tokens or
environment values. Disabled health skips execution entirely. A ready runtime
still reports `integrationReady: false`: Caddy, DNS, authorization and the running
app have separate readiness gates.

`.github/workflows/impeccable-runtime.yml` reconstructs the accepted adaptation
from a pinned public upstream base plus the checksum-verified patch under
`vendor/impeccable/`, builds Linux x86_64 musl, verifies bounded real startup,
health and clean shutdown, and uploads
archive/checksum/provenance artifacts. PR jobs have read-only repository access.
Publication runs when the exact `release/impeccable-gestalt-live-1` branch is
created at a reviewed commit. The Action builds and publishes before a main merge,
creating the immutable `impeccable-gestalt-live-1` tag and prerelease itself.
The exact tag trigger and an explicit manual input on `main` are also supported.
All routes require green same-run runtime and manager
checks and the `impeccable-release` environment, using a separate job that executes
no downloaded source or binary. It
publishes an immutable prerelease, never the latest manager release. Existing
assets must match before missing assets are uploaded; conflicting tags/assets
fail without overwrite. Configure required reviewers on the `impeccable-release`
environment and permit the exact release branch/tag in its deployment rules before
activation. An existing tag must resolve to the current CI commit; it is never moved.

`.github/workflows/manager-tests.yml` runs unchanged `npm test` on a disposable
hosted runner with the existing native-sandbox prerequisites. It has no version
commit, tag, push or Pages deployment. The existing Pages workflow is preserved.
The build/start inputs are documented in `vendor/impeccable/README.md`. Full
upstream Impeccable Rust, npm and browser suites are not run in Gestalt CI.
A failed build/start smoke or manager check blocks publication.

Set `GESTALT_IMPECCABLE_ENABLED=0` to disable runtime availability. Status and
doctor describe only the managed runtime; they do not claim Caddy, DNS, TLS,
authentication or a development app is ready. `gestalt impeccable uninstall`
removes private component files and preserves every project's `.impeccable/live`
journal. It does not stop project servers or terminate unrelated processes.

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
