# Codex MCP server notes (2026-01-31)

These notes were captured by running `codex mcp-server` locally with
`CODEX_HOME=/tmp/codex` and a small NDJSON client.

## Transport
- Stdio framing is NDJSON (one JSON-RPC message per line).
- Sending LSP-style `Content-Length` headers fails to parse (stderr shows
  `Failed to deserialize JSONRPCMessage`).

## Initialize handshake
Request (line-delimited JSON):

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"gestalt-probe","version":"0.1"}}}
```

Response (single-line JSON):

```json
{"id":1,"jsonrpc":"2.0","result":{"capabilities":{"tools":{"listChanged":true}},"protocolVersion":"2024-11-05","serverInfo":{"name":"codex-mcp-server","title":"Codex","version":"0.92.0"}}}
```

After initialize, a `{"jsonrpc":"2.0","method":"initialized","params":{}}`
notification was accepted.

## tools/list
The server exposes two tools:
- `codex`
- `codex-reply`

`codex` input schema highlights:
- `prompt` (required)
- `approval-policy` (enum: untrusted | on-failure | on-request | never)
- `base-instructions`
- `compact-prompt`
- `developer-instructions`
- `config` (object for config overrides)
- `cwd`, `model`, `profile`, `sandbox`

`codex-reply` input schema highlights:
- `threadId` (preferred)
- `conversationId` (deprecated, kept for backward compatibility)
- `prompt` (required)

`tools/list` output schema claims `threadId` + `content` strings for both
tools, but real responses may include richer structures (see below).

## codex tool call: events + response shape
A `tools/call` for `codex` immediately triggers server notifications
with method `codex/event` and a `_meta` block containing `requestId`
(the original call id) and `threadId`.

Observed `codex/event` message types (non-exhaustive):
- `session_configured`
- `mcp_startup_complete`
- `task_started`
- `raw_response_item`
- `item_started` / `item_completed`
- `user_message`
- `stream_error`

Example event envelope (truncated):

```json
{"jsonrpc":"2.0","method":"codex/event","params":{"_meta":{"requestId":3,"threadId":"..."},"id":"","msg":{"type":"session_configured"}}}
```

With no auth configured, the final `tools/call` response for id=3 returned
an error payload like:

```json
{"id":3,"jsonrpc":"2.0","result":{"content":[{"text":"unexpected status 401 Unauthorized: ","type":"text"}],"isError":true,"structuredContent":{"threadId":"...","content":"unexpected status 401 Unauthorized: "}}}
```

Notable: the response `result` includes `content` as an array of text
chunks, `isError`, and `structuredContent.threadId`. This does not match
the `tools/list` output schema exactly, so code should be defensive.

## Stderr observations
- When `CODEX_HOME=/tmp/codex`, stderr warned about helper binaries
  under `/tmp`.
- Without auth, stderr included repeated 401 logs for model refresh and
  response streaming.

## Implications for Gestalt
- Use NDJSON framing (newline-delimited JSON) for stdio MCP.
- Expect `codex/event` notifications interleaved with `tools/call` results.
- Surface MCP notifications in the session console as `[mcp <method>]` lines,
  using best-effort `codex/event` msg parsing and truncating long params.
- Treat `threadId` as the stable conversation handle.
- Be tolerant of response shapes beyond the advertised output schema.
