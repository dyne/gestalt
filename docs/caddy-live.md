# Live with an existing Caddy installation

The operator owns Caddy, DNS, certificates, firewall policy and service reloads.
Gestalt does not edit `/etc`, install Caddy or replace the operator's sites.
This bootstrap prepares the admin transport only. It does not enable Live or
expose a development server. Runtime installation, production Mobile auth,
admin isolation, the authenticated preview gateway and a registered loopback
app must all be ready before a preview route can be created.

## Choose the origins and bounded ports

For example, use `https://mobile.example.com` for Mobile and
`https://preview.example.com:9443` through `:9445` for three preview origins.
Replace these example names with operator-controlled names. Preview must have
a different hostname from Mobile, even when the ports differ: cookies are not
port-scoped. Keep the app at `/` and reserve `/__gestalt_live/` for the gateway.
Apps depending on upstream cookies are not supported initially.

Choose a small explicit inclusive port pool, such as TCP 9443–9445, and check
it against existing Caddy listeners and other services (`sudo ss -ltnp`).
Each origin stays assigned to one canonical app permanently, including after
Stop/restart; do not recycle it for another app or discard assignment records.
Exhaustion requires additional explicitly configured capacity. Caddy routes
will proxy only to the authenticated gateway, never directly to the app/helper.
No preview site blocks belong in this bootstrap; runtime route management owns
only namespaced Gestalt resources and must detect listener/route conflicts.

Create A/AAAA records for the preview hostname pointing to this server. Remove
an unusable AAAA record rather than relying on IPv4-only tests. Open only the
chosen TCP pool in the host and upstream firewall, for example with an existing
UFW policy: `sudo ufw allow 9443:9445/tcp`. Do not reset firewall rules. Keep
app/helper ports loopback-only and blocked externally, and never open TCP 2019.
The initial preview transport needs TCP; opening UDP for HTTP/3 is optional.

Caddy owns valid publicly trusted TLS for the preview hostname. ACME HTTP-01
and TLS-ALPN-01 use standard ports 80 and 443 even when the preview uses 9443;
preserve access to those ports or use an operator-configured DNS challenge with
the appropriate Caddy DNS module. Do not change global `http_port`/`https_port`
to preview ports, disable TLS verification or use `tls internal` for remote
production browsers. Bootstrap validation does not prove DNS, issuance or
remote TLS reachability. After authenticated routes are installed, verify each
origin from the browser's network with normal certificate verification; an
unauthenticated request must return denial with no app bytes.

## Restrict the admin boundary first

The supplied include sets `admin unix//run/caddy/gestalt-admin.sock|0600`.
The socket is owned by Caddy's service UID and accessible only to that UID/root.
Its parent must exist, be owned by the service UID, and deny access to project
users. On a conventional Linux service running as `caddy:caddy`, verify
`/run/caddy` is private (`0750` or stricter), is recreated at boot by the existing
service/tmpfiles policy, and is not writable by agents. If absent, the operator
can add `/etc/tmpfiles.d/gestalt-caddy.conf` containing:

```text
d /run/caddy 0750 caddy caddy -
```

Apply that file with `sudo systemd-tmpfiles --create
/etc/tmpfiles.d/gestalt-caddy.conf`. Adapt the UID/group to the actual service;
inspect existing directory ownership before doing so. Do not change certificate
storage permissions or add project users to the Caddy group.

The trusted controller needs a separately enforced boundary: a narrow broker
running as the Caddy service identity, or a managed sandbox rule that permits
only the trusted controller and denies the actual project/agent process. A
broker must authenticate the controller and accept only namespaced route
operations, configured ports and registered loopback gateway targets; it must
not accept arbitrary JSON, `/load`, shell commands or raw admin forwarding.
Do not run project tools or model children as Caddy/root, hand the socket to a
browser, mount it into an app container, or expose it through Mobile HTTP.
There is no TCP fallback, including localhost.

Mode 0600 alone cannot isolate a controller from an agent sharing its UID.
Before readiness, prove that the trusted controller can use the boundary and
the **actual effective project/agent process** cannot connect to the socket or
invoke its broker. Repeat the denial check after service/sandbox changes and
restart. This guide does not install a broker or prove isolation: until that
boundary and its denial evidence exist, Live remains not ready
(`LIVE_CADDY_ADMIN_UNISOLATED`). A temporary test under another UID is insufficient.

## Import without replacing existing configuration

Back up `/etc/caddy/Caddyfile` and any changed service override first. Copy
`examples/caddy/gestalt-live-admin.caddy` from this repository to
`/etc/caddy/gestalt-live-admin.caddy` as an operator-reviewed, root-owned,
non-project-writable file. Edit `/etc/caddy/Caddyfile` manually. In its existing
first global options block replace any existing `admin` option with this import;
keep every other option, snippet and site unchanged. If no global block exists,
add one before all sites:

```caddyfile
{
    # Keep existing global options here. There can only be one global block.
    import /etc/caddy/gestalt-live-admin.caddy
}

# All existing sites/imports continue below, unchanged.
```

Import exactly once. Do not import inside a site or import a second global
block. Caddy expands imports before parsing; repeated admin definitions can be
accepted silently, so successful adaptation does not prove a single import.
Relative imports resolve from the file containing the import. The include
is a global-options fragment; its standalone validation context is the `{ ... }`
block above, not the fragment by itself.

For the exact direct import above, check its count before adaptation:

```sh
sudo awk '$1 == "import" && $2 == "/etc/caddy/gestalt-live-admin.caddy" { n++ } END { if (n != 1) { print "Expected exactly one Gestalt admin import"; exit 1 } }' /etc/caddy/Caddyfile
```

Also inspect other imports (including globs) to ensure they cannot include this
file again or introduce another `admin` option. Keep it outside wildcard site
directories. The count check covers the documented direct import only.

Review an adapted candidate and validate it **before** loading it:

```sh
sudo caddy adapt --config /etc/caddy/Caddyfile --adapter caddyfile --pretty
sudo -u caddy caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
```

Adapted JSON can contain existing site secrets; inspect it privately and do not
paste it into logs. Validation provisions modules but does not start listeners;
use the service identity/environment so file and TLS storage checks are meaningful.
For an offline syntax fixture use HTTP-only unrelated sites and temporary storage,
as in `tests/caddy-bootstrap.test.mjs`; production validation may need configured
certificate files/modules. Confirm all unrelated sites still appear in the
adapted config. Validation failure means retain the current running config.

The first reload must address the **currently running** admin endpoint, not
the new one. For an existing default localhost admin endpoint, the operator can
use `sudo caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile
--address localhost:2019` once. If currently configured differently, use that
known address; do not enable TCP as a workaround for inaccessible admin.
Subsequent operator reloads use:

```sh
sudo -u caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile --address unix//run/caddy/gestalt-admin.sock
```

Use the address without `|0600` on the command line; the suffix is a listen-mode
setting. Check `sudo stat /run/caddy /run/caddy/gestalt-admin.sock`, the service
journal and existing sites after reload, and confirm TCP 2019 is no longer
listening. Update the existing service's reload command to the same explicit
socket address if it otherwise targets localhost. Preserve its original binary,
config, adapter, environment, privileges and other unit hardening. For the common
`/usr/bin/caddy` unit, the operator-reviewed `systemctl edit caddy` override is:

```ini
[Service]
ExecReload=
ExecReload=/usr/bin/caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile --address unix//run/caddy/gestalt-admin.sock
```

Then use `sudo systemctl daemon-reload`; this does not load Caddy configuration.
Adjust that example for the actual unit, not for a presumed installation.
Never reload a shared server with the standalone fixture or a generated Live-only
config. No automated installer performs these host operations.

## Persistence, reload and rollback ownership

Use the existing operator Caddyfile as the startup source. Dynamic admin JSON
changes affect the running config and Caddy's autosaved JSON; they do not write
back to the Caddyfile. A Caddyfile reload or restart from that file removes
runtime-only routes. Caddy `--resume` instead uses the autosaved config; do not
switch an existing service to it silently or mix startup sources. Preserve the
operator's existing `persist_config` policy and private Caddy storage. Autosave
is not the controller's ownership/assignment database or a backup of the Caddyfile.

Before reload/restart, quiesce Live and revoke leases through the controller.
The operator performs service changes; afterwards the controller rechecks
admin isolation and reconciles durable desired state. It may recreate only
owned namespaced routes, with conflict detection; it must never POST a complete
replacement config or touch unrelated sites. Preview stays unavailable until
auth, ownership and reconciliation succeed. Verify unrelated sites after each
change and restart. Losing private controller state requires recovery, not
reallocation of apparently free preview origins.

Rollback: Stop Live and verify owned streams/routes are revoked, close only
the newly opened preview firewall rules, restore the backed-up Caddyfile and
changed reload override, then adapt/validate the restored file. Load it through
the **current Unix admin socket** (the restored file may return admin to its
previous address). Remove the include/tmpfiles addition only after confirming
the restored service no longer needs them; never remove a shared runtime directory
or certificate store. Verify unrelated sites and the intended admin transport.
If admin is unavailable, the operator inspects the journal/socket/unit and may
perform a controlled restart from the validated full file; the controller does
not kill Caddy, reinstall it or overwrite configuration to recover.

## Read-only bootstrap diagnosis

Run this from the trusted operator/controller boundary with the configured
origins and pool, not as a project tool granted new socket access:

```sh
gestalt caddy doctor --json --mobile-origin https://mobile.example.com --preview-host preview.example.com --ports 9443-9445
gestalt impeccable doctor --json
```

`--admin unix//absolute/path.sock` selects an existing private Unix endpoint;
the default is `unix//run/caddy/gestalt-admin.sock`. No TCP address, mode suffix,
fallback or automatic elevation is accepted. Only `caddy version` and a bounded
`GET /config/` are used; doctor never adapts, loads, reloads, replaces or saves
config, starts a service, probes an app, binds a port or changes the firewall.
Version uses a three-second probe with 4 KiB output limit. Admin reads have a
three-second total deadline and a 256 KiB limit; oversized configurations need
operator/controller inspection, not an unbounded dump. Existing config and
probe output are never printed. Initial Caddy support is major 2, minimum 2.11.7.

The report separates `adminReady`, `configReady` and `portsReady`, rejects
non-0600 socket mode, writable non-sticky parent directories, insecure live
admin transport, shared Mobile/preview hostname and occupied pool ports.
The pool must be explicit, 1024–65535 and at most 16 ports. Listener checks read
Linux `/proc/net/tcp{,6}` and Caddy's current listener addresses; no reservation
or connection attempt is made to those ports. This is a point-in-time bootstrap
check, not a lease: the controller must recheck conflicts at allocation. An
existing active preview listener requires controller ownership/reconciliation
inspection and is conservatively reported as occupied by this bootstrap doctor.

Missing Caddy/socket, inaccessible admin, malformed/oversized responses, unknown
listener state and unsupported versions return bounded actionable reasons and
exit 1. Human output is available without `--json`. Runtime health is a separate
`gestalt impeccable doctor` check. DNS and TLS fields remain unverified, rather
than inferring success from a local admin connection. For DNS, inspect
`getent ahosts preview.example.com` and authoritative A/AAAA records. After the
controller safely creates authenticated routes, check each origin remotely
with certificate verification enabled, for example
`curl --max-time 5 -I https://preview.example.com:9443/`; expect authorization
denial with no app content. Do not supply bearer grants to these diagnostics.

This bootstrap doctor intentionally reports `ready: false`,
`integrationReady: false`, `isolation.ready: false` and `admin-unisolated` after
all other checks pass. The future controller/broker must prove denial from the
actual project/agent process and remote authenticated transport before enabling
Live. There is no `--isolated` assertion or evidence-file bypass. A private
temporary socket fixture is not that proof. The service owner follows the
reload/restart/recovery procedures above; doctor performs no recovery mutations.

References: Caddy's [import rules](https://caddyserver.com/docs/caddyfile/directives/import),
[admin and global options](https://caddyserver.com/docs/caddyfile/options#admin),
[Unix socket mode syntax](https://caddyserver.com/docs/conventions#network-addresses),
[adapt/validate/reload commands](https://caddyserver.com/docs/command-line),
[API persistence](https://caddyserver.com/docs/api) and
[service workflows](https://caddyserver.com/docs/running).


## Trusted host policy contract (version 1)

`gestalt live-policy probe|prepare|verify --manifest ABS` is a private,
controller-side interface for L10 packaged tools and L11 production admission.
It does not enable Start or grant browser/project/agent access to host RPC.
Normal `gestalt cli`, Mobile, Serena and existing doctor behavior is unchanged.
All three operations emit one bounded JSON object. Exit 0 means the policy
contract verified; exit 1 means unavailable. `schemaVersion` and
`contractVersion` are 1, `nativeVersion` is `0.161.0`, and `liveReady` is always
false. Success includes `policyDigest` and `configDigest`; preparation and
verification also return the private receipt path. Failure includes only
`schemaVersion`, `available:false`, `liveReady:false` and a fixed `reason`.
No config, credentials, command output or exception text is printed.

The trusted host-started Mobile controller builds the manifest from its current
resolved native state and trusted bootstrap configuration, not from browser,
project, MCP tool arguments or a sandbox metadata claim. The manifest is JSON,
mode 0600, in an owner-only 0700 directory outside every original writable
root. Its ancestors must be canonical and protected against replacement.
The controller's native policy must also protect this directory before accepting
untrusted work. A same-UID file mode is not an enforcement boundary: L9 host
proof and L11 admission must establish actual native denial. No command here
installs a service, edits user configuration or repairs a daemon directory.

The exact manifest fields are:

| Field | Version 1 input |
| --- | --- |
| `schemaVersion`, `profileId` | `1`, nonempty trusted resolved profile identity |
| `appRoot` | Existing canonical absolute app directory, without symlink aliases |
| `controller` | `{pid,birth,uid,namespaces:{mnt,user,pid}}`; birth is Linux `/proc/PID/stat` field 22, namespaces are `/proc/PID/ns/NAME` link values |
| `binary` | `{path,version:"0.161.0",sha256}`; canonical executable, outside writable roots, hash of installed binary bytes |
| `sandboxState` | Actual native `codex/sandbox-state-meta` shape: `permissionProfile`, `sandboxCwd`, `useLegacyLandlock:false`; optional `useMxc:false` and `codexLinuxSandboxExe:null` |
| `configFiles` | Nonempty `[{path,sha256}]` inventory of trusted effective configuration bytes, including prepared `runtimeHome/config.toml`; all private, canonical and outside writable roots |
| `components` | `{manager,mobile,agents}` with exact component versions/tree pins; changes invalidate previous receipts |
| `privatePaths` | `{auth:[ABS],controller:[ABS],caddy:[ABS],daemon:[ABS]}`; existing canonical protected paths, with private manifest directory in controller paths and `/tmp/codex-daemon-EUID` in daemon paths |
| `keyFile` | Private canonical file of exactly 32 random bytes below manifest directory; never forwarded |
| `runtimeHome` | Owner-only canonical directory below manifest directory |
| `environment` | Exactly `{PATH:"/usr/bin:/bin",HOME:runtimeHome,TMPDIR:"/tmp"}` |

V1 supports only managed restricted filesystem and restricted network state.
Filesystem entries use the pinned native shape
`{path:{type:"path",path:ABS},access:"read"|"write"|"deny"}`. Native special
root, minimal (read/deny only) and slash_tmp entries are accepted;
context-dependent project_roots and tmpdir entries must first be materialized
to absolute paths by the trusted producer; deny-only glob_pattern entries are retained.
Other states or special paths are explicitly incompatible, with no downgrade
or escalation. `sandboxCwd` is a native file URL resolving to `appRoot`.
Named profiles are resolved by the trusted owning runtime before preparing this
manifest; the manager never re-resolves a profile through ambient user config.
Missing profile, binary, protected path or config fails closed.

Composition preserves entries outside protected paths and adds covering deny
roots for private paths, the policy directory and `/proc`. Entries covered by
those roots are removed, including descendant grants that native most-specific
precedence could otherwise reopen. Redundant child denies are omitted because
native directory masks become read-only before child masks are constructed.
Every declared private category remains covered by a deny root.
It never adds readable roots, writable roots, enabled networking or credentials.
The running manager source is also measured: its canonical path, actual CLI
version and SHA256 bind receipts to the installed implementation. A manager
inside an original writable root is unavailable; replacing its bytes or version
invalidates receipts and triggers the worker monitor.
Private app overlap and initially project-writable policy/binary/config are
rejected. Symlink aliases, stale controller birth, mismatched UID/namespaces,
unknown schema fields and changed binary/config hashes are rejected.

The isolated `runtimeHome/config.toml` contains exactly:

```toml
[shell_environment_policy]
inherit = "none"
```

Project `.codex` directories anywhere in app ancestry and system Codex config
or requirements are unsupported in v1, with an explicit unavailable reason.
Existing ordinary non-Live workflows remain usable. This deliberately bounded
prepared config prevents hooks, plugin/MCP config and ambient shell policy from
being evaluated as launch authority. The trusted controller must inventory the
owning runtime's effective configuration separately; this sandbox CLI home is
not a replacement owning-session or model configuration.

`probe` validates paths, hashes, controller identity, directory namespace and
pinned binary `--version` under the isolated environment. It only diagnoses
contract compatibility, starts no project process and writes no receipt. A
masked mode000 native daemon directory yields `host-launch-unavailable`;
missing prerequisites yield `missing-prerequisite`. On a fresh host, the trusted
owning runtime may first run a bounded stock native `/usr/bin/true` using its
trusted named profile: native sandbox construction creates and validates the
reserved daemon root. The manager probe itself never creates it. Do not chmod the mask,
disable the sandbox, detach from an agent sandbox or use a second controller.

`prepare` atomically writes `policy-receipt.json` (0600) below the manifest
directory. It contains `{body,mac}`. `mac` is hex HMAC-SHA256 of recursively
key-sorted JSON `body` using `keyFile`. Body has `schemaVersion:1`,
`kind:"policy-contract"`, profile/manifest/policy/config digests, Linux `bootId`, exact binary
and component identities, app `{path,dev,ino}`, controller birth/namespace/UID,
the composed `sandboxState`, and `launch`. Launch specifies pinned executable,
`["sandbox","--sandbox-state-json",JSON.stringify(state),"--"]`, private cwd,
minimal environment plus `CODEX_HOME:runtimeHome` and `LANG:"C.UTF-8"`,
`stdio:["posix-pipe","posix-pipe","posix-pipe"]`, `lifetime:"per-call"`,
`extraFileDescriptors:false` and `credentials:false`.

`verify` authenticates that private receipt and recomputes every input.
Tampered content/key/MAC, controller restart, app inode replacement, profile,
permissions, config, private-path set, binary or component changes invalidate it.
The receipt is a **policy contract**, not evidence that a subprocess is confined.
L9.2 must issue process-bound launch evidence with actual leader PID/birth,
UID/namespaces and policy/config/component digest, verify it before reuse, and
reap only owned process groups/descendants. L10/L11 may never admit an existing
process by policy receipt, executable name or PID alone. Per-call workers end
before returning; any policy/config change cancels and reaps owned workers
before a new contract can be used. Persistent-worker reuse is unsupported by v1.

The L9.2 launcher consumes the private verified contract in the same trusted
host process, revalidates immediately before launch, passes no inherited extra
FDs or ambient environment, and appends only the admitted tool/helper command
and fixed arguments from trusted package/bootstrap configuration. This API is
never an arbitrary-command/credentials forwarding endpoint. Its native host
proof must exercise descendants, private sockets/files, proc/FD aliases and
permission narrowing. L10 separately authenticates package/config admission;
L11 combines these receipts with owning-session authority. Policy diagnosis
alone never implies Live readiness.

Focused offline tests use a stand-in binary and an isolated observation of host
daemon/config ancestry; they validate contracts, precedence and refusal paths.
They are not native confinement evidence. Actual pinned-host enforcement,
owned cleanup and full manager gates remain required in L9.2.

### Confined dispatch and process receipts

The optional private manifest `commands` record enables explicit dispatch.
Each key is a lowercase command ID matching `[a-z][a-z0-9-]{0,63}`; each value
is exactly `{executable,sha256,args,timeoutMs}`. Executable is a canonical,
non-writable, checksum-pinned file outside every original writable root; args
is a fixed array of strings without NUL. Timeout is an integer from 1 to
86400000 milliseconds. Package/config admission remains L10's responsibility;
an interpreter or helper's project input is untrusted even when its executable
is admitted. No command, argv, environment or credentials are taken from a
browser or generic controller RPC request.

The exact dispatch interface is:

```text
gestalt live-policy run --manifest ABS --command ID --launch-id UUID
gestalt live-policy verify-process --manifest ABS --launch-id UUID
gestalt live-policy stop --manifest ABS --launch-id UUID
```

UUID is a canonical lowercase random UUIDv4, supplied by the trusted caller and
never reused. `run` verifies the current signed policy receipt, holds one
private `dispatch.lock`, revalidates after acquiring it, and starts the pinned
native `sandbox --sandbox-state-json ... -- ADMITTED_EXECUTABLE FIXED_ARGS`.
It forwards only the caller's project stdin and the confined child's stdout/
stderr. Its controller environment is reconstructed; no ambient variables,
credential files, extra descriptors, startup hooks or alternate sandbox binary
are forwarded. Control failures use the same bounded unavailable JSON contract;
child output is project/tool output rather than diagnostic logs. `run` exits
with the native child status; cancellation and failed cleanup exit nonzero.

A separate manager supervisor owns a private lifetime pipe. Owner EOF,
SIGTERM/SIGINT, owner death, timeout, leader exit, or changed authenticated
policy inputs begin owned termination. Nonblocking pipe handles avoid a blocked
filesystem read holding supervisor teardown open. The supervisor checks
manifest, policy receipt, binary, command, config and component identities at
500 ms intervals. It sends TERM then KILL after 750 ms and waits up to another
two seconds for native close. Preparation is refused while a launch is active;
new policy preparation can happen only after the owned launch is reaped.
The owning controller must revoke dispatch and await Stop before publishing
permission changes; the monitor is a fail-closed backstop, not permission to
continue using an outdated connection. No persistent worker is reused for a
second policy or call.

The private `process-UUID.json` record is HMAC-authenticated with the same
private signing key. Its body is exactly the manager-authored launch provenance:
`schemaVersion:1`, `kind:"process-launch"`, `launchId`, `status`, `commandId`,
`contractDigest`, the complete `policy` contract, `parent`, `owner`, `process`
and `processGroup`. Parent, owner and process each bind PID, Linux birth ticks,
UID and mount/user/PID namespaces; the enclosing policy also binds the Linux
boot ID. Old receipts cannot signal a recycled PID after a host reboot. Process identifies the manager's newly
spawned native sandbox wrapper; it never adopts an external worker. The tested
native wrapper enforces the supplied state for its actual child/descendants.
A policy receipt alone still cannot establish that origin.

The temporary `authorized-launch` record is not a running-process receipt.
`verify-process` requires an authenticated `running` record, current parent
policy and matching live supervisor/native process identities. It returns
`process-verified` or a bounded refusal. `stop` validates the stored owned
identities and requests supervisor cancellation even after the manifest's policy
changes; it returns `stop-requested`, not a premature reaping claim. The caller
awaits an authenticated `exited` record. An unconfirmed close becomes
`cleanup-required` and retains the dispatch lock so new work cannot hide an
owned-process leak. Tampered records cannot target another PID.

The isolated `live-host-policy-proof.yml` workflow reuses the accepted Mobile
launcher source from reachable proof commit `2facdae7`: all three transitive
bundle inputs are byte-identical to accepted `c558dd6`, and CI checks their exact
inventory and SHA256 before use. This establishes launcher equivalence rather
than whole-package equivalence. It uses actual pinned native 0.161 binary and temporary Caddy. It runs
a mandatory exact project-input roundtrip through Node `process.stdin` (streamed
chunks, EOF and POSIX FIFO identity), plus public/private and proc/FD probes across child and grandchild, including a
detached grandchild, permission narrowing, owner death, native leader exit and
owned cleanup. The existing full manager workflow runs separately on the same
source commit. Uploaded provenance records exact tree, source hashes, component
pins, binary identity and effective policy. Offline launcher fixtures are not a
substitute for this supported-host evidence; production readiness remains false
until L10/L11 admission and Live integration gates are satisfied.

A failed launch may add a signed `failure` object to the process receipt with a
fixed phase (`native-spawn`, `native-identity`, `receipt-write`, or
`post-spawn-contract`, or `lifetime-contract`), an internal policy reason, a bounded OS error code, or
a JavaScript error class. Error messages, paths, environment and private contents
are never included in this diagnostic.

Under v1, before launch every project/ancestor `.codex` path remains unavailable. Pinned
native 0.161 creates transient empty `.codex` mount targets while enforcing
protected metadata. During an authenticated owned launch only, the manager
accepts an empty, non-symlink directory at `appRoot/.codex` owned by the current
UID. Its inode, birth/change timestamps and mode are pinned in the signed
process receipt while native PID/birth/namespaces remain valid. Files, ancestor
metadata, symlinks, replacements and stale native ownership still revoke the
launch. After owned reaping, cleanup preserves the placeholder and all project
metadata. It never removes metadata to make a later admission succeed. The v2
first-admission protocol below handles verified owning-runtime provenance.

A fixed trusted stdio bridge creates real POSIX pipes for native stdin/stdout/stderr.
Pinned native network seccomp denies `getsockname`; Node treats inherited Unix
socket stdio as an unknown handle and silently discards output or reports empty input. The bridge uses
canonical non-writable `/usr/bin/bash` and `/usr/bin/cat`, hashes both into the
receipt, and forwards only their pipe bytes to caller stdout/stderr. Bash runs
with no startup profiles and fixed argument forwarding, then execs the native
binary so its PID/birth remains the supervised launch identity. The three fixed
host consumers read only their stdin, remain in the owned process group, and
receive no extra descriptors or credentials. The input consumer forwards project bytes and EOF through its
dedicated POSIX pipe; the private lifetime pipe is never inherited by native children.

The `live-policy` Bash entry point execs its Node dispatcher. The caller's owned
CLI PID/birth therefore matches the receipt parent and the only holder of the
private lifetime writer. Killing that CLI closes the writer, revokes the native
launch and reaps its stdio consumers; an orphaned intermediate shell cannot keep
the owning Node process alive.

## Owning-runtime metadata provenance (contract version 2)

Version 1 remains available for its existing clean-first-launch contract. It
cannot admit metadata that already existed before preparation. Version 2 adds
an elected-controller protocol for the actual original owning Native runtime;
it does not adopt processes by PID or trust a directory because it is empty.
Use the same configured private controller directory and existing Mobile
election, rather than creating another controller or another election location.
The native runtime remains pinned to 0.161.0.

The host controller must obtain its real child-process launch handle, hold
ordinary/model/tool/child writers before original runtime birth, and keep that
hold until admission completes. Before launching, it supplies all v1 fields
with `schemaVersion:2` and these additional fields:

| Field | Exact v2 shape |
| --- | --- |
| `election` | `{holder:{pid,birth,uid,namespaces:{mnt,user,pid}},lockFile:ABS}`; the existing elected-controller child and `controller.lock` in the protected configured controller directory, which may be an ancestor of the generation manifest directory |
| `generation` | `{sessionId:UUID,generation:POSITIVE_SAFE_INTEGER}`; canonical owning relay session and current Live admission generation |
| `owningRuntime` | `{args:["app-server",...STRINGS]}`; exact original owning Native argv after the pinned executable |
| `runtimeProvenance` | `{captureId:UUID,effectiveConfigDigest:HEX,sessionFlagsDigest:HEX}`; current observed digests, added only after successful sealing |

UUIDs are lowercase RFC4122 version 4. No other added fields are accepted.
Manager checks the holder's actual parent, birth, UID and namespace identity,
its retained descriptor's device/inode, and the kernel FLOCK write lock on that
inode. An unlocked descriptor, busy-lock assertion, unsigned marker or holder
PID alone is insufficient. Signing/revocation commands must run as direct
children of the declared elected host controller. The controller's private
state and key must already be denied by its native effective profile. V2 uses
the exact canonical `live-policy.key` below the election directory as its
32-byte shared signing key; the directory must be declared in
`privatePaths.controller` and contain the generation manifest directory.
Per-generation keys cannot bypass an existing app admission fence. V1 retains
its original key-location requirements.

The sequence is:

```text
gestalt live-policy capture-runtime --manifest ABS --capture-id UUID
[the elected controller launches its owning runtime using the managed launch path]
[the controller resolves and validates that runtime's actual complete config sources]
gestalt live-policy seal-runtime --manifest ABS --capture-id UUID --runtime-pid PID --sources ABS
[add runtimeProvenance with captureId and current observed config/flag digests]
gestalt live-policy probe --manifest ABS
gestalt live-policy prepare --manifest ABS
gestalt live-policy verify --manifest ABS
```

These are private controller APIs, not instructions to launch a second runtime
from a model sandbox. Operator Codex invocations use `gestalt cli -- ARGS` and
Mobile invocations use `gestalt mobile -- ARGS`. No bare/resolved Codex
invocation, production configuration change, installation or restart is needed
or authorized by this protocol. The owning production adapter must supply the
original child handle and resolved policy rather than retrofit an already
running process. Required project-instruction discovery remains enabled.

Capture validates the trusted manifest and composed policy, observes absence
of `.codex` at every canonical app ancestor, and signs that observation before
birth. It pins each parent directory's device/inode/UID/mode and records Linux
USER_HZ ticks after the observation. Runtime birth must be strictly later than
that tick; equality fails closed. An atomic `runtime-admission-APP_HASH.lock` and signed
`runtime-admission-APP_HASH.json` reserve one admission in the configured election
scope, preventing competing captures/session generations even across different
manifest directories. `APP_HASH` is SHA256 of the canonical app path. Capture IDs cannot
be reused. Failed or abandoned captures require authenticated revocation.

Sealing verifies the original owner's live process identity, direct controller
parent, pinned executable bytes and exact argv. Its signed receipt binds the
actual original environment digest without returning environment values. It
also checks all captured parent identities and the current closed, empty
metadata directories using O_DIRECTORY/O_NOFOLLOW descriptors and repeated
identity/content checks. Directory sources use actual device/inode/UID/mode
and change timestamps. They never use copied directory snapshots or appear in
`configFiles`, which continues to contain regular private config files only.
Content, symlinks, replacement and unknown provenance fail closed.

`--sources` is a private, canonical mode0600 file containing exactly:

```json
{
  "schemaVersion": 2,
  "runtime": {"pid": 123, "birth": "456", "uid": 1000,
    "namespaces": {"mnt": "mnt:[...]", "user": "user:[...]", "pid": "pid:[...]"}},
  "policyDigest": "64 lowercase hex characters",
  "effectiveConfigDigest": "64 lowercase hex characters",
  "sessionFlagsDigest": "64 lowercase hex characters",
  "sources": []
}
```

The displayed empty array is a shape illustration and is rejected. The elected
controller derives this inventory from the actual owning Native config response
and original session flags, including disabled layers and referenced agent-role
files. It validates all effective launch surfaces with the Agents admission
contract before signing; no caller-provided digest, reconstructed thread-agnostic
config, filtered source list or model/project request can supply that authority.
Manager verifies the observable file/directory inventory and binds that trusted
controller observation. It cannot discover private native RPC configuration by
inspecting a process's PID or filesystem alone. L10/L11 must preserve this trust
boundary, independently validate the effective configuration, and refuse unknown
or incomplete Native source kinds.

The 1–128 source entries have one of three exact shapes:

- File: `{kind:"file",role:"user"|"system"|"packaged-defaults"|"managed"|"agent-role",path:ABS,sha256:HEX}`.
  Every original file must match `configFiles`; every original `configFiles`
  entry must be represented at sealing. Later generated MCP/bootstrap config
  files may be added to `configFiles`, which invalidates the policy receipt and
  requires fresh preparation, without pretending they were owning-runtime sources.
- Project directory: `{kind:"directory",role:"project",path:ABS,identity:{dev,ino,uid,mode,ctime,mtime,birthtime}}`.
  Only captured ancestral `.codex` paths qualify, and their identities must match
  the observed closed tree. `ctime`/`mtime`/`birthtime` are numeric `Stats` milliseconds. The native
  directory birth must be strictly attributable to the post-owner-birth interval.
  Missing filesystem birth timestamps or ambiguous pre-birth timestamps fail
  closed. Capture records the full clock-correlation uncertainty interval;
  sealing and use reject unsupported clock changes rather than tolerating races.
- Absent source: `{kind:"absent",role:"project"|"system",path:ABS}`.
  Every absent ancestral `.codex` and all three unsupported system paths
  `/etc/codex/{config,managed_config,requirements}.toml` must be represented.
  Unknown absent file sources are rejected. The prepared `runtimeHome/config.toml` must be a
  present file source. System config remains unsupported.

Paths are unique. Owner identity, effective policy digest and all required
file/directory/absence sources must match. Arbitrary directories, omitted
ancestry, duplicate sources and treating a directory as a config file are
rejected. Original effective config/session-flag digests enter the signed
source inventory; the producer must re-resolve them before every Live model
admission, populate the manifest's current observed digest fields, and revoke
if they change. Manager compares those current observations to the sealed values. Files and directory identities are also
revalidated by Manager at preparation, verification, every subprocess dispatch
and the existing running-worker monitor. The composed v2 native profile denies
all ancestral metadata paths, removes descendant grants that could override
those denies, and retains all existing private-state and `/proc` protections.
Native confinement prevents project/helper/child mutation between monitor
checks; monitoring alone is not an isolation boundary.

Signed `birth-UUID.json` and `runtime-UUID.json` receipts bind the election,
relay generation, boot, manager/binary/component identities, original policy,
parent/app identities, runtime birth/executable/argv/environment, and complete
source inventory. Replacing configuration, permissions, profile, component,
owner or election invalidates admission. Changing an owner source requires a
new owning-session resolution and admission; stale receipts cannot grant work.
An old generation cannot be relabeled to admit another session. Unsupported
recovery combinations remain unavailable rather than trusting leftover metadata.

```text
gestalt live-policy revoke-runtime --manifest ABS --capture-id UUID
```

Revocation verifies live elected controller authority and ownership of the exact
capture, writes the signed revoked fence before releasing its admission lock,
and is idempotent. It works after policy/config changes and never adopts or
signals the original runtime; its owning controller manages that child handle.
New dispatch is refused immediately; existing owned subprocesses are stopped by
the lifetime monitor before further authorized work. Shutdown and revocation
preserve all metadata and provenance artifacts. A later capture does not trust
leftover directories, even if empty.

Success from capture/seal/revoke uses `schemaVersion:2`, `available:true`,
`liveReady:false`, the capture ID, and respectively fixed reasons
`runtime-birth-captured`, `runtime-provenance-sealed`, or
`runtime-provenance-revoked`. Capture/seal also return the composed policy digest.
V2 probe/prepare/verify report `schemaVersion:2`, `contractVersion:2`, the existing
policy/config digest fields plus `sourceInventoryDigest`,
`runtimeProvenanceDigest`, and `generation`. The policy receipt and subprocess
receipts carry version 2. Failure retains the existing bounded error envelope
`schemaVersion:1,available:false,liveReady:false,reason:FIXED_STRING`, exit 1;
no path/config/environment/exception content is returned. New fixed reasons
include `missing-election`, `stale-election`, `unauthorized-controller`,
`invalid-generation`, `invalid-owning-runtime`, `invalid-capture-id`,
`reused-capture-id`, `active-runtime-admission`, `missing-runtime-provenance`,
`unverified-owning-runtime`, `stale-runtime-provenance`,
`incomplete-source-inventory`, `metadata-content-present`, `metadata-changed`,
`source-changed`, `revoked-runtime-provenance`, `unowned-runtime-admission`, and
`unsupported-host-clock`, `metadata-before-runtime-birth`, and
`metadata-provenance-unavailable`. Prior v1 reason categories remain supported.

Focused contract fixtures exercise actual flock/process identities with an
explicit copied-Node owner and simulated source response. They establish the
interface and rejection rules, not actual Native admission or confinement.
Supported-host pinned Native proof, real source discovery and the complete
Manager suite remain separate required L9.2 gates. Agents v1 manifests/receipts
and the earlier Mobile inventory are incompatible with this v2 extension and
must be corrected and reviewed before production consumption.

The supported-host proof also requires a managed original-runtime launch. The
ordinary interactive CLI runs plugin/service discovery and introduces a
supervisor, so it cannot be substituted for a verified owning-process birth.
The following private CLI forms are restricted to the real elected controller:

```text
gestalt cli -- --live-policy-bootstrap --manifest ABS
gestalt cli -- --live-policy-owning-runtime --manifest ABS --capture-id UUID
```

Bootstrap executes only the pinned Native read-only `/usr/bin/true` in the
isolated worker home to establish the reserved host daemon directory. It never
repairs an existing masked directory or starts model work. The owning form
requires the signed pre-birth capture, reserves the existing app fence, checks
absence immediately before handoff, and execs the original app-server image.
It preserves the actual owning child PID, direct controller parent and private
stdio. Optional plugin/MCP discovery cannot run before admission. All descriptors
above 2 are closed and inherited credentials are removed. Ordinary non-Live CLI
behavior remains unchanged.

`owningRuntime` additionally permits `home:ABS` and `instructionFiles:[ABS]`.
The original home must be canonical/private, covered by the declared protected
paths, and its actual config file must be in `configFiles`; it is distinct from
the sanitized worker home. The controller declares any nonstandard instruction
source filenames before birth. Capture pins existing `AGENTS.md` and
`AGENTS.override.md` files throughout app ancestry, original-home `AGENTS.md`/`AGENTS.override.md`,
and these explicitly declared files. Sealing cannot accept an instruction source
that was not pinned before original birth.

V2 inventories also distinguish `{kind:"file",role:"sandbox-worker",path,sha256}`
for the actual sanitized worker config from owning Native config layers, and
`{kind:"instruction",path,sha256,identity:{dev,ino,uid,mode,ctime,mtime}}` for
instruction files the actual Native startup reports as loaded. These are actual
regular files with descriptor/identity/content checks, not copied snapshots.
Instruction content is never returned or logged. Changes invalidate admission
and stop owned workers; project instructions remain enabled and are actually
loaded during the supported-host proof.

Every present pre-birth instruction candidate must appear in the inventory.
Candidates that Native did not load use `kind:"instruction-candidate"` with the
same path, hash and identity fields. This preserves the distinction between
loaded instructions and files shadowed by an override or outside Native's
project discovery boundary. Sealing and every later verification independently
recheck the entire signed candidate map, including absent paths. An omitted
present candidate is incomplete; any changed present file or newly appearing
file or symlink fails with `source-changed`, regardless of caller selection.

The managed handoff writes a signed `image-UUID.json` tying the original child
identity and handoff clock to its capture. Sealing preserves that image digest
and rejects stale handoffs or metadata predating it. The underlying Native
executable/version, original policy, source inventory and directory identity
must still pass every v2 check. A managed launch receipt alone never grants Live
readiness. Policy narrowing continues to revoke prior workers and refuse a stale
admission; it does not silently regrant an older broader policy.

### Native metadata mask candidate

The narrowly authorized Native patch is attributed to upstream commit
`979011409de0a60b52f179721948e65531d26144` in
`tests/fixtures/live-host-native-mask/manifest.json`. A missing protected
metadata directory retains its directory mask when an explicit deny is added.
An existing denied metadata file receives the stronger deny mask without a
second consuming file-data operation. Writable symlink checks, explicit denies,
private proc/FD controls and cleanup ownership remain in force.

The isolated GNU candidate from build run `38645681271` passed all 166 Native
unit tests and 61 integration tests; three subprocess fixtures remain explicitly
ignored outside their invoking tests. The baseline directory-mask assertion
failed before the production patch was applied. The original release lock was
prepared only by matching its 160 local workspace versions to their manifests;
external dependencies were unchanged and every Cargo command retained `--locked`.

`tests/fixtures/live-host-native-runtime.json` records the resulting source and
build trees, build inputs, archive digest and exact Native/bubblewrap/sandbox
helper hashes. It also records the unchanged official code-mode host from the
same source commit. That static x86-64 Linux musl executable runs as a separate
process; it does not link against the GNU CLI. The proof checks its archive,
binary and signature-bundle hashes and verifies the official release workflow's
Sigstore identity, GitHub issuer, certificate transparency and transparency-log
entry before use. The upstream release preserves V8's sandbox-enabled profile.
No V8 build is needed. The actual host protocol and confinement checks remain
part of the owning proof.

The isolated Native workflow caches Cargo archives and Git object databases,
compiled Cargo artifacts, and ccache objects separately. Keys bind the exact
toolchain, runner image and installed packages, target, lock, features and build
inputs. Compiled caches additionally bind the patch and build scripts. Restores
stay within that complete context and branch; cache contents never establish
runtime provenance. Restored crate archives are checked against the locked
checksums and compiled caches require a matching context marker. The workflow
records restored keys and machine-readable ccache counters. Its default smoke
compiles C++ twice and rebuilds Native's C bubblewrap package after cleaning that
package, requiring actual ccache hits in both cases. An explicit `full-build`
dispatch builds the complete candidate; `cold_build` disables cache restore and
save. The reused official code-mode host requires no C++ or V8 compilation.

The proof verifies those pins before launching the actual owning
runtime through `gestalt cli --`. The build manifest remains the original build
input; the runtime fixture records later verified artifact pins. These artifacts
and hashes are for the supported-host proof and L9 review. They do not install,
activate or release a replacement runtime, and a successful build does not
establish admission or descendant confinement.

The owning proof uses two original policies declared before runtime birth. The
restricted policy must reject local devserver binding with a permission errno.
A separate network-enabled policy must serve public app content through the
registered loopback ingress while its helper, child and detached grandchild
retain every private-path, proc, descriptor and metadata exclusion. Version 2
preserves either original Native network state under the sealed birth-policy
digest; changing that state revokes admission. Version 1 remains restricted.


Authenticated renewal of a revoked owning admission

`renew-runtime` is a private elected-controller API for Stop/Resume and recovery:

```sh
gestalt live-policy renew-runtime --manifest NEW_ABS --capture-id NEW_UUID \
  --previous-manifest OLD_ABS --previous-capture-id OLD_UUID \
  --mode survivor
# or: --mode replacement
```

The new manifest occupies a fresh private generation directory under the same
elected authority. It keeps the same canonical app/inode, relay `sessionId`,
controller birth, election holder/lock, key, Native binary, original owning home,
argv, private exclusions, components and owning config sources. Its generation
is strictly greater than both the prior sealed generation and any subsequently
revoked reservation. The exact sanitized worker config may move to the new
private worker home; original owning config bytes and paths remain unchanged.
Later generated MCP/bootstrap admission configs remain separate from the original
Native capture sources, as in the existing contract. Fresh generations may omit
old generated files from their own config list and add newly measured generation
files through normal preparation; all archived files referenced by old signed
manifests must remain intact, and L10 still validates generated admission surfaces.
Keep every previous manifest, policy, birth, image, seal and revocation receipt.
The old manifest remains immutable after its last prepared policy receipt.

Capture, seal, revoke, renewal and managed-image transitions serialize through
a separate nonblocking per-app kernel flock. It releases automatically on verifier
death; `admission-transition-busy` means retry after the competing operation.
This is separate from the captured/sealed admission lifetime lock. A delayed old
revoker cannot publish over a new generation, and failed seal publication revokes
its own reservation before releasing its lifetime lock.

First revoke the old capture through `revoke-runtime`, drain admitted calls and
wait for their actual process completion. Manager checks the old dispatch lock,
authenticated process receipts, exited status, actual process/owner births and
absence of the owned process groups. A signed exited label alone is insufficient.
The trusted controller must separately quiesce Native turns, child agents and
commands using its actual private RPC lifecycle, unsubscribe/reconfigure as
appropriate, and positively settle all work before requesting renewal. Manager
cannot infer Native internal turn quiescence from a PID. A controller readiness
callback or cached declaration cannot replace that actual lifecycle operation.

For pinned Native, `thread/unsubscribe` removes subscriptions; it does not
destroy an idle cached thread. An explicit changed permission override on an
idle unsubscribed directly owned thread causes `thread/resume` to await cold
session shutdown before rebuilding. The producer must inspect the actual
returned flags; ignored overrides do not authorize admission. The signed
code-mode host is a shared trusted process: shutting down its old remote
session need not exit that process. Actual model/tool denial under the fresh
profile distinguishes renewed confinement from a subscription acknowledgment.
Replacing the owning Native process additionally requires actual exit of its
old owned process descendants.

Initial sealing may include this optional typed `observation` alongside the
existing inventory fields; it is mandatory in both the original and fresh seal
for renewal:

```json
{
  "observation": {
    "sessionFlags": {
      "model": "actual Native model",
      "modelProvider": "actual Native provider",
      "cwd": "/canonical/app",
      "approvalPolicy": "never",
      "activePermissionProfile": {"id": "actual-profile", "extends": ":read-only"},
      "sandbox": {"type": "actual Native legacy sandbox projection"},
      "reasoningEffort": "medium",
      "runtimeWorkspaceRoots": [],
      "instructionSources": []
    },
    "accountDigest": "64 lowercase hex characters"
  }
}
```

The displayed values are illustrative, not a valid source observation. Copy
session flags from the actual owning Native start/resume/reconfigure response;
`sessionFlagsDigest` must hash their complete canonical JSON with sorted object
keys and preserved array order. `runtimeWorkspaceRoots` and `instructionSources`
are optional only when absent from that actual response. The account digest
binds canonical `{providerId,accountId}` derived from current trusted account RPC;
use a null account only for a provider whose genuine auth mode has no Native
account. Never hash/log a credential. Unknown account or observation semantics
must be refused by the producer. These are fresh controller observations, not
model-supplied values or authority inferred from an old receipt.

Fresh effective config digest, account, provider, model, thinking, approval,
workspace roots and instruction-source selection must match the original.
The active profile identifier and legacy sandbox projection may change only
with a same-or-narrower effective native state: entries keep identical paths and
ordering; accesses may only move write→read→deny, and network may only move
enabled→restricted. Other structural changes are incompatible. Named narrowing
profiles therefore must already exist in the unchanged owning config. Changing
config bytes or adding new roots does not become supported by calling renewal.
The trusted producer must validate the fresh Native effective state against the
manifest before sealing, as for initial admission.

In survivor mode the owning process must be the exact original signed
birth/executable/parent/argv/environment/cwd and remain alive. There is no new
Native launch, PID adoption or new managed image. After `renew-runtime`, obtain
fresh actual RPC config/session/account/source observations and `seal-runtime`
with the surviving PID. In replacement mode the original birth must have
actually exited. Launch exclusively through `gestalt cli --
--live-policy-owning-runtime --manifest NEW_ABS --capture-id NEW_UUID` after the
new observation interval. The managed launcher requires a new signed image,
executes the exact binary/argv/environment and restores the signed original cwd
through its fixed trusted env executable. A new PID alone cannot seal. Then
perform genuine startup/discovery and fresh RPC observation before sealing.

The new birth receipt contains immutable lineage references to the old signed
manifest/policy/birth/image/seal and revoked fences. All present and absent
metadata and instruction candidates, parent identities, config sources and
component identities are revalidated at capture, sealing and every dispatch or
monitor verification. Preserved directories are accepted only through this
verified original lineage; empty-directory trust, deletion or precreation is
never a recovery mechanism. Metadata changes, newly appearing absent candidates,
provider/account/model/thinking mismatch, broader policy, stale election/boot,
unknown drainage, reused IDs or generation rollback fail closed. The new
`runtimeProvenanceDigest` incorporates the new capture and lineage; existing
Agents schema2 generation/provenance digest binding remains applicable. The
accepted Agents consumer at `f7a142b` still validates an exact six-key inventory
and rejects the typed `observation` extension. Renewable admission therefore
requires a consumer validator update in the owning package milestone; this
Manager contract does not claim current packaged MCP compatibility. Every old
capture and tool receipt stays revoked.

A failed captured renewal can be explicitly revoked and retried against the
last sealed parent with a still newer generation. The signed reservation binds
that parent; any managed failed replacement image must have exited and all
recorded work must be drained. Replaying old revocation cannot remove a newer
reservation lock. Partial publication remains closed and cannot authorize work.
A reservation whose new fence was published can be revoked even if its birth
file was not published. Unidentifiable locks/receipts or unknown process
settlement remain unavailable; the API never resets an unknown fence.

Unsupported cases include controller/election transfer, relay/account/component
or source replacement, changed original environment/cwd, legacy captures without
an original typed observation, unsealed initial captures without a complete
parent, arbitrary Native births and chains longer than 16 renewals. No automatic
adoption, metadata deletion, token/environment forwarding, second controller or
running installation change is provided. Full pinned Native survivor,
replacement, narrowing, lifecycle and old-authority denial proof belongs to the
renewed L9.2 gate; contract fixtures alone do not establish those host results.

### Original installed bridge provenance extension

Manager schema2 optionally supports `original-installed-launch-source-v1`.
Successful `capture-runtime`, `seal-runtime`, `probe`, `prepare`, and `verify`
responses advertise it in `extensions`. Consumers must require the capability
from successful capture before original birth; older Managers reject the additional
manifest field. A successful capture observation can populate the exact inventory
entry without starting any original process.
Schema1 rejects it. Omitting the field preserves the existing schema2 contract.
An explicit empty array, additional entries, unknown fields or kinds refuse.

`manifest.originalLaunchSources` is exactly one descriptor:

```json
[{
  "kind": "context-mode-bridge-v1",
  "path": "/protected/original-home/bin/context-mode-mcp.mjs",
  "sha256": "<launcher SHA256>",
  "gestaltHome": "/protected/gestalt",
  "interpreter": {"path": "/exact/canonical/node", "sha256": "<Node SHA256>"},
  "package": {
    "root": "/protected/cache/context-mode/2.17.0",
    "version": "2.17.0",
    "treeDigest": "<complete package tree SHA256>"
  },
  "runtime": {
    "root": "/protected/gestalt/runtime/context-mode",
    "treeDigest": "<complete prepared runtime tree SHA256>"
  }
}]
```

The original home is `owningRuntime.home` (or `runtimeHome` when omitted).
The fixed launcher must be owned by the controller UID, mode0755, single-link,
canonical and outside all admitted write roots. Its original home is private;
its bin parent must be controller-owned and protected from replacement. Every
ancestor, interpreter, package/runtime file and directory must be canonical,
root/controller-owned, protected from admitted writes and non-writable by group
or other. Package/runtime directory pins additionally require the controller UID. Sticky temporary ancestry is permitted only with protected descendants.
Symlinks, special files, hardlinked files, unexpected paths and missing inputs
refuse. No original source is rewritten, chmodded, hidden or treated as TOML.

Manager independently reconstructs the exact accepted configurator's generated
MCP bridge bytes, including the fixed `GESTALT_HOME` fallback, `codex` platform,
workspace fallback and sole absolute import of `package.root/start.mjs`.
The package directory ends in `context-mode/VERSION`; its `package.json` must
name context-mode at that version. Both full trees are independently hashed
using Agents' `measureTree` encoding: sorted recursive file traversal, each row
`[relativePosixPath, mode & 0777, sha256(bytes)]`, then SHA256 of stable JSON.
The additional observation retains hashes of file/directory identities and
protected ancestry; identical-byte replacement also invalidates admission.
The bounded tree has at most20000 nodes and512MiB of file content. The prepared
runtime must have matching package identity and the exact schema2 preparation
manifest fields, current platform/architecture/Node modules ABI, and every
required runtime file with its verified manifest hash. This does not execute
package code or establish model readiness.

The interpreter must equal the canonical Node executable actually running the
Manager verifier, with a matching digest and executable mode. Consumers must
invoke Manager using that pinned interpreter. The original process environment
is observed without modification: duplicate environment keys, nonempty
`NODE_OPTIONS`/`NODE_PATH`, conflicting `GESTALT_HOME`/`CONTEXT_MODE_WORKSPACE`,
relative/empty, missing or writable PATH directories, or a first resolved `node` differing from the exact
interpreter refuse sealing. This pins the actual original bridge resolution;
it does not forward environment variables or authorize arbitrary launch commands.

Capture returns `originalLaunchSources`, an array of independently measured
`{kind:"installed-launch-source", descriptor, observation}` values, and signs
that exact observation before original process birth. The observation contains
`launcher`/`interpreter` SHA256 and identity (`dev`, `ino`, `uid`, `mode`, `size`,
`nlink`, `ctime`, `mtime`), `package`/`runtime` tree and identity digests,
`parentsDigest`, and original `home`, `bin`, `gestaltHome` directory identities
(`dev`, `ino`, `uid`, `mode`). Pass that exact object as an additional element of
`sources` in the schema2 source inventory; it has no config-file role or path
field outside its descriptor. Omission, duplication or changed observations
refuse. All previous present/absent config/instruction requirements and unknown
kind/role rejection remain in force. Consumers independently check the descriptor,
accepted package/runtime pins and exact generated source; Manager's observation
is an additional independent check, not a substitute for consumer verification.

The complete observed source enters the policy receipt and provenance basis,
birth receipt, sealed inventory, generation fence digest and revoked fence.
Manager remeasures it at capture publication, sealing, preparation,
verification, every dispatch and the existing lifetime monitor. Renewal retains
the exact original source/observation across generations, including original
home when a fresh worker home is generated. Changed sources cannot be blessed
by updating descriptor digests or relabeling an old generation. Revocation
remains available after source tampering so the controller can stop admission;
restoration requires unchanged original provenance or a fresh owning session.

New bounded failure reasons are `invalid-installed-launch-source` (unsupported
shape, path relationship or prepared descriptor),
`untrusted-installed-launch-source` (ownership/mode/write authority),
`installed-launch-source-changed` (bytes, identity or tree mismatch), and
`unverified-installed-launch-environment` (actual original resolution differs).
Existing `missing-prerequisite`, `symlink-path`, `incomplete-source-inventory`,
`stale-runtime-provenance`, `renewal-source-changed` and revocation errors still
apply. This extension admits no hooks, extra plugins, transports, URLs,
commands, environment maps or invocation authority. Original native MCP catalog,
retirement/drain, same app-server/thread preservation and actual post-seal
initialization remain separate L10/L11 checks; genuine-provider proof remains
required separately.
