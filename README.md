<h1 align="center">
  Gestalt<br/><br/>
  <sub>We invite you to stop assembling the pieces and start perceiving the whole.</sub>
</h1>

<p align="center">
  <a href="https://dyne.org">
    <img src="https://img.shields.io/badge/%3C%2F%3E%20with%20%E2%9D%A4%20by-Dyne.org-blue.svg" alt="Dyne.org">
  </a>
</p>

<br><br>

### 📖 More info on [dyne.org/gestalt](https://dyne.org/gestalt) <!-- omit in toc -->

***

<div id="toc">

### 🚩 Table of Contents  <!-- omit in toc -->
- [🎮 Quick setup](#-quick-setup)
- [💾 Build](#-build)
- [🧪 Testing](#-testing)
- [💼 License](#-license)

</div>

## 🎮 Quick setup

Download a release tarball from [GitHub Releases](https://github.com/dyne/gestalt/releases/latest). Each archive contains multiple binaries (`gestalt`, `gestalt-agent`, `gestalt-send`, `gestalt-notify`, `gestalt-otel`).

Install all binaries into your `PATH`.

Linux/macOS example:

```sh
# choose your platform archive (example: linux amd64)
curl -L -o gestalt.tar.gz https://github.com/dyne/gestalt/releases/latest/download/gestalt-linux-amd64.tar.gz
sudo tar -xzf gestalt.tar.gz -C /usr/local/bin
```

Windows PowerShell example:

```powershell
# choose your platform archive (example: windows amd64)
Invoke-WebRequest -Uri https://github.com/dyne/gestalt/releases/latest/download/gestalt-windows-amd64.tar.gz -OutFile gestalt.tar.gz
tar -xzf .\gestalt.tar.gz
Move-Item .\gestalt.exe "$env:USERPROFILE\bin\gestalt.exe"
Move-Item .\gestalt-agent.exe "$env:USERPROFILE\bin\gestalt-agent.exe"
Move-Item .\gestalt-send.exe "$env:USERPROFILE\bin\gestalt-send.exe"
Move-Item .\gestalt-notify.exe "$env:USERPROFILE\bin\gestalt-notify.exe"
Move-Item .\gestalt-otel.exe "$env:USERPROFILE\bin\gestalt-otel.exe"
```

Run:

```sh
# starts dashboard + API server
# dashboard URL: http://localhost:57417
gestalt

# runs an agent profile in standalone mode (requires Codex CLI installed)
gestalt-agent <agent-name>
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

Copyright (C) 2025-2026 Dyne.org foundation

Designed and written by Denis "[Jaromil](https://jaromil.dyne.org/)" Roio.

This program is free software: you can redistribute it and/or modify it under the terms of the GNU Affero General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License along with this program. If not, see https://www.gnu.org/licenses/.


