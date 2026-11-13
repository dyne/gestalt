# npm release operations

`@jaromil/gestalt` packages the standalone Bash manager; it is not a TypeScript
rewrite. `npm/gestalt` resolves npm's executable symlink and replaces itself with
the bundled manager, retaining arguments, standard input, signals, and exit status.
There are no runtime npm dependencies or installation hooks. Setup remains an
explicit `gestalt install` operation.

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
| Environment | Leave empty |
| Allowed action | Stage publish only; disable direct Publish |

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
npm stage list @jaromil/gestalt
npm stage view STAGE_ID
npm stage approve STAGE_ID
```

The version becomes installable with its `latest` tag only after approval.
GitHub's staging job succeeding does not mean the package is publicly published.
See [npm staged publishing](https://docs.npmjs.com/staged-publishing/).

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
