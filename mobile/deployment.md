# Network deployment

Gestalt Mobile includes passkey authentication but does not terminate TLS.

## Localhost

```sh
gestalt mobile \
  --cwd "$HOME/devel" \
  --public-origin http://localhost:3000
```

Plain HTTP is valid only for `localhost`. The default loopback listener is the
safest place to complete first-device enrollment.

## Trusted HTTPS boundary

For mobile or network use, place a trusted HTTPS reverse proxy or tunnel in
front of the relay:

```sh
gestalt mobile \
  --cwd "$HOME/devel" \
  --host 127.0.0.1 \
  --port 3000 \
  --public-origin https://relay.example.org
```

The proxy must preserve the external hostname and origin, cookies, and
WebSocket upgrades. Do not mount the relay under a rewritten path prefix.

::: danger Do not expose an open bootstrap
An empty authorization store is intentionally open so the first verified
passkey can become the owner. Enroll that device before exposing the endpoint.
Do not bind `0.0.0.0` until HTTPS and the exact public origin are in place.
:::

## Origin stability

The public origin must exactly match the browser's external scheme, hostname,
and port. Once credentials exist, Mobile refuses an RP-ID hostname change that
would strand them. Multiple instances on the same hostname can use different
ports and share authorization.

## Unsafe mode

`--disable-passkey-auth` gives every reachable client complete access to
workspaces, Codex sessions, and Git actions. Use it only inside an already
access-controlled local environment and never expose it directly to a shared or
public network.
