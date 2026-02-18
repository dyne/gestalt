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
- gopkg.in/yaml.v3: YAML frontmatter parsing for skills metadata in
  internal/skill.
