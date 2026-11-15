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

On Windows 10 or later the target is created atomically inside an unnamed Job
Object with `PROC_THREAD_ATTRIBUTE_JOB_LIST`. This avoids the suspended-process
gap before a later `AssignProcessToJobObject`. The job has kill-on-last-close and
both breakaway options disabled. Only target stdin/stdout/stderr handles enter
`PROC_THREAD_ATTRIBUTE_HANDLE_LIST`; the job and private control-pipe handles are
not inherited. Controller-channel EOF stops the tree; a helper crash closes the
last job handle. Nested-job failures refuse launch rather than requesting
breakaway. A private helper console permits best-effort CTRL_BREAK to the owned
group; escalation uses `TerminateJobObject`. Release requires successful active
process accounting of zero on that same job handle, a root-handle result, settled
streams and a successful helper exit. Parent exit, `taskkill`, process-name
matching and elapsed time are not release evidence. See Microsoft's
[atomic job assignment](https://devblogs.microsoft.com/oldnewthing/20230209-00/?p=107812/)
and [Job Objects](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects).

On Linux the helper creates new user/mount and PID namespaces, maps only the
current UID/GID, sets no-new-privileges and mounts a private proc view. It refuses
launch if these kernel operations are unavailable. A small namespace init starts
the command and reaps its root; descendants may fork, create sessions/groups or
enter nested namespaces and still belong to the enclosing PID namespace.
Controller death kills the helper through `PR_SET_PDEATHSIG`; helper death kills
namespace init. A private liveness pipe closes the pre-prctl race before target
creation. Graceful stop signals within that private PID namespace; escalation
kills its init. Release requires reaping that exact direct child. Linux's
[`zap_pid_ns_processes`](https://github.com/torvalds/linux/blob/master/kernel/pid_namespace.c)
disables PID allocation, kills and waits for remaining members before init can
be reaped. No host process-group scan or PID existence check supplies this proof.
Namespace support is a runtime prerequisite, not permission to change host
settings or run an unconfined fallback. Managed processes cannot elevate
credentials; inherited session scratch variables remain unchanged.

Namespace creation alone is insufficient: policy must permit the capabilities
needed for identity mapping and the private PID/proc setup. For example, Ubuntu
can transition a newly created user namespace into an AppArmor profile that
denies those operations. Refusals report the setup stage, errno, requested/current
IDs, proc ownership and active policy; unknown results retain scratch. Runtime
does not change sysctls, install profiles, or retry outside confinement. The
disposable GitHub-hosted Ubuntu test job uses the same narrowly scoped userns
prerequisite setup as the existing full-PR workflow and records before/after
values. Evidence from that prepared runner does not establish support under a
different production policy.

These ownership units cover actual OS descendants. Work delegated to an existing
external broker/service is outside that ancestry and requires its own lifetime
contract; consumers cannot infer its absence from this lease. The adapter is a
lifetime boundary, not a filesystem/network sandbox against arbitrary same-user
or administrator interference.

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
