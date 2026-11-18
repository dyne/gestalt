These gzip files contain unmodified `public/gestalt` and `public/install.sh`
bytes from the released Git tags recorded in `manifest.json`. Gzip timestamps
are zero. The manifest binds each uncompressed fixture to its release commit,
Git blob, byte count and SHA-256. Tests verify these identities before use.

The upgrade tests serve the current standalone asset over local HTTP. A
`BASH_ENV` hook observes the final `main "$@"` invocation and substitutes only
component installer/launcher edges, plus explicitly injected staging faults.
The released parser, updater, cleanup, destination-local promotion and
`exec bash … __updated-manager` continuation execute unchanged. These tests
prove the upgrade protocol; they do not claim fresh external component or
model installation. Installed core/package behavior has separate target tests.
