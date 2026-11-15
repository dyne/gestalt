# Native process ownership candidate

`NativeOwnedProcesses` is a development adapter with a separate
`OwnedProcessPort`. Existing stream forwarding remains available for bounded
utility commands; it supplies no tree-lifetime proof. Neither adapter is wired
into the released Bash CLI or installed delivery yet.

`launch` accepts an absolute executable, token array, working directory,
environment and binary streams. It reuses the complete npm Node shim recognizer;
batch text and caller arguments are never evaluated by a shell. The returned
lease has a unique ID and `unknown`, `active` or `released` claim. Concurrent stops
share one operation. Grace and cleanup intervals are bounded; reaching a deadline
produces `unknown`, retains the lease and kills the controller helper. A late
message cannot change that uncertainty into release. `result` reports the observed
command exit/signal independently; if supervision ends before observation it reports
an unknown outcome. `completed` waits for helper/stream processing and reports
cleanup certainty. Callers await both to flush streams and preserve command status,
including when macOS cleanup remains unknown. Recovery from a saved lease
ID always returns `unknown`: a PID or an old JSON claim cannot reopen ownership
or authorize a signal. This adapter never deletes scratch. Consumers must retain
it on unknown and require the current lease's native empty proof before release.

Input forwarding has one acknowledged frame of at most 32 KiB in flight. Unix
target-input pipes are nonblocking and share the output/control poll loop through
write readiness; a child that writes large outputs before reading input cannot
block that loop's stdin write. The producer waits for each drained-frame
acknowledgement before submitting more input or EOF. Stop frames remain writable
while input is backpressured. EOF closes target stdin only after queued bytes
drain; an executable closing stdin early retires that pipe. Other input failures
refuse verification. Large simultaneous stdin/stdout/stderr contracts verify
exact byte order, EOF and command status on the actual targets.

Input is borrowed. Helper close cancels the adapter's pending input read and
removes its listeners before `completed` settles; it pauses Node Readables
without destroying them or consuming queued/future bytes. Caller listeners stay
installed. Generic async iterables receive `return()` on cancellation and must
cooperate to release their own pending I/O; a foreign unbounded `return()` cannot
hold native settlement open. Input completion still sends byte EOF or requests
terminal stop, and input errors request stop. This cleanup changes neither the
native absence proof nor the result-versus-completion contract.

Windows quoting counts the actual escaped UTF-16 command line, including its
terminating NUL, against the native 32767-unit limit. The public Node adapter
also launches an outer helper command line containing control arguments and the
target vector. That outer command line has its own Windows limit: a target vector
at the native limit can therefore fail safely before helper/payload launch with
`ENAMETOOLONG`. This adapter does not promise every 32767-unit target vector will
fit through the outer wrapper. Literal empty, Unicode, quote and backslash
arguments are preserved within that outer budget; no shell or alternate argv
transport is used. Native helper limit tests and outer-wrapper refusal tests
cover these separate boundaries.

`launchTerminal` is an additive `OwnedTerminalPort`; byte-stream `launch` keeps
its existing contract. Each terminal lease allocates a new POSIX PTY or Windows
ConPTY inside the same ownership lifetime. The target receives actual terminal
stdin, stdout and stderr. Supply the initial `{ columns, rows }` and await
`resize` to apply subsequent dimensions. Dimensions range from 1 to 32767.
Dimension errors reject asynchronously. Each resize acknowledgement is bounded
by the lease's cleanup deadline. A lost acknowledgement, control write failure,
or helper ending with an outstanding resize retains unknown ownership. Queued
resize requests reject after stop begins; resize never blocks the stop channel.
Terminal output is one merged byte stream, including stderr, echo, Unicode and
VT/control sequences. Consumers render those bytes and answer terminal queries;
they cannot recover separate stderr from a terminal stream.

Input is literal bytes with the same bounded acknowledged queue. Send control
characters explicitly: a raw-mode application receives them as input, while
normal terminal processing can turn Ctrl-C into an interrupt. Ctrl-D has the
application/terminal line discipline's meaning. A terminal has no independent
stdin half-close: ending the input iterable requests bounded owned stop after
the queued input drains. `stop` and controller EOF use the ownership mechanism,
independently of whether the application reads input. Windows ConPTY uses a
separate input worker so a stalled console cannot block control handling;
its output drains while bounded terminal close runs on another thread.

The core-first release preserves the existing Unix launch, sandbox, Org/IPC and
cleanup implementations through its compatibility backend. The development
Linux PID-namespace helper is not a compatible interactive runtime: caller-only
UID mapping can hide host-root owners, and host peer PIDs can become
unrepresentable. It must not wrap interactive Unix descendants. The
discriminated selection in `platform/interactive.ts` returns the legacy entry
and explicit stdio-only process replacement on Unix, and the owned producer on
Windows. See [the Unix compatibility handoff](native-unix-compatibility.md).
L4 consumer composition remains separate; producer tests do not establish the
complete consumer integration.
Direct-child compatibility execution supplies no managed complete-tree proof.
Actual unknown managed lifetimes still retain resources.

The experimental ptrace/seccomp diagnostic and exact pre-revision source have
been archived, removed from the candidate fixtures and removed from CI. It is
not a shipping backend or a release gate. There is no new Unix debugger/ABI
restriction, delegated cgroup, broker or service requirement.

Neutral fixed-asset helpers remain under `tests/fixtures/native-runtime/`.
Canonical Git modes and executable semantics are checked separately from local
umask bits; they never change the real Org/private-storage guard. Pinned Codex
0.164.0 uses frozen official metadata and fixed published SHA256 values, with
the original observation recorded separately from downloads. No live metadata
API/token is needed. The terminal exercise observes actual sign-in choices
without credentials or a model request.

The explicit Windows preflight runs before the complete standard-user suite.
It saves raw private-protocol stdout/stderr, independent Win32/Node target
reports, actual three-descriptor terminal detection, token, dimensions,
Unicode/control input, exit status and helper build provenance. ConPTY launch
clears the helper's standard-handle table while preserving its already captured
private protocol handles. A failed clear refuses before payload launch. The
primary must pass before a test-only legacy variant reproduces the original
pipe/TTY/framing failure, and a forced-clear failure verifies refusal. Variants
retain Job operations and never replace the primary. Corrupt protocol is retained without resynchronizing it into
release. Deadlines terminate only the actual owned helper; diagnostic teardown
is recorded separately from a successful scenario.

`scripts/test-platform-standard-user.ps1 -Lane windows-correction` first requires
the primary preflight, then focused ownership/terminal/harness contracts and the
pinned real Codex TUI. Full storage/native-platform, Unix and repository acceptance
stay explicitly unexecuted. The default lane still requires preflight, full native
contracts and the pinned TUI. A manifested temporary CI overlay can select only the
Windows correction job. Disabled repository/Unix jobs supply no acceptance
evidence. Stable acceptance runs the full affected gates again.

Opt-in build/case journals preserve UTC progress and distinguish helper process
exit from stream closure. The outer CI wait observes only its associated harness
process, bounds the diagnostic lane, and copies partial artifacts in `finally`.
An unobserved/null exit cannot pass. This harness wait supplies no descendant
absence proof; a timeout fails and retains unknown teardown for runner disposal.
The CI wrapper pins its associated process handle before waiting and verifies
known exit 0/23 fixtures through that same observation path. Null or mismatched
codes fail before running the native suite; they are never coerced to success.

Tree fixtures relay the detached leaf's actual identity through the existing
leaf/worker IPC channel before reporting readiness. Each worker validates its
child's PID, and the test still retains real process handles across stop. A
detached Windows child cannot supply its identity through the parent's console.
The missing-resize-ack fixture explicitly enables raw terminal input so Windows
window-input events can reach Node, observes actual application before its
unchanged deadline, and retains raw terminal output and target phase evidence.

macOS uses the approved best-effort lifetime contract. The native helper creates
a private session and keeps a dedicated group-anchor child alive. It uses
[`posix_spawn` group attributes](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man3/posix_spawnattr_setpgroup.3.html)
to put the command in that group before user code runs. TERM followed by bounded
KILL targets only that live, unreaped owned anchor's group; PID/group recovery is
never attempted. A separate private pipe makes the anchor attempt bounded group
cleanup if its supervisor dies. Controller-channel EOF also initiates cleanup.
The supervisor observes the actual command result separately from group cleanup.

This is useful native execution, not complete descendant containment. A child
can detach through a new group/session, and abrupt supervisor/anchor loss has no
established kernel kill-on-close guarantee. Group absence, parent exit and elapsed
time never become release evidence. Every completed macOS lease remains unknown,
including a normally exited command; scratch must stay retained. Stream draining
and stop attempts are bounded, so detached processes/output may outlive them.
Native code never emits the complete-tree release frame on macOS and TypeScript
rejects one there. The `active` state denotes a live execution/anchor, not a
certificate that no descendant escaped. No broker, privileged service, profile
installation or system-policy change is required. Windows/Linux keep their
stronger release guarantees.

The original MIT helper source and full license are in
`src/cli/platform/native/owned-process.c`. `npm run build:ownership` uses the
target's installed C compiler: `cc`/Clang on Linux/macOS or an installed MSVC/Windows SDK
discovered through Microsoft's `vswhere` on Windows. It installs nothing. Only
the validated installed `VsDevCmd.bat` path enters a fixed compiler-environment
command; candidate paths and managed argv remain native token arrays. Missing
tools fail actionably. Builds record source/artifact SHA256, exact compiler
arguments/version, OS, architecture, Node and UTC observation in the neighboring
`.provenance.json`. Tests build the exact current source, and CI preserves that
tested artifact's provenance separately from other builds. Consumers would use
prebuilt, verified target artifacts; packaging/cutover is a later milestone.

Real fixtures exercise resistant detached grandchildren, nested ownership,
graceful exit, root exit behind an explicit readiness gate, stdin EOF, concurrent
stops, launch failure, stale lease recovery, abrupt controller death during and
after launch, independent non-owned sentinels, binary streams and literal npm
shim arguments. A held output consumer deterministically withholds absence
verification until after the cleanup deadline, then releases and settles the
fixture; the claim remains unknown. Linux fixture identities and Windows retained
read-only process handles independently check fixture cleanup. Target/architecture
coverage and failed/prerequisite-limited executions must accompany any result.

macOS additionally exercises literal argv/binary streams/nonzero results, bounded
group stop, controller EOF and an actually detached survivor. Independent private
fixture channels observe cleanup and allow the survivor to exit voluntarily after
the unknown/retained-scratch assertions. The test never uses a stale PID kill as
teardown authority. Hosted Apple Silicon evidence and unexecuted Intel/other
scenarios are reported separately; there is no local Mac or exhaustive parity
prerequisite under the approved best-effort plan.
