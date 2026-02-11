# Quick setup

Use the latest release tarball from GitHub Releases and install the binaries you need.

## Install

```sh
# Example: install only gestalt and gestalt-agent from an extracted release directory
install -m 0755 gestalt /usr/local/bin/gestalt
install -m 0755 gestalt-agent /usr/local/bin/gestalt-agent
```

## Run

```sh
gestalt
# dashboard: http://localhost:57417

gestalt-agent <agent-id>
# standalone agent runner (requires Codex CLI)
```

If you do not have Temporal CLI installed, start Gestalt with:

```sh
gestalt --temporal-dev-server=false
```
