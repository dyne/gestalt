# Unix compatibility producer and consumer handoff

`src/cli/platform/interactive.ts` separates platform selection from ownership.
`selectInteractiveRuntime` returns the proven Windows owned byte/terminal ports
on Windows. Linux and macOS return an explicit `unix-compatibility` capability
with the unchanged packaged legacy manager entry. They never select the
caller-only UID/PID namespace helper. Missing Windows helper artifacts fail;
there is no Windows Bash or unowned terminal fallback.

The Unix result has `legacyEntry` and `exec(invocation)`. It has no lease,
`claim`, `stop` or release result and must never be converted into an
`OwnedProcessPort` or an `apiVersion: 1` managed-command capability. Ordinary
legacy execution is governed by its existing launch, sandbox, scratch and
cleanup implementation. Actual uncertain managed lifetimes still retain their
resources; a direct child exit supplies no new complete-tree proof.

For a direct stdio-only CLI entry, `exec` replaces the current process through
Node 24's experimental `process.execve`. It preserves the current UID/PID
namespace, process creation identity, literal argv, environment and fd0/1/2.
The executable image changes to the selected program. Availability is checked;
missing support fails actionably with guidance to use the existing Unix entry.
No compiler, broker, cgroup delegation, debugger or ABI restriction is added.

Replacement discards JavaScript cleanup callbacks and other descriptors.
Select and delegate **before** acquiring JS resources, writing buffered output,
consuming input, or borrowing scratch/transaction leases. Do not call it from
an imported continuation API or an established private-descriptor protocol.
Those paths must retain their existing Unix launcher via `legacyEntry`.
The artifact must be the separately packaged unchanged `public/gestalt`, never
the new compiled Node entry (which would recurse).

L4 owns consumer wiring. Its direct entry/composition must select Unix
compatibility before its frozen `RuntimeCommands`, `SetupContext` or interactive
handler instantiates `NativeOwnedProcesses`. It must retain accepted Agents'
standalone Unix behavior rather than manufacture a managed lease or change
Agents defaults. Imported update continuations keep their existing Unix flow;
they must not replace their host process through this entry-only capability.
No L4 source is implemented or copied by this producer change. Packaging owns
the legacy artifact location and byte/mode equivalence.

Producer tests exercise literal/binary forwarding, actual creation identity and
namespace views, real three-fd terminals, cancellation, and the unchanged legacy
manager's help/retirement behavior. The explicit Unix authority gate imports the
exact accepted Org scripts, calls the real canonical guard/publication and the
entire `NativeLocalIpc` authenticated exchange against a distinct host process.
Raw credential measurements are recorded separately; they cannot substitute for
successful guard/publication/whole-IPC checks. A missing prerequisite fails that
gate. It never trusts unmapped UID 65534, remaps caller TMP or relaxes ancestors.

The complete installed CLI→Codex→Org/IPC consumer slice remains L4-owned.
Producer execution is not evidence of that consumer integration. Windows
producer evidence remains reusable only while its source, harness and runtime
inputs stay bound to the recorded successful target run.
