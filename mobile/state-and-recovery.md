# State and recovery

Gestalt Mobile separates workspace relay state from shared authorization.

## Relay databases

With `--data-dir <path>`, relay state is stored in `<path>/relay.sqlite`.
Without it, Mobile uses:

```text
${XDG_STATE_HOME:-~/.local/state}/gestalt-mobile/<workspace-hash>/relay.sqlite
```

Each workspace root therefore receives independent session state. A matching
legacy `codex-relay` database is reused when present.

## Authorization database

Passkeys and authorization sessions are shared at:

```text
~/.codex-gestalt/gestalt-mobile/auth.sqlite
```

The containing directory is created owner-only. Treat the database as private.
For a consistent backup, stop every Mobile instance and copy the database with
its `-wal` and `-shm` sidecars, or use SQLite backup tooling.

<MobileScreenshot
  src="17-authorized-devices.png"
  alt="Authorized Devices screen listing the current phone and a laptop with rename and revoke controls."
  caption="Device management exposes names and recent use without exporting passkey credentials."
  :width="375"
  :height="1076"
  eager
/>

## Lost every passkey

Recovery is deliberately local and manual:

1. Stop every Gestalt Mobile instance.
2. Back up `auth.sqlite` and its sidecars.
3. Remove only `auth.sqlite`, `auth.sqlite-wal`, and `auth.sqlite-shm`.
4. Restart locally into the visibly open bootstrap state.
5. Immediately enroll a new passkey before exposing the relay.

This discards shared authorization, device, and authorization-session state.
It does not remove workspace relay databases or Codex history.

::: danger Exact target only
Never perform this recovery while any relay instance is running or publicly
reachable. There is no remote administrator, recovery code, credential export,
or automatic reset.
:::

After a local reset or an explicit lock, the relay returns to its passkey gate.

<MobileScreenshot
  src="02-passkey-login.png"
  alt="Gestalt Mobile passkey login screen."
  caption="A discoverable passkey unlocks the relay without a username or recovery-code flow."
  :width="375"
  :height="812"
/>

## Browser recovery

The browser stores the selected session, replay cursor, and per-session
composer drafts. After a dropped connection it replays retained events; if the
server has pruned that gap, it reloads canonical Codex thread history.
