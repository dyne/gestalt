# Gestalt

Gestalt is a local dashboard and API server for running multiple agent sessions in parallel. It gives you one place to start, monitor, and automate coding workflows with terminal streams, workflow controls, and event feeds.

The project combines a Go backend, a Svelte frontend, and optional standalone binaries for agent-first workflows. Use this documentation as the source of truth for setup, operation, configuration, and API details.

## Quick Setup

```sh
# Install dependencies
npm i
make

# Start Gestalt dashboard
./gestalt
# Opens http://localhost:57417
```

## Read Next

- [Run Gestalt on a project](./getting-started/running-on-a-project)
- [Agent configuration](./agent-configuration)
- [CLI reference](./reference/cli)
- [HTTP API reference](./reference/http-api)
- [Temporal workflow notes](./reference/http-api#workflow-and-metrics-endpoints)
- [OpenTelemetry architecture](./observability-otel-architecture)
- [Testing guide](./guides/build-dev-testing#testing)

