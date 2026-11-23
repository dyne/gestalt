# npm release operations

`@jaromil/gestalt` bundles the Node CLI, the existing Unix Bash backend, and a
source-verified Windows x64 helper for the native core candidate. There are no
declared runtime npm dependencies. A global postinstall reconciles Gestalt
Agents and context-mode when lifecycle scripts are allowed. The compatible
Codex CLI is a prerequisite. Linux and macOS keep optional Mobile, Serena and
Playwright routes. Native Windows defers these features.

Public npm version `0.16.1` is a different Linux/macOS artifact. The locally
validated native candidate also identifies itself as `0.16.1`. Its package
supports `win32` and needs a new release version for delivery. Check the
published version and `os` metadata before giving Windows installation
instructions.

For npm 12 registry installs use `--allow-scripts=@jaromil/gestalt`. For a
local tarball allow its resolved `file:` identity instead. Use
`--ignore-scripts` to defer profile provisioning. See
[installation and rollback](../start/install.md#install-with-npm) for the commands.

## Native candidate acceptance

| Target | Validated coverage |
| --- | --- |
| Windows x64, Node 24.22.3, Codex 0.164.0 | Ordinary-account core install, npm 11.19.1 and npm 12.2.2 lifecycle, terminal and AppServer, installed Org and authenticated IPC, setup/update and rollback behavior, skills and scratch |
| Linux x64 | Existing Unix backend, installed npm package, native sandbox and sockets, full Manager suite, previous standalone upgrades |
| macOS arm64 | Existing Unix backend, installed npm package, previous standalone upgrades and installer checks. Process cleanup remains best effort |

The candidate validation leaves other Windows architectures, actual UNC paths
and optional native integrations outside the release scope. Native Windows
has Job-based managed ownership. If cleanup cannot be confirmed, retain the
affected state for inspection. macOS may retain detached processes.

Deliver the compatible Agents marketplace first. Validate its published
setup API and installed runtime against the Manager release target, then
approve that target for publication. The native candidate used an immutable
Agents snapshot. A newer marketplace checkout needs its own relevant-input
comparison and validation. Confirm the new version is available in the
registry namespace, including staged releases, before publishing.

## Routine releases

GitHub Actions publishes new releases directly to npm through OIDC after the
approved release enters `main`. Review the exact release and dependency inputs
before merging. The existing trusted publisher must allow direct
`npm publish`. Keep its workflow filename `deploy-pages.yml` and environment
name empty.

After the release job succeeds, verify the public version:

```bash
npm view @jaromil/gestalt version dist-tags os gitHead --json
npm install -g @jaromil/gestalt@latest --foreground-scripts --allow-scripts=@jaromil/gestalt
gestalt --version
```

## Initial staged release journey in Bash

The commands in this section record the manual bootstrap process used before
trusted publishing was configured. They also apply to an explicitly chosen
manual staged release; routine CI releases now publish directly.

Prepare releases on Linux or macOS with Bash, Node.js 24 or newer, and Git.
Use npm 11.15 or newer for staging; the commands below explicitly select a
known staging-capable npm client without replacing your global installation.

The following steps describe the original Unix-only artifact. Use the current
build and verification process below for a native-capable release.

For the initial release, build the verified tag in a separate checkout:

```bash
git clone --depth 1 --branch v0.14.0 https://github.com/dyne/gestalt.git gestalt-release
cd gestalt-release
npm run build:package
npm run test:package
node -p "JSON.stringify(require('./dist/npm/package.json'), null, 2)"
```

Detached HEAD is expected when checking out a release tag. Packaging uses only
Node's standard library and Git, so `npm ci` is not needed for these commands.
Check the generated name `@jaromil/gestalt`, version, `gitHead`, `bin.gestalt`,
public access, and repository URL before submission. The actual package is the
`dist/npm` directory; `npm pack ./dist/npm` can also produce a reviewable tarball.

Authenticate and submit the package for review:

```bash
npm login
npx --yes npm@11.19.1 stage publish ./dist/npm --access public --tag latest
npx --yes npm@11.19.1 stage list @jaromil/gestalt
```

Configure the npm trusted publisher using the fields below. Then inspect the
staged version in **npmjs.com → account settings → Staged Packages**, or use its
reported stage ID:

```bash
npx --yes npm@11.19.1 stage view STAGE_ID
npx --yes npm@11.19.1 stage approve STAGE_ID
npm view @jaromil/gestalt version dist-tags --json
```

Approval requires your 2FA. After approval, check installation on a supported host:

```bash
npm install -g @jaromil/gestalt@latest --foreground-scripts
gestalt --version
gestalt install
```

The global npm install above automatically reconciles Agents. `gestalt install`
also provisions Mobile and the optional collection. Keep the staged version,
the approved/public version, and the installed
version distinct when checking progress.

## Build and verify

Use a clean checkout of the intended release commit, Node.js 24 and a
lockfile-based `npm ci`. For a Windows-enabled manifest, first put the helper
and its provenance JSON from a Windows x64 build of the same three native
source files in `dist/cli/native`. The package builder verifies their identity.

```sh
npm run build:package
npm run test:package
npm pack ./dist/npm --ignore-scripts
```

The build creates `dist/npm` with the launcher, compiled CLI, canonical Unix
backend, Windows helper and provenance and license, post-install script, npm
README, root license and package manifest. The package version comes from
`GESTALT_CLI_VERSION`. The manifest's `gitHead` records the checked-out
commit. A preview from a working tree with changes therefore needs a separate
source and payload digest. Release provenance comes from a clean, verified
release checkout. The VitePress development package remains private.

The smoke tests pack and install the actual tarball into an isolated npm prefix,
offline and with lifecycle scripts disabled. They verify the installed command,
symlink handling, argument and stdin forwarding, exit status, and preservation of
npm-owned files during component updates.

Additional lifecycle tests enable the actual npm post-install hook against a
stubbed Agents installer: initial and repeated global installations reconcile
Agents, installer failures fail npm with recovery guidance, and `--ignore-scripts`
and non-global installation leave profile provisioning explicit. Nested runtime
npm operations do not inherit the outer global install's prefix or global mode.

## Automatic releases

The `Release npm package` job in `.github/workflows/deploy-pages.yml` follows the
tested manager release produced by the existing Pages build job. It checks out
the exact `vMAJOR.MINOR.PATCH` tag, builds and verifies the npm payload, and then
publishes it with `npm publish ./dist/npm --access public --tag latest`.
Successful publication makes the version available immediately with the
`latest` tag. Publication runs only for `main` in
`dyne/gestalt`, independently of the Pages deployment job.

As in Gestalt Mobile, authentication uses npm trusted publishing through GitHub
Actions OIDC. The publishing job has `contents: read` and `id-token: write`.
There is no npm token secret to copy from Mobile. npm must separately authorize
this package and workflow.

Configure the trusted publisher for `@jaromil/gestalt` with:

| Field | Value |
| --- | --- |
| Provider | GitHub Actions |
| Organization | `dyne` |
| Repository | `gestalt` |
| Workflow filename | `deploy-pages.yml` |
| Environment name | Leave empty |
| `npm stage publish` | Always allowed |
| Allow direct `npm publish` | Checked |
| Allow managing dist-tags | Unchecked |

Enter only `deploy-pages.yml`, not `.github/workflows/deploy-pages.yml`. The npm
job has no GitHub environment; `github-pages` belongs to documentation deployment
and must not be entered here. Publication assigns the `latest` tag, so CI does
not need permission to manage dist-tags separately.

## First package registration

npm requires a package to exist before its trusted publisher can be configured.
The first authenticated staging request can create the package: npm publishes
only a `0.0.0-stage` placeholder, while the actual version waits for approval.
There is no need to publish the full package directly as a bootstrap step.

Use a clean checkout of the verified manager release tag (initially `v0.14.0`)
and npm 11.15 or newer. The npm account must have package publishing access and
2FA enabled. Staging itself does not prompt for 2FA:

```sh
npm login
npm run build:package
npm run test:package
npm stage publish ./dist/npm --access public --tag latest
```

If `npm stage` reports `Unknown command`, use a staging-capable client without
changing the global npm installation:

```sh
npx --yes npm@11.19.1 stage publish ./dist/npm --access public --tag latest
```

Then configure the trusted publisher in the package's npm settings, or use npm
11.15 or newer with account 2FA enabled:

```sh
npm trust github @jaromil/gestalt --repo dyne/gestalt \
  --file deploy-pages.yml --allow-stage-publish
```

This command establishes stage-publish permission for the bootstrap. Enable
**Allow direct `npm publish`** in npm settings before using the routine release
workflow. Later publishing jobs need no npm login or long-lived
publish token in CI. Run the next CI publishing job within npm's trust-configuration
expiry window; see the current
[npm trusted publishing documentation](https://docs.npmjs.com/trusted-publishers/).

## Review and approval of existing staged versions

Switching CI to direct publishing does not approve existing staged versions.
These instructions apply only to versions already staged or staged manually.

Open **Staged Packages** in npm account settings, inspect the version, and click
**Approve** with 2FA. Alternatively, use an authenticated maintainer session:

```sh
npx --yes npm@11.19.1 stage list @jaromil/gestalt
npx --yes npm@11.19.1 stage view STAGE_ID
npx --yes npm@11.19.1 stage approve STAGE_ID
```

The version becomes installable with its `latest` tag only after approval.
GitHub's staging job succeeding does not mean the package is publicly published.
See [npm staged publishing](https://docs.npmjs.com/staged-publishing/).

## Activate GitHub Actions

Save the trusted-publisher configuration on npm with direct publishing enabled,
then merge the direct-publishing workflow
change after its PR checks pass. A conventional `fix` or `feat` on `main` produces
the next version and tag through the existing Pages workflow. The `Release npm
package` job builds that exact tag and publishes it through GitHub OIDC. No npm
token secret or reusable Mobile credential is needed.

For a fresh manual run after the workflow is on `main`:

```bash
gh workflow run deploy-pages.yml --repo dyne/gestalt --ref main
gh run list --repo dyne/gestalt --workflow deploy-pages.yml --limit 5
```

Do not rerun an older workflow attempt expecting it to use the new publishing
command: reruns retain the old workflow revision. Start a fresh run of `main`.
Watch for `Release npm package` to succeed, then verify the public version with
`npm view @jaromil/gestalt version dist-tags --json`.

A new trusted-publisher connection initially shows **Pending validation**. Make
the first successful CI submission before the deadline displayed by npm, then
confirm validation in npm settings. A local/manual staging request establishes
the package but does not demonstrate that the GitHub OIDC connection works.

## Retries

The registry check skips an already published version only when its package
name, version, and `gitHead` match the checked-out release. An existing version
from another commit, an authentication failure, or a registry outage fails the
job. Do not overwrite versions or move release tags to force a retry.

If publishing fails after Pages or tagging succeeds, rerun the
workflow. It rebuilds from the same release tag and checks the registry before
retrying publication.

Staged and published versions share the same version namespace. If an older
workflow already staged the version, npm rejects direct publication of that
same version: inspect and approve the existing item or release a new version.
CI does not inspect, replace, or approve pending stages. Do not interpret a
missing public registry version as proof that no staged version exists.
