# Quick setup

Use a release tarball from [GitHub Releases](https://github.com/dyne/gestalt/releases/latest). Each archive contains multiple binaries, including `gestalt` and `gestalt-agent`.

## 1) Install binaries

Linux and macOS:

```sh
# Example: linux amd64 archive
curl -L -o gestalt.tar.gz https://github.com/dyne/gestalt/releases/latest/download/gestalt-linux-amd64.tar.gz
tar -xzf gestalt.tar.gz

# Install only the binaries you want in PATH
install -m 0755 gestalt /usr/local/bin/gestalt
install -m 0755 gestalt-agent /usr/local/bin/gestalt-agent
```

Windows (PowerShell):

```powershell
# Example: windows amd64 archive
Invoke-WebRequest -Uri https://github.com/dyne/gestalt/releases/latest/download/gestalt-windows-amd64.tar.gz -OutFile gestalt.tar.gz
tar -xzf .\gestalt.tar.gz
Move-Item .\gestalt.exe "$env:USERPROFILE\bin\gestalt.exe"
Move-Item .\gestalt-agent.exe "$env:USERPROFILE\bin\gestalt-agent.exe"
```

## 2) Start Gestalt

Run `gestalt` from your project root:

```sh
gestalt
```

By default, the dashboard is available at `http://localhost:57417`.

## 3) Run a standalone agent (optional)

```sh
gestalt-agent <agent-id>
```

`gestalt-agent` creates an external session from the selected profile, then attaches tmux (`attach` or `switch-client`), so Codex CLI and tmux must be installed and available in `PATH`.

## Temporal dev server requirement

By default, Gestalt auto-starts a Temporal dev server. This path requires the `temporal` CLI.

- Install Temporal CLI: https://temporal.io/setup/install-temporal-cli
- Or disable auto-start:

```sh
gestalt --temporal-dev-server=false
```
