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

The `Publish npm package` job in `.github/workflows/deploy-pages.yml` follows the
tested manager release produced by the existing Pages build job. It checks out
the exact `vMAJOR.MINOR.PATCH` tag, builds and verifies the npm payload, and then
publishes it publicly with the `latest` tag. Publication runs only for `main` in
`dyne/gestalt`, independently of the Pages deployment job.

As in Gestalt Mobile, authentication uses npm trusted publishing through GitHub
Actions OIDC. The publication job has `contents: read` and `id-token: write`.
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
| Allowed action | Publish |

## First publication

npm requires a package to exist before its trusted publisher can be configured.
An authenticated npm maintainer must bootstrap the new package once. Before
merging the packaging feature, use a clean checkout of its verified commit. The
bootstrap package uses the current manager version; merging the feature then
creates the next manager release for the first automated OIDC publication.

```sh
npm login
npm run build:package
npm run test:package
npm publish ./dist/npm --access public --tag latest
```

Then configure the trusted publisher in the package's npm settings, or use npm
11.15 or newer with account 2FA enabled:

```sh
npm trust github @jaromil/gestalt --repo dyne/gestalt \
  --file deploy-pages.yml --allow-publish
```

Configure trust before merging the packaging feature, then let its next release
publish through OIDC. Later releases need no npm login or long-lived publish token
in CI. The first successful OIDC publication must
occur within npm's configuration-expiry window; see the current
[npm trusted publishing documentation](https://docs.npmjs.com/trusted-publishers/).

## Retries

The registry check skips an already published version only when its package
name, version, and `gitHead` match the checked-out release. An existing version
from another commit, an authentication failure, or a registry outage fails the
job. Do not overwrite versions or move release tags to force a retry.

If npm publication fails after Pages or tagging succeeds, rerun the workflow.
It rebuilds from the same release tag and retries the missing npm publication.
