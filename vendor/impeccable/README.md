# Pinned Impeccable Live adaptation

`remote-public-url.patch` is the accepted L2 diff, SHA256
`b55136bf55c3225fc39292c43bb332ac8f17f39164b760348a01f56e50f61c25`.
Upstream source remains owned by pbakaus/impeccable (Apache-2.0).
The patch was AI assisted. No formatting changes are authorized.

CI fetches public base `778c8a7b71ccd5bfe3ca6ac68c15d9d872d0f87d`, verifies
and applies this patch, then checks the reconstructed Git tree and Cargo/Bun
lockfile hashes in `provenance.json`. The accepted adaptation commit is a local
identity reference, **not** an available public fetch target.

The build toolchain is Rust 1.99.0 and Linux x86_64 musl; Node 24.21.0 runs
source preparation and packaging helpers. Pinned Debian musl archives are
checksum verified and extracted into disposable CI storage; they are not
installed on a user's host. Cargo builds directly from the reconstructed tracked
source/assets with `--locked`. It needs no Bun/npm install, browser, fonts or
Wasm target. The Cargo/Bun lockfile hashes still identify the accepted source.
Runner/compiler identity is retained in output provenance; byte-identical
rebuilds across changing hosted images are not claimed.

Gestalt CI builds the patched musl runtime, probes native version/protocol
identity, then starts that exact binary in a temporary project. The bounded
smoke checks real loopback health and public URL configuration, sends SIGTERM,
waits for exit0 and verifies server-record cleanup and closed listener. Private
server logs/tokens are discarded; only bounded JSON smoke evidence is retained
in artifact provenance. Full upstream Rust, npm and browser suites are not run
in Gestalt CI. Accepted L2 test evidence remains historical evidence.

The unchanged full Gestalt manager suite runs independently on a disposable
hosted runner and remains required for publication and integration acceptance.
