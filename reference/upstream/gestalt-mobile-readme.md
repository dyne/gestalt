# Gestalt Mobile

Mobile-first web relay for [Gestalt](https://dyne.org/gestalt) orchestrated
development with durable Codex sessions.

## Prerequisites

- Node.js 24 or newer.
- The `codex` CLI installed, available on `PATH`, and authenticated.
- Optionally, `codex-profile` and a `~/.codex-gestalt` home. When both exist,
  sessions use `codex-profile cli gestalt app-server --stdio`; otherwise they
  fall back automatically to `codex app-server --stdio`.

## Install and run

Run the latest release without a permanent installation:

```sh
npx gestalt-mobile --cwd .
npx --yes gestalt-mobile@latest --cwd ~/devel --port 3000
```

For frequent use, install the executable globally:

```sh
npm install --global gestalt-mobile
gestalt-mobile --cwd .
```

The command prints the loopback URL when it is ready. Open that URL in a
browser. Press Ctrl-C, or send SIGINT or SIGTERM, to stop the HTTP server,
active Codex subprocesses, and database cleanly.

## Skill profiles

Global profiles live in `~/.gestalt/skill-profiles/<name>.yml` and use version 1 YAML:

```yaml
version: 1
name: focused
skills:
  - name: typescript-advanced-types
    path: /absolute/path/to/SKILL.md
    enabled: true
```

Use `gestalt-mobile --skills focused` to apply that profile to every Codex child,
or `gestalt-mobile --skills list` to inspect saved profiles and their enabled
skill paths without starting the server. Without `--skills`, a workspace
`gestalt-skills.yml` is used when present; otherwise Codex-native selection is
preserved. Explicit profiles take precedence over project defaults, which take
precedence over native configuration. Gestalt Mobile never rewrites Codex
configuration or skill files.

## Themes

Themes are registered once in `src/client/features/theme/theme-registry.ts`. Add a
stable ID, user-facing label, `colorScheme`, and `logoTone` there; the Appearance
control is registry-driven. The bootstrap boundary resolves storage before Svelte
mounts: `light` and `dark` migrate to the Minimal themes, while missing, malformed,
unknown, retired, and `system` values use the `dyne-org` default. Keep the
`gestalt-mobile.theme` storage key compatible and never persist a migration until a
user makes an explicit choice.

Each registry entry needs a complete semantic-token values file under
`src/client/features/theme/styles/`, including surfaces, text, borders, focus,
controls, status states, code, typography, motion, and the declared color scheme.
Components consume those shared tokens and must not branch on theme IDs. Fonts and
branding assets are bundled locally (no runtime CDN); preserve their licenses.
Keep normal text at 4.5:1 contrast and large text, icons, and control boundaries at
3:1 or better. Gestalt branding remains unchanged. Dyne.org is intentionally
light-only today; a separate light/dark mode axis requires a future product decision.
See the authoritative [Dyne.org branding](https://dyne.org/branding/) and
[Delta style reference](https://delta.dyne.org/899/).

Before adding a fourth theme, update registry/token unit tests, selector/component
coverage, and the Playwright evidence matrix in `test/e2e/theme-evidence.ts`: all
themes at 390×844/100% for each representative state, plus the named dense states at
320×568 and 768×1024 with 200% text. Verify pre-navigation storage, 44px controls,
focus/non-color state cues, no visible horizontal overflow, zero unexpected console
or request errors, and no failed local font/branding request. Dry-run the release
checks with `npm run format:check`, `npm run license:check`, `npm run check`,
`npm test`, `npm run lint`, `npm run build`, `npm run test:e2e`, and
`npm run test:package`.

## Command-line options

| Option                     | Default              | Purpose                                                            |
| -------------------------- | -------------------- | ------------------------------------------------------------------ |
| `--cwd <path>`             | Current directory    | Root containing selectable workspaces                              |
| `--host <address>`         | `127.0.0.1`          | HTTP listen address                                                |
| `--port <number>`          | `3000`               | HTTP listen port, from 1 through 65535                             |
| `--public-origin <origin>` | Loopback Vite origin | Exact browser origin for passkeys; required for non-loopback hosts |
| `--disable-passkey-auth`   | Off                  | Disable passkey access control and serve every client directly     |
| `--data-dir <path>`        | XDG state directory  | Directory containing `relay.sqlite`                                |
| `--skills <profile>`       |                      | Apply a saved global profile to every session                      |
| `--skills list`            |                      | List global profiles without starting the server                   |
| `--help`                   |                      | Print usage without starting the application                       |
| `--version`                |                      | Print the installed package version                                |

`--cwd` may be relative to the directory where the command is invoked. The
resolved directory is the selectable root of a recursive workspace tree. Dot
directories are omitted, and traversal stops at each directory containing a
`.git` marker, so repositories are terminal nodes. Directory symlinks are
included only when their real target stays under the resolved root; repeated
real targets and cycles are omitted. Browser requests select nodes by opaque ID,
while real filesystem paths remain server-side.

## Passkey deployment and operating limits

Gestalt Mobile has built-in passkey authentication, but does **not** terminate
TLS. `--public-origin` must be the exact origin the browser uses, including its
external HTTPS scheme and port. `http://localhost` is the development-only
exception; every other browser origin must be HTTPS. For example, a local
developer may use `--public-origin http://localhost:3000`, while a deployed
relay may listen locally on `127.0.0.1:3000` behind
`https://relay.example.org` with `--public-origin https://relay.example.org`.

For mobile or network use, put a trusted HTTPS reverse proxy or tunnel in front
of the relay. It must preserve the external host and origin, cookies, and
WebSocket upgrade; do not mount the relay below a rewritten path prefix. Same
hostname processes may use different ports and share authorization (for example,
two local relay processes behind `https://relay.example.org`), but an RP-ID/
hostname change is refused once credentials exist because it would strand those
credentials. Do not expose `--host 0.0.0.0` before that boundary is in place.

An empty authorization store is deliberately bootstrap-open: the first verified passkey becomes the owner. Register a device before exposing the service. The
final authorized device cannot be deleted; synced passkeys can represent more
than one physical device, and an enrollment QR/link is neither recovery nor
proof of physical proximity.

`--disable-passkey-auth` restores the unprotected serving behavior from before
passkey authentication was added. It does not create or consult passkey state,
and `--public-origin` is not required even for a non-loopback listener. This is
an explicit unsafe mode: anyone who can reach the HTTP or WebSocket listener has
full access to workspaces, Codex sessions, and Git operations. Use it only on a
trusted, access-controlled local environment; never expose that mode directly
to a shared or public network. Startup prints a warning whenever it is active.

## Persistent state

With `--data-dir <path>`, state is stored in `<path>/relay.sqlite`; a relative
path is resolved from the command's working directory. Without it, state is
stored below `$XDG_STATE_HOME/gestalt-mobile/<workspace-hash>/relay.sqlite`, or
`~/.local/state/gestalt-mobile/<workspace-hash>/relay.sqlite` when
`XDG_STATE_HOME` is unset. A matching legacy `codex-relay` database is reused
when present.

Authorization is separate shared state at
`~/.codex-gestalt/gestalt-mobile/auth.sqlite` (under the selected home). Its
directory is created owner-only (`0700`); treat the database as sensitive and
keep it accessible only to the local relay user. Back up `auth.sqlite`
with `auth.sqlite-wal` and `auth.sqlite-shm` only after **all** instances are
stopped, or use SQLite backup tooling.

Recovery after losing every passkey is deliberately local and manual: stop every
instance, make a backup, remove only `auth.sqlite` and its `-wal`/`-shm`
sidecars, restart into visibly open bootstrap mode, and immediately enroll a new
device before exposing the service. This discards authorization, device, and
session state only—not workspace relay databases or history—and is dangerous
while the service is exposed. There is intentionally no hosted service,
federation, remote administrator, recovery code, credential export,
authentication audit analytics, attestation trust policy, or automatic reset.

For the protocol requirements behind these limits, see the official
[SimpleWebAuthn server guide](https://simplewebauthn.dev/docs/packages/server),
[SimpleWebAuthn passkey guidance](https://simplewebauthn.dev/docs/advanced/passkeys/),
[MDN passkey guide](https://developer.mozilla.org/en-US/docs/Web/Security/Authentication/Passkeys),
and [W3C WebAuthn RP ID, origin, and challenge requirements](https://www.w3.org/TR/webauthn-3/).

## Sessions

The Sessions tab uses the recursive filesystem tree to choose the base directory
for a new session. Expand or collapse folders with the disclosure buttons, or
use Left/Right while a tree item is focused. Up/Down, Home, and End move through
visible items; Enter or Space selects the focused directory. The selected path
remains highlighted when branches are folded or the catalog is refreshed.

At startup, Gestalt Mobile asks the installed Codex app-server for its available
models. New sessions use `gpt-5.6-terra` by default; choose another discovered
model from the Session tab before creating the session. The default is defined
centrally as `DEFAULT_SESSION_MODEL` in `src/server/features/sessions/application/start-settings.ts`
so it can be changed in a future configuration surface. The selected model is
stored with the relay session and shown in managed session entries.

Use **Open** to relaunch a released, stopped, or attention-required
relay session from the browser.

If an upgrade has removed the Codex rollout for a saved session, **Open** keeps
the relay session and its settings but creates and binds a replacement Codex
thread. The client shows that the earlier Codex history is unavailable; other
restore failures leave the saved thread unchanged so Open can be retried.

The relay keeps SQLite state under the supplied data directory, or
under the root-hashed XDG state directory when `--data-dir` is
omitted. Active durable threads are resumed after a relay restart. A
failed child process is retried with bounded backoff before the
session is marked as requiring attention.

## Mobile recovery

The browser stores the selected session, its replay cursor, and per-session
composer drafts. On a dropped connection it replays retained events; if the
server has pruned the gap, it reloads canonical Codex thread history.

## Git

The Git tab has its own filesystem-tree selection, independent from the base
directory selected in Sessions. Selecting a repository shows its branch
divergence, dirty counts, recent commits, and repository actions even when no
relay session exists. Selecting an ordinary directory makes it the destination
for Clone; repositories cannot be clone destinations. After a successful clone,
the catalog refreshes and selects the new repository without changing the
Sessions selection.

Pull uses `git pull --rebase`. Push is available only for a branch that has an
upstream, is ahead, and is not behind; it never creates an upstream or
force-pushes.

Operation results appear as non-modal notifications. Errors are announced
assertively, successful operations politely, and each notification can be
dismissed with its keyboard-accessible close button.

## Versions and upgrades

Inspect the executable and registry versions with:

```sh
gestalt-mobile --version
npm view gestalt-mobile version
```

`npx --yes gestalt-mobile@latest` fetches the current release according to npm's
cache rules. Upgrade a global installation with:

```sh
npm install --global gestalt-mobile@latest
```

If startup reports an incompatible Codex protocol, upgrade Gestalt Mobile or
install the Codex CLI version supported by that release. If session startup
fails, first confirm `codex --version` works in the same shell and that Codex is
authenticated. Use `gestalt-mobile --help` to diagnose rejected options without
starting the server.

From a source checkout, maintainers can exercise the installed isolated
`gestalt` profile without touching their normal relay data:

```sh
npm run test:open-profile-smoke
```

The opt-in smoke derives the selected profile's CLI version, creates a
temporary workspace and relay database, then checks normal Open and
missing-rollout replacement. It creates one bounded harmless turn to establish
durable history, deletes only the exact smoke-created Codex threads through the
app-server afterwards, and always removes its temporary state. It reports
`SKIP` when the isolated profile is unavailable.

## Run from source

```sh
npm ci
npm run build
npm start -- --cwd <relay-root>
```

## Development

Run `npm run check`, `npm test`, `npm run lint`, and `npm run build`.

Maintainers should follow the [npm release operations guide](./gestalt-mobile-releasing.md)
when configuring GitHub, rotating credentials, or recovering a partial release.

## Copyright and license

Copyright (C) 2026 Dyne.org foundation
Designed by Denis Roio <jaromil@dyne.org>

SPDX-License-Identifier: AGPL-3.0-or-later

Gestalt Mobile is distributed under the GNU Affero General Public License
version 3 or, at your option, any later version. See the
[GNU AGPL version 3](https://www.gnu.org/licenses/agpl-3.0.html).
