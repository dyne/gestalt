# Codex Relay Design Specification

**Date:** 2026-07-14
**Status:** Approved design, awaiting written-spec review

## Purpose

Codex Relay is a private, mobile-first web application for controlling Codex CLI app-server sessions from a phone. It runs inside a directory containing project folders, starts isolated Codex app-server processes for selected folder/profile pairs, survives flaky browser connections, restores saved sessions after relay restarts, and makes each Codex thread easy to resume later from SSH.

The first release provided three primary views: Chat, Git, and Sessions. This
historical design is superseded for authentication by the current README:
Gestalt Mobile now has built-in passkey authentication, while TLS termination
remains external.

## Product Decisions

- The relay root is the process working directory unless `--cwd <path>` is supplied.
- A workspace is an immediate child directory of the relay root. The relay root itself and nested descendants cannot be selected.
- Workspace selection is based on a canonical real path. A symlink that resolves outside the root is rejected.
- Profiles are discovered through `codex-profile`; `default` represents `~/.codex`, while named profiles represent the corresponding managed Codex homes.
- A Relay Session binds one workspace, one profile, one supervised app-server process, and one Codex thread ID.
- Sessions remain alive across browser disconnections and are restored after the relay process restarts.
- Codex thread history is the durable conversation source of truth. Relay persistence supplements it with lifecycle state and a bounded live-event journal.
- V1 supports command approvals, file-change approvals, and Codex `requestUserInput` prompts.
- Git status performs a throttled background fetch. Push is explicit, never forced, and disabled when the current branch has no configured upstream.
- Passkey authentication is required; deployment still needs an external HTTPS boundary.

## Architecture

Codex Relay is a TypeScript modular monolith. Fastify serves the compiled Svelte SPA, JSON REST endpoints, and a relay-owned WebSocket from one origin and one plain-HTTP port. The production artifact runs as one Node.js process plus one supervised Codex app-server child process per active Relay Session.

The browser does not connect directly to Codex app-server. Each app-server uses JSON-RPC over stdio behind an anti-corruption adapter. The relay translates Codex protocol types and notifications into stable application concepts, adds event sequence numbers and replay, and prevents Codex protocol churn from leaking into the UI.

### Bounded contexts

1. **Session Control (core):** Creates, restores, stops, releases, and reconnects Relay Sessions; orchestrates turns, approvals, prompts, and event delivery.
2. **Workspace Catalog (supporting):** Discovers selectable child folders and Codex profiles, canonicalizes paths, and rejects selections outside the root.
3. **Git Operations (supporting):** Reads repository state and recent history, performs throttled fetches, and pushes the configured upstream.

These contexts are modules inside one deployable application, not microservices.

### Architecture rules

- Vertical Slice Architecture organizes code by use case rather than technical layer.
- Every HTTP route follows REPR: a route-local Request type/schema, Endpoint adapter, and Response type/schema.
- Each slice has one public registration entry point and owns its validation, orchestration, ports, response mapping, and primary integration test.
- Domain and use-case code do not import Fastify, Svelte, child-process, filesystem, SQLite, or Git implementation types.
- Outbound capabilities are expressed as narrow ports owned by their consuming slice or application module.
- Adapters implement Codex app-server, SQLite, filesystem, `codex-profile`, clock, process, and Git I/O.
- A single composition root creates adapters and registers slices. No service locator or mutable global container is used.
- Shared abstractions are extracted only for genuine cross-slice concepts or cross-cutting infrastructure.

### Logical flow

```text
Svelte mobile UI
    | REST commands and queries
    | resumable relay WebSocket events
Fastify inbound adapters (REPR)
    -> vertical-slice use cases
    -> domain model and invariants
    -> outbound ports
         |- Codex JSON-RPC/stdio adapter
         |- SQLite session and event adapter
         |- Git subprocess adapter
         |- workspace filesystem adapter
         `- codex-profile adapter
```

## Domain Model

### Ubiquitous language

- **Relay Root:** Directory supplied through process cwd or `--cwd`.
- **Workspace:** Selectable immediate child directory under the Relay Root.
- **Codex Profile:** `default` or a name managed by `codex-profile`.
- **Relay Session:** Durable relay-owned record binding a Workspace, Codex Profile, app-server lifecycle, and Codex Thread.
- **Codex Thread:** Codex-owned durable conversation identified by `threadId`.
- **Active Turn:** The sole in-progress user-to-Codex interaction in a Relay Session.
- **Pending Interaction:** An unresolved approval or user-input request raised by Codex.
- **Desired State:** Whether a session should be active or stopped across relay restarts.
- **Release for Terminal:** Stop relay ownership of the app-server while retaining the session and thread for SSH resume.
- **Event Cursor:** The latest relay event sequence observed by a browser connection.

### Aggregate and value objects

`RelaySession` is the aggregate root. It contains IDs and lifecycle state, not child-process handles. `WorkspacePath`, `CodexProfile`, `ThreadId`, `EventSequence`, and `IdempotencyKey` are validated value objects.

The aggregate enforces these invariants:

- A workspace canonical path is an immediate child of the canonical Relay Root.
- A named profile must be present in the current profile catalog when creating a session.
- A session binds at most one supervised app-server process.
- A session has at most one Active Turn.
- A Pending Interaction can resolve only once.
- Lifecycle transitions follow an explicit state machine and cannot skip required initialization.
- Stop changes Desired State to stopped; process loss does not.
- Release stops relay ownership without deleting the Thread ID or SSH resume metadata.

Relevant domain events include `SessionStarted`, `ThreadBound`, `TurnStarted`, `InteractionRequested`, `InteractionResolved`, `SessionDisconnected`, `SessionRecoveryStarted`, `SessionRestored`, `SessionStopped`, and `SessionReleased`. Events drive persistence and browser notifications in-process. The system is not event sourced.

## Session Lifecycle

### Discovery and bootstrap

The SPA bootstrap query returns:

- Immediate child workspaces under the Relay Root.
- Profiles returned by `codex-profile`, including `default` when `~/.codex` is available.
- Saved Relay Sessions and their current lifecycle states.
- Server capabilities and Codex protocol compatibility state.

Directory enumeration does not recurse. The server validates every submitted workspace/profile again; browser-provided paths are never trusted.

### Starting a session

1. The browser sends workspace ID, profile, selected Codex settings, and an idempotency key.
2. The endpoint validates its Request schema and invokes the start-session use case.
3. The use case revalidates the canonical workspace and profile, creates the Relay Session, and persists its desired active state.
4. The adapter launches `codex-profile cli <profile> app-server --stdio` in the selected workspace.
5. The adapter sends one `initialize` request and one `initialized` notification.
6. It calls `thread/start`, transactionally persists the returned Thread ID, and publishes `ThreadBound`.
7. The session becomes ready and can accept a turn.

The same profile command path is used for `default` to keep launch behavior uniform.

### Restoring a session

At startup, the relay loads sessions whose Desired State is active. Each is relaunched, initialized, and restored with `thread/resume`. Restoration is bounded and asynchronous so one broken profile or workspace cannot block the server or unrelated sessions.

An unexpected child exit moves a session to `recovering`. The supervisor retries with bounded exponential backoff. A successfully relaunched process resumes the saved thread. Exhausted retries move the session to `attentionRequired` without deleting metadata.

If a process or relay exits during an Active Turn, completed Codex rollout history is recovered, but continuation of the interrupted model generation is not guaranteed. After restoration the UI marks that turn as interrupted/recovery-uncertain and permits a new turn after canonical state reconciliation.

On normal relay shutdown, child processes stop while active Desired State is retained. An explicit session Stop sets Desired State to stopped before terminating the child.

### SSH handoff

The Sessions view and `gestalt-mobile sessions` expose the workspace, profile, Thread ID, state, and an exact copyable resume command:

```sh
codex-profile cli <profile> resume <thread-id> -C <workspace> --include-non-interactive
```

“Release for terminal” terminates the managed child and leaves the saved session stopped/released. This prevents simultaneous relay and terminal writers to the same thread. A released session can later be restored in the relay.

## Chat and Reliable Reconnection

REST endpoints carry reliable browser mutations: session lifecycle changes, turn start/interrupt, approval decisions, and prompt answers. Each mutation accepts an idempotency key. WebSocket is used for server-originated lifecycle, turn, item, delta, Git-refresh, and interaction events.

Each normalized event contains `sessionId`, a monotonically increasing `sequence`, event type, timestamp, and typed payload. Sequence allocation and journal insertion are transactional. A browser reconnects with its last Event Cursor:

- If retained events cover the cursor, the relay replays the gap and switches to live delivery.
- If the gap has been pruned, the relay signals `resyncRequired`; the browser requests canonical session/thread history and reconnects from the returned current sequence.

SQLite retains a bounded event journal sufficient for ordinary network outages. Completed token deltas may be compacted into the resulting item to control growth. Codex `thread/read` with `includeTurns: true` remains the authoritative source for full persisted conversation history.

The browser uses IndexedDB for the selected session, per-session Event Cursor, unsent composer draft, and a bounded rendered-message cache. IndexedDB is a performance and offline-recovery aid, never the authoritative store.

Multiple browser connections may observe one Relay Session. Mutations are serialized and idempotent. A second turn start while one is active returns a conflict. The first valid resolution of a Pending Interaction wins; later responses receive an already-resolved conflict and refresh canonical state.

## HTTP and WebSocket Surface

The exact URL layout may be refined in the implementation plan, but V1 requires REPR slices for these capabilities:

- Bootstrap workspace/profile/session catalog.
- List and inspect Relay Sessions.
- Start, stop, restore, and release a Relay Session.
- Read canonical conversation history.
- Start and interrupt a turn.
- Resolve an approval.
- Answer a Codex user-input request.
- Read Git status/history.
- Request a throttled Git refresh.
- Push the current branch to its configured upstream.
- Query health/readiness and Codex protocol compatibility.

HTTP errors use a typed problem document with a stable error code, human-readable detail, retryability, and optional field errors. The WebSocket uses the same stable error codes in lifecycle events.

## Mobile User Experience

The application defaults to the Chat view and uses three bottom navigation destinations:

1. **Chat:** Conversation, streaming activity, approvals, prompts, and composer.
2. **Git:** Active workspace repository state, history, fetch state, divergence, and push.
3. **Sessions:** Create, switch, restore, stop, release, and inspect sessions.

A compact header always identifies the active workspace, profile, and session state. Starting a session opens a native modal dialog styled as a mobile sheet. Its labeled fields select workspace, profile, model, sandbox, and approval policy; server validation remains authoritative.

### Chat view

- Render user and agent messages as the primary content.
- Render reasoning, commands, file changes, and other tool activity as separately labeled, collapsible items.
- Stream deltas without moving focus or overwhelming assistive technology.
- Render approvals and Codex questions as inline interaction cards with explicit action labels.
- Keep the composer reachable above the software keyboard and safe-area inset.
- Preserve unsent text across disconnection and reload.
- Show persistent but compact offline, reconnecting, recovering, and attention-required states.
- Provide an interrupt control only while a turn is active.

### Accessibility and responsive behavior

- Use semantic landmarks, headings, lists, forms, labels, buttons, and native dialog behavior before ARIA substitutes.
- Visible labels are required; placeholders never serve as labels.
- Validation occurs on blur/submit, clears during correction, and focuses an error summary after invalid submission.
- All controls have visible focus and meaningful accessible names.
- Touch targets are at least 44 by 44 CSS pixels, with larger sizing for coarse pointers where practical.
- Completed agent messages and meaningful connection changes use polite live announcements; streamed token deltas remain silent.
- Critical failures that block safe continuation may use an assertive alert.
- Color is never the sole carrier of state. Contrast supports normal and forced-color presentation.
- Motion respects `prefers-reduced-motion`; no essential behavior depends on animation or hover.
- Page zoom remains enabled. Code and command output scroll horizontally within their container rather than widening the viewport.
- Bottom navigation, composer, and sheets account for safe-area insets and dynamic mobile viewport changes.

## Git Operations

Git information is scoped to the active Relay Session workspace. Non-repositories receive a clear unavailable state rather than an application error.

The Git view returns cached status immediately. If the last successful fetch is older than 60 seconds, it schedules `git fetch --prune <upstream-remote>` in the background. Concurrent refresh requests for the same repository are coalesced. The previous successful data remains visible with its age while a refresh runs or fails.

The view reports:

- Current branch and configured upstream.
- Ahead and behind counts relative to the upstream remote-tracking branch.
- Dirty working-tree summary.
- Recent commit hash, subject, author, and relative/absolute time.
- Last fetch time, refresh progress, and refresh failure.

Push is enabled only when the branch has a configured upstream and is ahead. It executes a normal push to that upstream, never `--force`, `--force-with-lease`, or automatic history rewriting. Non-fast-forward and authentication failures are reported without remediation. A branch without an upstream gets a focusable disabled-state explanation and must be configured outside Codex Relay.

## Persistence and Data Ownership

SQLite stores Relay Session identity, workspace/profile binding, Thread ID, lifecycle and Desired State, protocol version metadata, idempotency results, Event Sequences, and the bounded replay journal. Schema changes use explicit forward migrations.

The following remain owned elsewhere:

- Codex owns rollout/conversation history in the selected Codex home.
- Git owns repository history and credentials.
- `codex-profile` owns profile creation, removal, authentication, and profile configuration.
- The browser owns only disposable UI cache and drafts.

Prompts, generated message bodies, approval answers, command output, environment values, and Git credentials are not written to operational logs by default.

## Codex Protocol Compatibility

The Codex app-server API is experimental. TypeScript bindings are generated with `codex app-server generate-ts` and record the generating Codex CLI version. The relay checks the installed CLI version at startup. A mismatch produces an explicit compatibility failure or degraded health state rather than silently assuming schemas are compatible.

The development workflow includes a binding regeneration/check command and contract tests for the supported protocol subset: initialization, thread start/resume/read, turn start/interrupt, streamed items, approvals, user-input requests, and completion/error notifications. Unknown notifications are logged as metadata and ignored only when safe; malformed known messages fail the affected adapter/session without crashing the process.

## Error Handling and Observability

- Domain, application, adapter, and transport errors are mapped at boundaries and retain stable machine codes.
- Session failures are isolated; one child process cannot crash or block another session.
- App-server stdout is reserved for protocol data and parsed incrementally; stderr is captured as bounded diagnostic metadata.
- Retry applies only to classified transient operations. User mutations are not blindly repeated.
- Health distinguishes HTTP readiness, database readiness, Codex compatibility, and per-session attention states.
- Structured logs contain correlation/session IDs, lifecycle transitions, durations, protocol method names, and redacted errors.
- No telemetry or external analytics is required for V1.

## Testing Strategy

1. **Domain tests:** Workspace/profile value objects, Relay Session state transitions, single-active-turn invariant, one-shot interactions, and domain events.
2. **Use-case tests:** Fake outbound ports verify orchestration, idempotency, restoration, and error mapping.
3. **Endpoint tests:** Every REPR slice is exercised through its Fastify registration entry point for success, validation, conflict, not-found, and adapter-failure behavior.
4. **Codex adapter contract tests:** A controllable fake JSON-RPC child verifies framing, request correlation, notifications, server requests, process exit, malformed input, and restart behavior. An opt-in installed-CLI suite verifies the supported real protocol.
5. **Git adapter tests:** Temporary working and bare repositories cover fetch throttling/coalescing, recent history, dirty state, ahead/behind, no upstream, successful push, and rejected push.
6. **Persistence tests:** Temporary SQLite databases cover migrations, transactional lifecycle changes, sequence allocation, journal pruning, idempotency replay, shutdown, and startup restoration.
7. **Svelte tests:** Components and stores cover streamed items, connection states, approvals, prompts, drafts, disabled Git actions, and resynchronization.
8. **Browser tests:** Playwright mobile viewports cover bootstrap, session start, chat, network interruption/reconnection, full resync, Git refresh, push, release, and restoration.
9. **Accessibility tests:** Automated checks plus keyboard/focus assertions for navigation, dialogs, forms, interaction cards, and live announcements.
10. **Static verification:** Type checking, linting, formatting, and Svelte autofixer validation run in CI.

## V1 Scope Boundaries

V1 intentionally excludes:

- TLS termination, CORS configuration, hosted accounts, roles, federation, and remote administration.
- Profile creation, deletion, login, or configuration management.
- Arbitrary filesystem browsing, nested workspace discovery, or paths outside the Relay Root.
- Force push, upstream creation, merge/rebase, commit creation, or conflict resolution.
- Multiple relay nodes, remote databases, distributed process supervision, or high availability.
- Guaranteed continuation of an in-flight model generation after app-server or relay process death.
- Direct browser access to Codex app-server WebSockets.
- Full replacement of Codex CLI/TUI configuration screens.

## Success Criteria

The design is successful when a phone on the VPN can:

1. Open one plain-HTTP URL and see immediate child workspaces, available profiles, and prior Relay Sessions.
2. Start a Codex app-server session for a selected workspace/profile and receive its durable Thread ID.
3. Chat with streamed feedback, safely answer approvals/questions, and interrupt an active turn.
4. Disconnect and reconnect without losing canonical conversation state or unsent draft text.
5. Restart the relay and restore active saved sessions to their Codex threads.
6. Copy or list an exact SSH command that resumes the same thread with the correct workspace and profile.
7. Inspect current Git history and accurate upstream divergence after a throttled fetch.
8. Push an ahead branch only to its configured upstream using a normal non-forced push.
9. Complete these flows at a mobile viewport with keyboard, touch, and screen-reader-compatible semantics.
