# Pinned Impeccable Live adaptation

`remote-public-url.patch` is the accepted L2 diff, SHA256
`b55136bf55c3225fc39292c43bb332ac8f17f39164b760348a01f56e50f61c25`.
Upstream source remains owned by pbakaus/impeccable (Apache-2.0).
The patch was AI assisted. No formatting changes are authorized.

CI fetches public base `778c8a7b71ccd5bfe3ca6ac68c15d9d872d0f87d`, verifies
and applies this patch, then checks the reconstructed Git tree and Cargo/Bun
lockfile hashes in `provenance.json`. The accepted adaptation commit is a local
identity reference, **not** an available public fetch target.

The toolchain is Rust 1.99.0, Bun 1.3.13, Node 24.21.0 and Linux x86_64 musl.
Pinned Debian musl/fonts archives are checksum verified and extracted into
disposable CI storage; they are not installed on a user's host. The runner image
and compiler identity are retained in output provenance; byte-identical rebuilds
across changing hosted images are not claimed.

Existing upstream Rust workspace, source-first build, default npm and full opt-in
Live E2E tests run in CI. Only these two exact pinned-baseline opt-in failures may
be classified as known limitations; no tests are removed or skipped:

- Astro Vite7 core: stylesheet HTTP500 at `astro&type=style&index=1`.
- Vite8 React plain headless manual Apply: secondary-action copy remains
  original instead of “Secondary duplicate action”.

Any different failure or incomplete result blocks artifact publication. Reports
retain actual test/failure/skip counts rather than calling the opt-in suite green.
Manager integration gates retain their own independent full-suite requirement.
