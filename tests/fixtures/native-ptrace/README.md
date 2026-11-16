These files are a bounded Linux ownership hypothesis and native-CI diagnostics.
They are never imported by the CLI, selected as a production backend or shipped
by the package builder. Production Linux still uses its PID namespace.

`prototype.c` includes the original MIT preflight/filter source. The fixture
programs cover per-thread/fork/vfork/exec behavior, CLONE_PARENT, detached trees,
creation-time tracer death, group stop/continue and real per-test kernel denial.
`run.py` records native commands, source/binary modes/digests, compiler/ABI,
actual pidfd observations, failures and an untouched non-owned sentinel. A
private inherited release pipe provides voluntary fixture teardown after failed
assertions; release is never used as evidence for the preceding ownership check.

The no-new-privileges/seccomp filter prevents CLONE_UNTRACED, rejects clone3/x32,
and rejects compatibility ABIs. Nested tracing is incompatible. The diagnostic
task table is limited to 1024 lifecycle identities; a failure exits with no
release claim. These restrictions need an explicit adoption decision and real
hosted compatibility evidence. No privilege, service or host policy repair is
performed by these programs.

`agents-source.json` binds the complete Org script subtree to accepted commit
e98a4cd15f945a7749d02e071126a854376b93c0. An already published immutable test
snapshot supplies equivalent script bytes; it does not become the accepted
whole-repository identity. The driver verifies every byte/mode/blob before
loading the unchanged guard/publication implementation. The fixtures write only
their generated private plan and status. `inherited.mjs` exercises the entire
real NativeLocalIpc exchange, wrong-creation rejection and independent same-socket
peer measurement; these are separate assertions.

The Codex helper uses official pinned prebuilt artifacts in private test state.
It checks the annotated tag's commit and fixed published artifact digest. Actual
interactive sign-in choices, not a banner/source assertion alone, establish that
the owned terminal passes the TUI precondition. Only terminal reports and Ctrl-C
are sent; no authentication choice, prompt or model request is submitted.
