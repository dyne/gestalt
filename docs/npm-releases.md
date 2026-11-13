# npm release operations

`@jaromil/gestalt` packages the standalone Bash manager; it is not a TypeScript
rewrite. `npm/gestalt` resolves npm's executable symlink and replaces itself with
the bundled manager, retaining arguments, standard input, signals, and exit status.
There are no runtime npm dependencies or installation hooks. Setup remains an
explicit `gestalt install` operation.

## Release journey in Bash

Prepare releases on Linux or macOS with Bash, Node.js 24 or newer, and Git.
Use npm 11.15 or newer for staging; the commands below explicitly select a
known staging-capable npm client without replacing your global installation.

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
npm install -g @jaromil/gestalt
gestalt --version
gestalt install
```

`gestalt install` is explicit setup of managed components, not an npm lifecycle
hook. Keep the staged version, the approved/public version, and the installed
version distinct when checking progress.

## Build and verify

```sh
npm run build:package
npm run test:package
npm pack ./dist/npm --ignore-scripts
```

The build creates `dist/npm` from an explicit list: the launcher, the canonical
`public/gestalt` manager, the npm README, the license, and package metadata. The
package version comes from `GESTALT_CLI_VERSION`; `gitHead` records the checked-out
commit. The VitePress development package remains private.

The smoke tests pack and install the actual tarball into an isolated npm prefix,
offline and with lifecycle scripts disabled. They verify the installed command,
symlink handling, argument and stdin forwarding, exit status, and preservation of
npm-owned files during component updates.

## Automatic releases

The `Stage npm package` job in `.github/workflows/deploy-pages.yml` follows the
tested manager release produced by the existing Pages build job. It checks out
the exact `vMAJOR.MINOR.PATCH` tag, builds and verifies the npm payload, and then
submits it with `npm stage publish ./dist/npm --access public --tag latest`.
It appears in the maintainer's **Staged Packages** page until the maintainer
approves it with 2FA. CI never approves a staged version or runs direct
`npm publish`. Submission runs only for `main` in
`dyne/gestalt`, independently of the Pages deployment job.

As in Gestalt Mobile, authentication uses npm trusted publishing through GitHub
Actions OIDC. The staging job has `contents: read` and `id-token: write`.
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
| Allow direct `npm publish` | Unchecked |
| Allow managing dist-tags | Unchecked |

Enter only `deploy-pages.yml`, not `.github/workflows/deploy-pages.yml`. The npm
job has no GitHub environment; `github-pages` belongs to documentation deployment
and must not be entered here. The staged version already carries its `latest`
tag, so CI does not need permission to manage dist-tags separately.

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

Only stage-publish permission is needed; direct publishing should remain disabled
for this trust relationship. Later staging jobs need no npm login or long-lived
publish token in CI. Run the next CI staging job within npm's trust-configuration
expiry window; see the current
[npm trusted publishing documentation](https://docs.npmjs.com/trusted-publishers/).

## Review and approval

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

Save the trusted-publisher configuration on npm, then merge the staging workflow
change after its PR checks pass. A conventional `fix` or `feat` on `main` produces
the next version and tag through the existing Pages workflow. The `Stage npm
package` job builds that exact tag and submits it through GitHub OIDC. No npm
token secret or reusable Mobile credential is needed.

For a fresh manual run after the workflow is on `main`:

```bash
gh workflow run deploy-pages.yml --repo dyne/gestalt --ref main
gh run list --repo dyne/gestalt --workflow deploy-pages.yml --limit 5
```

Do not rerun a pre-fix workflow attempt expecting it to use the new staging
command: reruns retain the old workflow revision. Start a fresh run of `main`.
Watch for `Stage npm package` to succeed, then review and approve the resulting
version on npmjs.com. CI never performs approval.

A new trusted-publisher connection initially shows **Pending validation**. Make
the first successful CI submission before the deadline displayed by npm, then
confirm validation in npm settings. A local/manual staging request establishes
the package but does not demonstrate that the GitHub OIDC connection works.

## Retries

The registry check skips an already approved version only when its package
name, version, and `gitHead` match the checked-out release. An existing version
from another commit, an authentication failure, or a registry outage fails the
job. Do not overwrite versions or move release tags to force a retry.

If staging fails before submission after Pages or tagging succeeds, rerun the
workflow. It rebuilds from the same release tag and retries staging.

Staged and approved versions share the same version namespace. If a previous
attempt already staged the version, npm rejects a duplicate submission: inspect
and approve the existing item instead of repeatedly rerunning CI. OIDC staging
credentials cannot run `npm stage list`, `view`, or `approve`, so CI does not try
to inspect, replace, or approve pending versions. Do not interpret a missing
public registry version as proof that no staged version exists.
