# Launcher temporary routing

The launcher consumes version 1 of Mobile's `docs/session-scratch-contract.md`.
`GESTALT_SESSION_TMP_CONTEXT` locates the selected canonical workspace and owner;
it is validated before command dispatch creates state. A present invalid value
fails visibly. The launcher rejects links, foreign ownership, nonprivate managed
directories, malformed markers, mismatched session hashes and invalid leases.
An existing user-owned workspace `tmp` parent keeps its permissions and contents;
the private-directory requirement applies to `sessions`, owners and leases.
It derives `TMPDIR`, `TMP` and `TEMP` from the validated directory. It never
repairs a marker or falls back from invalid managed context to OS scratch.

Native Codex descendants, managed MCP transports, Serena preparation and browser
guardians inherit these four values. The selected workspace remains independent
of a nested command or MCP project cwd. CLI configuration discovery and XERJ
shared-backend readiness scrub all four owning-session values. Without context,
discovery retains operator-supplied temporary roots. Persistent tool variables,
existing environment allowlists and native permission profiles retain their
other settings; managed MCP allowlists append only the four contract variables.

## Allocation inventory

All paths below are created by `public/gestalt`. This inventory distinguishes
disposable operation scratch, persistent state and destination-local staging.

| Site | Ownership and routing | Retention |
| --- | --- | --- |
| Nested CLI/service owner publication: `.containment-lock` | Short private fence on every validated attributed ancestor, outermost first | Coordination metadata, not payload; reverse release only for the acquired identity. Busy/crashed/replaced claims stay protected |
| `test_xerj`: `gestalt-xerj-regression-*` | `mkdtemp(tmpdir())`; inherits validated managed directory | Removes only its fixture tree after owned test children stop |
| `codex_sandbox_probe`: `gestalt-sandbox-doctor-*` | `mkdtemp(tmpdir())`; managed directory when context exists | Probe-owned tree removed on completion |
| `serena_operator`: `gestalt-serena-doctor-*` | `mkdtemp(tmpdir())`; managed directory when context exists | Probe-owned tree; temporary probe HOME/config stays inside it |
| Serena workspace preparation | Managed temp variables preserved; standalone defaults remain `.gestalt/serena/tmp`, with operator overrides honored | `.gestalt/serena` indexes, caches, configuration, memories, prepared compilation inputs and language-server state remain persistent |
| Playwright CLI frontend: `gestalt-cli-playwright-*` | `mkdtemp(tmpdir())`; managed directory when context exists | Dedicated socket and frontend tree removed after backend shutdown |
| Playwright stdio backend and Chrome children | Validated context forwarded through the allowlist and guardian | Profiles, cookies, browser output, package metadata and workspace contention records stay in existing `GESTALT_HOME/playwright` state |
| XERJ MCP/search/index/autoindex | Validated context forwarded through the restricted native environment; absent context preserves operator temporary roots | `CODEX_HOME/xerj-data`, code storage, indexes, model/dependency caches and autoindex journals stay persistent |
| XERJ shared backend/readiness | Scrubs caller's context. A new backend with explicit `--cwd ROOT` receives its own workspace-service owner | Reused backends retain their original workspace/owner across callers. Without a selected root, existing tool storage remains outside managed cleanup; never infer a root from cwd |
| Chrome download: `gestalt-chrome.*` | `mktemp` under validated `TMPDIR`, or standalone operator/OS fallback | Download removed after installer completion |
| Skills archive transfer: `gestalt-skills-export.*`, `gestalt-skills-import.*` | `mktemp` under validated `TMPDIR`, or standalone fallback | Operation-owned transfer tree removed on completion |
| Manager download: `gestalt-update.*` | `mktemp` under validated `TMPDIR`, or standalone fallback | Download removed on completion |
| Mobile approval executable: `run/codex-approval.*/bin/codex` | Persistent launch support under `GESTALT_HOME/run` | Existing wrappers may remain referenced by running/reopenable Mobile; excluded from disposable session cleanup |

## Installation and atomic staging exceptions

These paths remain beside their destinations. Moving them to session scratch
would turn a same-filesystem rename into a potentially non-atomic copy/move.
Their existence does not authorize Clean TMP to delete their parent directories.

- XERJ `.install.*` release download/extraction and `.config.*` replacement
  files, under its managed installation/data roots.
- Serena `release-<version>.*`, including private installer HOME, uv tooling,
  cache and tmp. The validated release is promoted in place; installer temporary
  paths are deliberately confined to the candidate release. Interpreter/model
  installations retain their existing lifecycle.
- Playwright `release-<version>-*` package/lock/integrity staging, promoted in
  its package root; prior active releases remain installed.
- Optional skills `.gestalt-skills-stage-*` and Agents `.gestalt-setup.*`
  checkout/install candidates, promoted beside persistent managed content.
- Skill archive `.gestalt-skills.*`, profile `.gestalt-profile.*`, manager
  `.gestalt.*`, Mobile `.mobile-*`, and XERJ lifecycle/config replacements.
- Serena configuration and prepared compilation-database `NamedTemporaryFile`
  replacements, each in the directory containing the destination.
- Existing CLI browser compatibility records use a temporary file in their
  persistent record directory before atomic rename.

These exceptions are installation transactions or atomic replacement files,
not an assertion that all files whose names include `tmp` are disposable.
No staging or runtime path change overrides a filesystem's `noexec` mount.

## Standalone invocation and lease lifetime

Each standalone `gestalt cli -C ROOT ...` invocation allocates a private
`cli-<UUID>` owner before native startup. The explicit selected canonical root
stays fixed even when commands run in nested directories. Separate invocations,
including later native resumes of the same provider thread, receive separate
owners. Mobile's stable relay owner and reopen semantics are unchanged.

The native CLI receives its original selectors, profiles, images, permissions,
stdin and arguments. There is no new frontend, selector interception, restart,
input replay, runtime WebSocket dependency or persistent CLI identity registry.
Native `SessionStart.session_id` supplies the root association after startup or
resume; `SubagentStart.agent_id` supplies the child association. The latter
event's `session_id` still identifies the root. Transcript filenames and argv
selectors are never interpreted as provider IDs. Children inherit all four
context values. Marker updates use same-directory atomic replacement and a
bounded association lock. If native hook discovery/trust is unavailable, the
CLI keeps working and reports the limitation; an indeterminate borrower lease
keeps its owner protected instead of inventing an association.

Without an explicit selected workspace there is no managed CLI allocation. The
launcher reports that limitation and preserves ordinary native behavior and
operator temporary settings. An inherited validated Mobile/CLI context remains
the owning context for a nested launch.

CLI and workspace-service leases follow the v1 marker/lease extension. The
guardian uses a separate lifetime pipe, preserves the real native stdin, and
tracks the actual child it spawned. Controller interruption or death closes
that pipe. The guardian sends TERM, permits bounded grace, and sends KILL only
while the original group leader is still observed alive. Once the leader exits,
only read-only group-absence checks are permitted. Successful signal delivery,
PID absence or age never releases a lease. Surviving descendants, zombies,
permission failures, malformed records and unconfirmed shutdown retain active
protected metadata. A released lease has no consumers.

Managed Serena/browser guardians register borrower leases before detached
native work starts; the invocation's primary lease cannot release until those
borrowers release. XERJ's shared backend has an independent `service-<UUID>`
owner and service consumer, so caller exit does not release or relocate it.
Its stop acknowledgement waits for confirmed native-group absence within the
existing control deadline. Persistent indexes, profiles and caches survive.

L4 must treat `tmp/sessions/<owner>/.controller-lock` as the owner's lifetime
fence and `.association-lock` as an in-progress marker update. Cleanup may claim
the same controller-lock path only after it is absent, then must recheck the
marker, every lease and durable Mobile protection under that claim. Allocation
publishes active metadata while holding the fence; release publishes only
after confirmed shutdown and removes the fence last. Unknown/partial locks or
leases are preserved. These files grant no authority to delete arbitrary paths.
Stopping the CLI retains its directory; deletion belongs to later cleanup.
## Nested owner publication and containment

A selected workspace may be inside another managed owner's payload. Before
creating a CLI or workspace-service owner there, the launcher validates all
attributed `tmp/sessions/<owner>` ancestors and claims their private
`.containment-lock` directories, outermost first. These short publication
fences are separate from lifetime `.controller-lock` and metadata
`.association-lock` claims. Ordinary workspaces require no ancestor fence.

Under those claims the launcher revalidates the canonical selected workspace,
directory identities and immutable ancestor marker provenance. A retired,
replaced, aliased or indeterminate ancestor fails before allocation; it never
recursively recreates a missing workspace. Valid released CLI/service ancestors
still supply attribution, without supplying deletion authority. Unknown markers,
partial leases and existing containment fences are not repaired, adopted or
expired by timestamp/PID.

Full owner and initial lease metadata are published synchronously before reverse
release. An open directory handle and filesystem identity distinguish each
acquired fence from a replacement; a mismatched claim is retained. Failures
release only still-matching owned instances. No containment claim spans the
native process lifetime, changes the v1 envelope or changes native arguments,
stdin, permissions, persistent roots or per-invocation CLI UUIDs.

Mobile cleanup/publication uses the same containment namespace and lock order.
Cleanup must hold the corresponding claims through its final authority checks
and retirement/deletion, and conservatively protect existing nested owner or
indeterminate namespaces. Launcher publication alone never grants cleanup
authority over the outer or inner payload.

### Recovering an abandoned Mobile containment fence

A publisher killed during metadata publication cannot run its `finally` block.
An empty `.containment-lock` may then block every later context-mode execution
with `SESSION_SCRATCH_BORROW_FAILED`. Its age, emptiness and a missing PID do not
prove that it is abandoned. Normal publication continues to fail closed.

For **offline operator recovery**, first stop Mobile and **all** runtimes,
tools and other publishers using the affected owner, including detached tools.
Pausing Autopilot alone is insufficient. Preserve a redacted incident capture
before stopping services. Do not run recovery against an active session.

```sh
gestalt scratch inspect --cwd /absolute/workspace --owner mobile-OWNER_HASH
gestalt scratch recover --cwd /absolute/workspace --owner mobile-OWNER_HASH \
  --expect TOKEN_FROM_INSPECT --confirm-quiescent
```

`inspect` is read-only and emits a token bound to the validated Mobile owner
and exact empty fence identity. `recover` revalidates both, requires the
operator's explicit quiescence assertion, and removes only that empty directory.
It rejects changed identities, aliases, non-private or nonempty fences, unknown
owner/lease metadata and existing controller or association claims. It never
expires leases, deletes payload, changes a session database or resumes a session.
Active lease records remain protective after recovery. CLI/service lifetime
claims and other unknown locks are outside this recovery command's scope.

The token prevents accidental recovery of a replacement fence; it is not a
process-liveness test or synchronization with a live writer. After recovery,
restart the owner normally and resolve any pending attention request with
Resume, optionally providing guidance. This source change is not deployed until
the installed manager has been updated.

## Mobile detached tool borrowers and long Unix socket paths

Validated Mobile owners use the same v1 borrower records as CLI and service
owners. A Mobile marker retains its `mobile-roster` authority; a tool never
invents a primary CLI lease or changes the marker. Before launching a detached
Serena, Playwright or existing browser-broker backend, the launcher publishes
its own private active lease. It creates a missing private `leases` directory
under the owner's controller fence and validates an existing directory. A
Mobile resource controller may release its own lease while a detached tool
borrower remains active; cleanup must inspect every lease.

Mobile borrower publication and release briefly claim `.controller-lock`, the
same fence used by Mobile cleanup and association. Existing or crashed fences
are never stolen. CLI/service controllers retain their lifetime controller
fence; their borrower publication and primary release serialize through
`.association-lock`. Unknown, malformed and partially published metadata stay
protected. A borrower releases only its exact claim after the guardian confirms
the complete owned process group absent. EOF and TERM initiate owned shutdown;
an unconfirmed or hard-killed guardian leaves its active lease intact.

The optional browser broker keeps its private Unix socket inside its managed
temporary directory. A confirmed long-owner-path failure exceeded the Unix
socket address limit. On Linux the broker holds an open directory descriptor
and binds/connects through `/proc/<broker-pid>/fd/<descriptor>/frontend.sock`.
This addresses the same socket file under the owner, with mode 0600; it neither
relocates scratch nor introduces a network listener. The descriptor stays open
through frontend/backend shutdown. Short socket paths retain their existing
address. Other platforms preserve their existing socket address and surface
unsupported-path errors; there is no outside-owner fallback. Native arguments, stdin and
the existing Unix transport/control protocol remain unchanged.
