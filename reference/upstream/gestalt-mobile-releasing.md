# npm release operations

Gestalt Mobile releases are produced only by the `Release` job in
`.github/workflows/ci.yml` after the exact `main` commit passes both required
jobs. Maintainers do not publish or create release tags from local clones.

## Repository configuration checklist

Configure the canonical `dyne/gestalt-mobile` repository as follows:

- Protect `main` and require pull requests.
- Require the stable `Verify` and `Package smoke` status checks before merge.
- Do not permit administrators or other roles to bypass failed required checks.
- Allow GitHub Actions to create releases and tags. The release job itself has
  only `contents: write` and `id-token: write`; all other jobs have
  `contents: read`.
- Configure npm trusted publishing for the unscoped `gestalt-mobile` package.
  Select GitHub Actions and set the organization to `dyne`, the repository to
  `gestalt-mobile`, and the workflow filename to `ci.yml`.
- Do not configure an npm token or a `NODE_AUTH_TOKEN` secret. The release job
  exchanges GitHub's short-lived OIDC identity for npm publication access via
  its `id-token: write` permission.

Verify these settings after changing the workflow or repository rules. A test
pull request with a deliberately failing required check must remain unmergeable.
Pull-request jobs retain read-only permissions and cannot request the OIDC token
used for npm publication.

## Version and release behavior

The first release is npm `gestalt-mobile@0.1.0`, Git tag `v0.1.0`, and a matching
GitHub Release. Later versions are selected by `ietf-tools/semver-action` from
conventional commits:

- `feat` and `feature` bump the minor version.
- `fix`, `bugfix`, `perf`, `refactor`, `test`, and `tests` bump the patch version.
- `BREAKING CHANGE` and the conventional `!` marker bump the major version.
- `docs`, `chore`, `style`, and unrecognized commit types do not release alone.

The workflow changes package versions only inside its runner. It publishes npm
first, then pushes exactly one annotated tag, then creates the GitHub Release.
It never changes the version committed on `main`.

## Trusted publisher changes and revocation

The workflow follows Zenroom's npm release technique: it installs the latest npm
client, then publishes through [npm trusted publishing](https://docs.npmjs.com/trusted-publishers)
without a long-lived npm credential. If the repository or workflow filename
changes, update the trusted publisher configuration on npm before the next
release. To revoke publication access, remove or replace that trusted publisher.
Then audit npm package versions and GitHub releases before retrying a release.

## Partial-release recovery

Rerun the failed workflow for the same `main` commit. The release policy inspects
npm, the exact Git tag, and the GitHub Release before mutation:

- If npm publication failed, a retry publishes normally.
- If npm succeeded but the tag is absent, a retry verifies npm `gitHead`, then
  creates only the missing tag and release.
- If npm and the tag exist but the release is absent, a retry at the tagged
  commit creates only the missing GitHub Release.
- If all three exist and match, the retry succeeds without mutation.
- Any package/version ownership conflict, mismatched commit, or unexpected
  pre-bootstrap remote tag fails closed and requires investigation.

Never delete or move an existing release tag to force a retry, and never
republish an existing npm version.

## Inherited tags

Some developer clones retain unrelated `v1.x` tags from the former Gestalt
repository. Never run `git push --tags` from such a clone. Push branches normally;
the workflow uses `git push origin "refs/tags/v$VERSION"` to push one exact tag.
Before `v0.1.0`, any unexpected remote `v*` tag intentionally blocks release so
maintainers can resolve the collision without importing the inherited history.
