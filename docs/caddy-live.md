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
