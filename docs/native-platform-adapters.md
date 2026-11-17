# Native platform adapters

These adapters are development migration seams. The released Bash entry and
Linux/macOS package restriction remain in place. `npm run test:platform` runs
real host contracts; a Linux result does not establish Windows or macOS support.

`NativePaths` accepts native absolute drive/UNC paths on Windows and compares
canonical ancestor file identities. It does not infer containment from a
case-folded prefix, including in case-sensitive Windows directories. Missing
leaves resolve through their nearest existing ancestor. Device paths, alternate
data streams, reserved device names, trailing-dot/space aliases and drive-relative
paths fail explicitly. Storage rejects junction/symlink traversal even when the
canonical target is inside its root.

`NativeExecutables` searches absolute PATH entries in native order, with PATHEXT
on Windows, and excludes implicit current-directory search. `forwardCommand`
uses argv arrays and binary streams. Windows batch files require a shell in
[Node's process API](https://nodejs.org/download/release/v24.22.1/docs/api/child_process.html).
The adapter recognizes the complete plain-Node npm `cmd-shim` template, resolves
its target, and invokes the current Node binary directly. It never executes the
batch source or interpolates caller arguments. Unknown batch formats and Node
shebang options fail explicitly. This supports npm Node shims; arbitrary batch
programs require a separate, explicitly designed command contract. Unix npm
symlinks continue to resolve to executable files. Child-tree ownership belongs
to the separate ownership adapter; stream forwarding proves no tree cleanup.

The npm 11.19.1 Windows capture in `tests/fixtures/native-npm-node-shim.json`
preserves the exact 337 bytes and SHA256 from the standard-user native run.
Its PATHEXT assignment is inside the `ELSE` block. The recognizer accepts that
complete template and the previously supported final-line assignment form;
missing/duplicate assignments, changed interpreters and extra batch source fail.
Both forms resolve a plain Node entry and forward tokens through Node directly.

`NativePrivateStorage.open` creates a single private directory or validates an
existing one without changing its ACL/mode or removing unrelated entries.
Operations require a private root and private existing intermediate directories.
The lookup namespace is checked before creation and on every storage operation:
every ancestor must be owned by the current user or the trusted OS namespace
custodian and must refuse replacement by other unprivileged principals. Unix
rejects group/other-writable non-sticky ancestors; sticky OS temporary roots are
allowed. macOS also inspects ancestor ACL mutation rights. Windows checks owner
and DACL deletion, permission/ownership and attribute mutation rights, including
null DACL refusal and read-only verification of administrator owners. Unknown
owner/group/ACL evidence is a refusal. A protected root DACL or repeated inode
check alone does not establish this precondition. Existing unsafe directories
and their data/permissions are retained rather than repaired automatically.
Unix checks user ownership and excludes group/other mode bits. macOS also requires
the target-built probe to inspect extended ACLs, including private socket ACLs;
allow entries for other principals fail even if mode bits remain `0700`.
The probe distinguishes an absent ACL property after successful `lstatx_np` from
an error retrieving ACLs; errors never become privacy success. See Apple's
[ACL API](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man3/acl.3.html).
Windows uses
Windows PowerShell 5.1 and .NET Framework APIs to create a protected DACL at
directory creation and inspect every effective access entry. Only the current
user and LocalSystem may have allow entries; the owner must be the current user.
Failure to inspect permissions is a failure, rather than a mode-bit fallback.

Locks use exclusive creation and retained open-file identity. There is no timed
or PID-based lock theft. An interrupted controller can leave a lock/stage for
explicit recovery; the next invocation preserves it and reports contention.
Replacements create an exclusive random stage beside the destination, sync its
contents, and rename at the commit boundary. Unix syncs the destination directory;
Windows calls `MoveFileExW(REPLACE_EXISTING | WRITE_THROUGH)`. A failure before
promotion retains the working file and removes only this invocation's stage.
A failure after promotion can report uncertain durability with the complete new
file present; it never rolls back over a later transaction. Cooperative writers
use these locks; arbitrary concurrent modifications by the same user or an
administrator are outside the private-directory boundary.

The interruption contract uses a child-only I/O hold after a real partial stage
write and sync. IPC readiness establishes the live lock, private partial stage
and old destination before the child is killed. Teardown releases and settles
any surviving gate. Filesystem notifications and large writes are not used as
proof that interruption happened before promotion.

`NativeLocalIpc` uses local Unix sockets or managed Windows named pipes. It never
falls back to TCP. Unix checks private socket/directory ownership, then passes the
already-connected socket as inherited descriptor 3 to a small target-built probe.
The probe uses Linux `SO_PEERCRED` or macOS `LOCAL_PEERPID`/`getpeereid`, then reads
the server's OS creation identity and executable. All must match the expected
owner. Long session paths can use a relative socket basename in a child whose
working directory is the validated private directory; inherited TMPDIR and owner
context stay intact. Absolute socket paths over the portable limit fail clearly.

On Windows a fixed PowerShell/.NET client checks the connected pipe's owner and
allow entries, calls `GetNamedPipeServerProcessId` on that same handle, and
compares process creation ticks and executable before forwarding bytes. Default
pipe ACLs are insufficient: Windows documents that they can permit Everyone and
anonymous read access. Servers must create an explicit private DACL; the test
fixture does so. See [Microsoft's pipe security contract](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights).

The IPC `CommandStreams` object treats `stdin` as bytes received from the peer
and `stdout` as bytes sent to the peer. `stderr` rejects writes so diagnostics
cannot enter the protocol. Windows proxy stdin/stdout carries only binary data;
readiness/errors use a separate stderr channel.

## Native helper provenance

All helper source is original in this repository and licensed under MIT; the
complete license is in `src/cli/platform/native/unix-peer.c`. The same MIT terms
apply to `windows-system.ts` and the test-only PowerShell pipe fixture. No helper
binary, library, package or source is downloaded at runtime. API references are
design documentation, rather than vendored implementation.

`npm run build:platform` compiles the Unix read-only credential probe using the
target's C compiler. Node 24 runs the resulting executable; consumers do not
compile it. Build-time `cc`/Clang is required only for maintainers/CI. Target
artifacts must be built and tested separately for each OS/architecture before
packaging. The build writes `dist/cli/native/gestalt-unix-peer` and a neighboring
`.provenance.json` containing exact source/artifact SHA256, compiler/version,
argv, Node version, platform/architecture and observation time. Rebuild from the
same source/compiler/target with `node scripts/build-native-platform.mjs` to
reproduce the artifact. No cross-target binary is inferred from a host build.

The Windows source is a fixed string bundled into JavaScript. Requests are JSON
encoded separately in one child environment value; no request value becomes
PowerShell source. It uses built-in Windows PowerShell/.NET Framework, with
Reflection.Emit for two fixed Win32 P/Invoke signatures. No C#/TypeScript runtime
compiler is invoked. There is no persistent helper installation or separate downloadable DLL. Its
source SHA256 and framework/OS identity must accompany native test evidence.
Consumers need Windows PowerShell 5.1/.NET Framework in addition to Node 24.
Missing or policy-disabled PowerShell reports a concrete error; no bypass or
installation is attempted. Long paths use explicit extended Windows path syntax.

Native execution evidence is pending for Windows/macOS until actual runner
results are recorded. ARM64 and UNC-share execution remain separate target
coverage; path parsing/escape refusal alone does not prove an accessible share.

## Setup ownership and rollback

Direct Unix commands now enter the unchanged packaged legacy backend before
acquiring Node resources, as described in `native-unix-compatibility.md`.
The managed Node API details below describe that API's explicit ownership
contract; they do not replace the existing public Unix setup/cleanup backend.

The development Node setup/update commands use `NativeOwnedProcesses` for
installer and verification subprocesses. The canonical Agents setup API v1 runs
its filesystem orchestration in the host namespace and receives an explicit
asynchronous owned-command capability, environment, streams, cwd and cancellation
signal. Required host-filesystem/shared-context capabilities and selected source
digests are checked before calling it; missing APIs fail with an update instruction.
The whole Agents CLI is never launched inside the caller-only UID namespace.
Install/core update validate all existing managed homes first and create absent
homes through the private-storage adapter before running skills or Agents. An
existing permissive mode or ACL is rejected unchanged; setup does not repair it
with chmod or allow a subordinate installer to choose the root's permissions.
A command's exit status does not prove
that its descendants stopped. Configuration rollback and transaction cleanup
wait for the ownership adapter's release proof. Windows uses its Job Object;
Linux requires permitted unprivileged user, mount and PID namespaces. Missing
helpers or namespace authority fail explicitly; setup never falls back to an
unconfined child. The target-built ownership and ACL helpers are resolved beside
the compiled CLI, without environment-based helper redirection.

Inherited managed contexts register `manager-setup` borrower leases through
the existing v1/v2 publication and metadata fences. Only confirmed process
release permits borrower release. Unknown ownership leaves the borrower active;
elapsed time, parent exit and process lookup do not release it. The current
development runtime uses the shared host-side scratch metadata owner. Windows
validates native namespace/DACL privacy and privately creates new fences; it
does not substitute POSIX mode bits for an ACL. This boundary does not change the scratch schema or
relocate an existing owner/runtime root.

On macOS a successful command outcome remains usable under the approved
best-effort contract. Complete descendant absence stays unknown, so ownership
reports and setup snapshots remain retained. A failed or interrupted operation
with unknown ownership keeps its snapshot and manager transaction lock and
reports that rollback is unconfirmed. A later setup refuses the retained lock;
it never steals it by age or PID. Existing `recover-tool-leases` handling for
Serena/Playwright does not retire `manager-setup` borrowers. Inspection and any
manual recovery require all affected publishers and descendants to be stopped.
Managed macOS success also retains the manager operation lock and the canonical
Agents transaction locks/snapshots. It does not publish a released-setup provenance
record or automatically repeat/update that managed transaction. The successful
command outcome remains separate from that retained authority. Standalone Agents
setup retains its documented direct-child compatibility; it makes no complete-tree
claim and is never used as a managed fallback.

Standalone manager replacement keeps its serialization lock and cancellation
listener through the continuation. A native version declaration and update API
v1 marker identify the downloaded format; syntax success is never ownership
release. Promotion uses a destination-local stage and records the pending
handoff. The new compiled module imports without starting its CLI and receives
literal invocation tokens, cwd, copied environment, streams, cancellation and
the shared owned-command capability. Its filesystem owners remain in the host
namespace. Only external commands enter the native ownership backend.
Failure or interruption may leave the complete replacement installed; there is
no automatic executable rollback or crash-resume/exactly-once promise. Unknown
lifetime retains the update lock, receipt and download even after successful
syntax validation or continuation. Confirmed release allows cleanup of only
this update's private state. Managed macOS repeats may require explicit recovery
of this authority as well as the Agents setup authority.

## Development runtime composition

The Windows compiled development entry composes CLI/AppServer, diagnostics,
scratch and skills handlers in Node. Direct Unix entries delegate to the existing
packaged manager before acquiring resources or consuming streams. The extracted
profile/startup and managed MCP approval
policies keep explicit environment, cwd and stream inputs. External commands
use the same native lifetime port as setup; AppServer client EOF, command exit
and interruption remain separate outcomes. Unknown lifetime protects borrowed
scratch rather than inferring complete release from a PID, group or timeout.
The public Bash/npm entry and operating-system admission remain separate
delivery gates; building this artifact does not switch the installed command.

The consumer integration lane exercises the actual Codex binary, accepted
installed Org scripts, inherited scratch and authenticated host IPC. Windows
uses Job/ConPTY authority; Unix retains the existing launch/sandbox backend and
host identity. A caller-only mapped UID namespace is not a Unix compatibility
backend. A refusal must retain authority and explain the prerequisite; ownership
checks are never weakened to turn it into a pass.

Default Node path/version/doctor handlers read filesystem metadata and do not
launch commands, publish borrowers, initialize profiles or reconcile configuration.
Package metadata is labeled separately from a probed executable version.
Required CLI launch verification is a separate owned operation and preserves
the validated inherited context and TMP variables. `doctor --probe-sandbox`
explicitly requests the selected native Codex OS sandbox and a private isolated
home, never a Linux-only prerequisite on another OS. With an inherited session
temporary owner, the read-only diagnostic reports that the host-shell probe is
unavailable rather than altering its leases. An uncertain probe retains its
private data. Optional component installation metadata is distinct from live
Serena/Playwright readiness. Native Serena, Playwright and Mobile ports are
deferred in this core release; Windows diagnostics state this without requiring
Python, uv or a browser. Existing supported Unix delegates remain available.

The development Mobile command and successful restart handoff require a native
Mobile host with authenticated live restart authority. A descriptor and numeric
PID do not grant that authority. Until that host boundary is present, explicit
requests fail with a prerequisite, preserving CLI/Agents and the running relay.
The restart worker can finish an update without terminating the relay; it
reports the unavailable handoff and preserves its descriptor.

Skills collection filesystem transactions run on the host; only Git crosses
the owned command boundary. Unknown ownership retains the collection lock and
journal even after a successful command. Inspection yields a current inventory
token; explicit offline quiescence confirmation is required for recovery, never
PID-age inference. Collection maintenance and archive transfer fence each other.
Archive format 1 is read/written with Node tar/gzip handling. Validation covers
all members before extraction, including PAX/GNU pathname metadata, link and
special-file refusal, collisions and bounded input/payload sizes. Existing
unrelated skills remain in place. Imported selected entries use destination-local
candidates and exact rollback journals; package content is never executed.
Tar files retain only owner-execute intent: files extract as `0600` or `0700`
and directories as `0700`, without special or group/other permission bits.
POSIX executable resources survive extraction, import and re-export. Windows
private ACLs remain authoritative and cannot represent an independent POSIX
owner-execute bit. Byte preservation on Windows does not promise a POSIX-mode
roundtrip through Windows extraction/re-export; no metadata sidecar is invented.

Windows runtime scratch checks can opt into the packaged ownership helper's
separate `--storage-check-v1` read mode. It reads literal path owner/DACL and
reparse metadata using the current token, at every existing guard checkpoint.
Metadata comes from `GetFileAttributesW`; owner/DACL comes from a separate
`GetNamedSecurityInfoW` read, matching the existing path-based guard semantics.
These are separate snapshots with existing path/ACL race limits, not an atomic
path-to-object binding. The caller retains its existing Node identity checks,
private namespace checks and mutation fences; this read mode adds no confinement
or process-lifetime authority. Fixed diagnostic stages identify failures in the
current call without attributing an earlier generic guard error to an API.
It emits no ownership or release claim, keeps no cross-request validation cache,
and never changes ACLs or launches a target. Missing/mismatched helpers and
unsupported callback/object/conditional ACE representations fail with an
actionable prerequisite; no PowerShell read fallback or ACL repair is attempted.
The existing setup/default scratch factory and secure-directory creation retain
their fixed PowerShell checker. Unix paths and process/terminal modes are
unchanged. Actual native predicate parity and readiness require target evidence;
the protocol and source build alone do not prove them.

Native capability discovery uses a shared startup budget of 10 seconds on
Windows and 5 seconds on other platforms. This Windows allowance includes
validated owner/ACL bookkeeping for consecutive config and hook queries. Each
query retains its own cap of `min(5000 ms, remaining shared budget)`; the shared
deadline starts before launch and is not reset per query. This is a new bounded
Windows policy, not evidence that the earlier 5-second policy passed. Required
command captures keep their separate 10-second timers. Synchronous checks can
delay timer delivery, and owned cleanup can extend elapsed wall time beyond a
logical readiness deadline; this policy is not a hard whole-CLI runtime limit.
No guard, inherited temporary context, cleanup proof or hook-trust gate is waived.
