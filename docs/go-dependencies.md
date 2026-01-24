# Go dependencies

This document lists the direct Go module dependencies from go.mod and why they
exist. Indirect dependencies are pulled in by these modules. Update this file
when adding or removing direct dependencies.

## Direct dependencies (go.mod)

- github.com/creack/pty: PTY support for terminal sessions in internal/terminal.
- github.com/fsnotify/fsnotify: filesystem watching for config and repo events in
  internal/watcher.
- github.com/gorilla/websocket: WebSocket transport for terminal and event
  streams in internal/api.
- go.temporal.io/sdk: Temporal workflow runtime used by internal/temporal and
  workflow-enabled terminal sessions.
- gopkg.in/yaml.v3: YAML frontmatter parsing for skills metadata in
  internal/skill.
- github.com/klauspost/compress: zstd compression for SCIP index handling in
  internal/scip.
- github.com/mattn/go-sqlite3: SQLite driver for SCIP index storage in
  internal/scip.
- github.com/sourcegraph/scip: SCIP schema and helpers used for code
  intelligence indexing and queries in internal/scip.
- go.temporal.io/api: Temporal API types and service errors used by
  internal/temporal and workflow REST endpoints.
- golang.org/x/time: rate limiter for SCIP endpoints in internal/api.
- google.golang.org/protobuf: protobuf support for SCIP index merging and tests
  in internal/scip.

## Notes

- Temporal and SCIP introduce sizable indirect dependency trees. Keep feature
  flags and build tags aligned with these modules to avoid pulling them when not
  needed.
