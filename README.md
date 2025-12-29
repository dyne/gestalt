# Gestalt

We invite you to stop assembling the pieces and start perceiving the whole.

Welcome to Gestalt.

### More info on [dyne.org/gestalt](https://dyne.org/gestalt)

## Quick Start

Build (needs nodejs and npm):
```
make
```

Launch (needs golang)
```
go run cmd/gestalt/main.go
```

Default listens to 0.0.0.0 port 8080

When running local open browser at http://localhost:8080

## Testing

Backend:
```
go test ./...
```

Frontend:
```
cd frontend
npm run build
```

### Token authentication

If you don’t set GESTALT_TOKEN, auth is disabled.

- REST auth is Authorization: Bearer <token> when `GESTALT_TOKEN` is set (handled in frontend/src/lib/api.js).
- WS auth uses ?token=<token> in the URL (also handled in frontend/src/lib/api.js).
- Default port is 8080; override with `GESTALT_PORT`.

`GESTALT_TOKEN` is just an arbitrary shared secret you choose. The
server only checks that incoming REST/WS requests present the same
token. To generate a random token:

- macOS/Linux: `export GESTALT_TOKEN=$(openssl rand -hex 16)`
- Windows PowerShell `$env:GESTALT_TOKEN = -join ((48..57)+(97..102) | Get-Random -Count 32 | % {[char]$_})`

## API endpoints

API (development snapshot)
- GET /api/status - system status (terminal count, server time)
- GET /api/terminals - list active terminals
- POST /api/terminals - create a new terminal
- DELETE /api/terminals/:id - terminate a terminal
- GET /api/terminals/:id/output - recent output lines (buffered)

Auth
- REST endpoints expect `Authorization: Bearer <token>` when `GESTALT_TOKEN` is set.
- WebSocket connections accept `?token=<token>` for browser compatibility.

## License

Copyright (C) 2025-2026 Dyne.org foundation

Designed and written by Denis "[Jaromil](https://jaromil.dyne.org)"
Roio.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful, but
WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public
License along with this program.  If not, see
<https://www.gnu.org/licenses/>.

<p align="center">
  <a href="https://dyne.org">
    <img src="https://files.dyne.org/software_by_dyne.png" width="170">
  </a>
</p>
