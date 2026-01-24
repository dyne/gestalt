# otelcol-gestalt

This directory contains the OpenTelemetry Collector build configuration for Gestalt.

Build
- Ensure Go is installed.
- From this directory, run:
  - `make Linux_x86_64`

Notes
- The build uses the OpenTelemetry Collector Builder (ocb).
- The builder config is `builder-config.yaml`.
- The resulting binary is `otelcol-gestalt` in this directory.
- `fileexporter` is sourced from the collector-contrib module.

Sample config
- `debug.yaml` sends OTLP logs to the debug exporter for local inspection.
