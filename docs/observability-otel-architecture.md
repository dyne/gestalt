# OpenTelemetry integration architecture

Goal
- Replace Gestalt's custom logging/event/metrics with OpenTelemetry for logs, metrics, and traces.
- Keep OTLP as the on-the-wire standard to avoid vendor lock-in.
- Run a local collector that can later be swapped for remote backends.

Scope
- Backend: emit OTLP logs, metrics, traces from Go services.
- Collector: otelcol-gestalt started with the server and stopped on exit.
- Frontend: read logs and metrics via Collector HTTP endpoints (OTLP/HTTP).
- Temporal: propagate trace context and record workflow spans/attributes.

High-level design
- Gestalt server owns a local Collector lifecycle (start before Temporal dev server, stop on exit).
- Backend SDK exports OTLP to the local Collector:
  - Traces: OTLP/HTTP by default, gRPC optional.
  - Metrics: OTLP/HTTP by default, gRPC optional.
  - Logs: OTLP/HTTP by default, gRPC optional.
- Collector pipeline:
  - Receiver: otlpreceiver (grpc + http)
  - Processor: batchprocessor
  - Exporters: otlpexporter (future remote), fileexporter (local persistence)

Runtime topology
- Server process:
  - starts collector as a child process
  - initializes OTel SDK providers and exporters
  - wires HTTP middleware and event/log bridges
- Collector process:
  - listens on localhost (OTLP gRPC 4317, OTLP HTTP 4318)
  - writes persistent log/metric/trace files to .gestalt/otel/

Configuration
- New config block (server):
  - otel.enabled (bool, default true)
  - otel.endpoint (string, default http://127.0.0.1:4318)
  - otel.service_name (string, default gestalt)
  - otel.resource_attributes (map[string]string)
  - otel.exporter (http|grpc)
  - otel.log_level (INFO default for frontend)
- Collector config file:
  - Location: .gestalt/otel/collector.yaml
  - Owned by Gestalt (rendered at startup with ports and file paths)
- Environment variables (runtime):
  - GESTALT_OTEL_ENABLED (collector on/off)
  - GESTALT_OTEL_COLLECTOR (collector binary path)
  - GESTALT_OTEL_CONFIG (collector config path)
  - GESTALT_OTEL_DATA_DIR (collector data dir)
  - GESTALT_OTEL_GRPC_ENDPOINT / GESTALT_OTEL_HTTP_ENDPOINT (collector listen endpoints)
  - GESTALT_OTEL_REMOTE_ENDPOINT (optional OTLP gRPC exporter target)
  - GESTALT_OTEL_REMOTE_INSECURE (true to skip TLS verification for remote exporter)
  - GESTALT_OTEL_SELF_METRICS (true to enable collector self-metrics)
  - GESTALT_OTEL_MAX_RECORDS (cap records read from local otel.json for APIs)
  - GESTALT_OTEL_SDK_ENABLED (SDK on/off)
  - GESTALT_OTEL_SERVICE_NAME (service.name override)
  - GESTALT_OTEL_RESOURCE_ATTRIBUTES (comma-separated key=value list)
- Port selection:
  - Defaults to 127.0.0.1:4317 (gRPC) and 127.0.0.1:4318 (HTTP).
  - If defaults are occupied and no endpoint env vars are set, Gestalt picks an available adjacent port pair and logs the selection.
  - Setting GESTALT_OTEL_GRPC_ENDPOINT or GESTALT_OTEL_HTTP_ENDPOINT disables randomization for the collector.

Resource model
- Resource attributes (static):
  - service.name=gestalt
  - service.version=<build version>
  - service.instance.id=<hostname or random instance id>
  - os.type, os.version
  - build.commit, build.time (if available)

Log mapping
- internal/logging.LogEntry -> OTel LogRecord
  - body: message
  - severity: mapped from Level
  - attributes: context map
  - timestamp: LogEntry.Timestamp
- Event bus records -> LogRecord with type attributes:
  - event.bus
  - event.type
  - event.payload (structured where possible)

Metrics mapping
- Replace internal/metrics.Registry metrics with OTel instruments:
  - workflows.started/completed/failed/paused
  - event_bus.subscribers (gauge)
  - events.published/dropped (counter)
  - terminal.sessions (gauge)

Tracing model
- HTTP server spans: per-request spans with standard http.* attributes.
- Explicit spans for key actions:
  - terminal.create, terminal.delete, agent.input, terminal.output
- Propagate trace context:
  - from inbound HTTP headers (traceparent)
  - into WebSocket connect spans
  - into Temporal workflow start and activity contexts

Frontend access
- Logs ingest: POST /api/otel/logs (OTLP LogRecords).
- Traces: /api/otel/traces (trace_id/span_name/since/until/limit/query).
- Metrics: /api/otel/metrics (name/since/until/limit/query).
- Log stream: /api/logs/stream (SSE, OTLP LogRecords) with a last-hour replay on connect.

Log retention and replay
- Collector writes otel.json; Gestalt rotates it by size/age/count limits.
- Retrieval is via /api/logs/stream (last-hour replay from LogHub) and local files.

Migration plan (high level)
- Phase 1: run OTel in parallel with existing logging and event bus.
- Phase 2: wire OTel to all API endpoints and key events.
- Phase 3: swap frontend log source to OTel.
- Phase 4: remove internal/logging and internal/metrics usage.

Testing strategy
- Unit tests for log and event mapping to OTel attributes.
- Integration tests for OTLP exporter wiring (mock OTLP endpoint).
- End-to-end tests for HTTP span creation and error tagging.

Open questions
- Final storage format for fileexporter (JSON or OTLP/Protobuf)
- Retention policy for .gestalt/otel/ data
- Whether to expose Collector port to LAN or keep localhost only
