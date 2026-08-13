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

<MobileScreenshot
  src="01-first-device-enrollment.png"
  alt="First-device enrollment with nickname, authorization button, QR code, and setup link."
  caption="The visibly open first-run state must be claimed locally before the relay is exposed."
  :width="375"
  :height="1077"
  eager
/>

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

Once the first passkey exists, an authorized device can create a short-lived
handoff link for another browser. The enrollment secret remains in the URL
fragment, where it is not sent in the HTTP request.

<MobileScreenshot
  src="18-add-device.png"
  alt="Authorized Devices screen with an active enrollment QR code and link controls."
  caption="Create a time-limited enrollment handoff from an already authorized device."
  :width="375"
  :height="1585"
/>

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
