# Testing

## Quick commands

Backend (Go):
```
go test ./...
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

Frontend (Vite + Svelte):
```
cd frontend
npm test
npm run test:coverage
```

## Coverage audit (baseline)

Coverage captured with `go test -cover ./...`.

Package | Coverage | Notes
--- | --- | ---
cmd/gestalt | 43.3% | Config + static dir coverage.
internal/agent | 92.5% | Unit tests cover prompt parsing + loader behavior.
internal/api | 44.1% | REST + websocket integration tests present.
internal/logging | 73.0% | Buffer + hub + logger tests present.
internal/orchestrator | [no statements] | Stub package.
internal/terminal | 80.2% | Manager + session + buffer + broadcaster coverage.

Coverage targets:
- Overall backend: 70%+
- Critical packages (agent, terminal, api): 80%+
- Frontend coverage thresholds are enforced via Vitest.

## Known untested paths

- Prompt injection sequencing and timing edge cases beyond current coverage.
- Session lifecycle teardown under error conditions.
- WebSocket reconnection behavior (frontend store + backend integration).

## Test patterns in this repo

- Fake PTYs: `internal/api/rest_test.go`, `internal/terminal/manager_test.go`.
- Table-driven tests: `internal/agent/agent_test.go`, `internal/terminal/shell_test.go`.
- Integration helpers: websocket harnesses in `internal/api/*_integration_test.go`.

## Best practices

- Prefer table-driven tests for parsing/validation logic.
- Keep tests deterministic: mock PTYs, WebSockets, and fetch/WS clients.
- Use `t.TempDir()` for filesystem work; avoid shared state.
- Keep unit tests fast; isolate integration tests with clear timeouts.
- Add fuzz tests for parsing or boundary-heavy code paths.

## Test types

- Unit tests: pure logic (parsers, stores, validation).
- Integration tests: API handlers, websocket bridges, fake PTY flows.
- E2E tests: full server flow (REST + WebSocket) with real routing.

## Fixtures and helpers

- PTY fakes live in `internal/terminal/manager_test.go` and `internal/api/rest_test.go`.
- WebSocket helpers live in `internal/api/terminal_handler_integration_test.go`.

## Debugging

- Go: `go test ./... -run TestName -v`
- Frontend: `cd frontend && npm test -- --run tests/file.test.js`

## Performance guidance

- Unit tests: <10ms each when possible.
- Integration tests: <100ms each; avoid long sleeps.

## When tests are hard to write

- `cmd/gestalt` server startup and signal handling depend on process lifecycle.
  Prefer testing helper functions (config loading, static dir detection, agent load)
  and keep full server tests minimal.

## CI

GitHub Actions runs Go unit tests plus frontend Vitest runs (including coverage).

## CLI flag conventions

- Flag names are lowercase env var names with underscores replaced by dashes (GESTALT_FOO_BAR -> --foo-bar).
- Priority: CLI flag overrides env var, env var overrides default.
- Help format: "Description (env: GESTALT_FOO, default: value)" for each flag.
- Common flags: --help, --version, --verbose.
- Tool-specific flags: gestalt (server config), gestalt-send (client config), gestalt-desktop (gestalt flags plus window config).
- Exit codes: 0 success, 1 usage error, 2 runtime error, 3 network error.
- Subcommands: gestalt validate-skill, gestalt completion.
- CLI framework: stdlib flag stays for now; cobra/viper deferred until CLI complexity grows.
