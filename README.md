<h1 align="center">
  Gestalt<br/><br/>
  <sub>We invite you to stop assembling the pieces and start perceiving the whole.</sub>
</h1>

<p align="center">
  <a href="https://dyne.org">
    <img src="https://img.shields.io/badge/%3C%2F%3E%20with%20%E2%9D%A4%20by-Dyne.org-blue.svg" alt="Dyne.org">
  </a>
</p>

Gestalt is a local dashboard and API server for multi-session agent workflows.

Docs:
- Local docs site: `npm run docs` then open `http://localhost:5173`
- Docs source: [`docs/`](docs/)
- Project page: [dyne.org/gestalt](https://dyne.org/gestalt)

## 🎮 Quick setup

Download a release tarball from [GitHub Releases](https://github.com/dyne/gestalt/releases/latest). Each archive contains multiple binaries (`gestalt`, `gestalt-agent`, `gestalt-send`, `gestalt-notify`, `gestalt-otel`).

Install only `gestalt` and `gestalt-agent` into your `PATH`.

Linux/macOS example:

```sh
# choose your platform archive (example: linux amd64)
curl -L -o gestalt.tar.gz https://github.com/dyne/gestalt/releases/latest/download/gestalt-linux-amd64.tar.gz
tar -xzf gestalt.tar.gz
install -m 0755 gestalt /usr/local/bin/gestalt
install -m 0755 gestalt-agent /usr/local/bin/gestalt-agent
```

Windows PowerShell example:

```powershell
# choose your platform archive (example: windows amd64)
Invoke-WebRequest -Uri https://github.com/dyne/gestalt/releases/latest/download/gestalt-windows-amd64.tar.gz -OutFile gestalt.tar.gz
tar -xzf .\gestalt.tar.gz
Move-Item .\gestalt.exe "$env:USERPROFILE\bin\gestalt.exe"
Move-Item .\gestalt-agent.exe "$env:USERPROFILE\bin\gestalt-agent.exe"
```

Run:

```sh
# starts dashboard + API server
# dashboard URL: http://localhost:57417
gestalt

# runs an agent profile in standalone mode (requires Codex CLI installed)
gestalt-agent <agent-id>
```

Temporal workflow dev server auto-start requires the `temporal` CLI.
If `temporal` is not installed, disable auto-start:

```sh
gestalt --temporal-dev-server=false
```

## 💾 Build

Prerequisites:
- Go and Node.js (see pinned versions in [`mise.toml`](mise.toml))

Build and install:

```sh
npm i
make
make install
```

## 🧪 Testing

Main test command:

```sh
make test
```

Optional direct commands:

```sh
go test ./...
cd frontend && npm test -- --run
```

More details: [`TESTING.md`](TESTING.md)

## 💼 License

Gestalt is licensed under the GNU Affero General Public License v3.0 or later (AGPL-3.0-or-later). See [`LICENSE`](LICENSE) for the full license text and conditions.
