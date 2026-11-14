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

`NativePrivateStorage.open` creates a single private directory or validates an
existing one without changing its ACL/mode or removing unrelated entries.
Operations require a private root and private existing intermediate directories.
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
