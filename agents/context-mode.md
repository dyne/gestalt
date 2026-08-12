# Context mode

Context mode keeps raw, potentially huge tool output in an indexed local
context and returns only the derived findings needed for the current decision.
It is a transport and memory layer, not another agent.

## Typical routing

| Need | Operation |
| --- | --- |
| Run several inspections and query their combined output | `ctx_batch_execute` |
| Analyze one large file without reading it into chat | `ctx_execute_file` |
| Recall indexed project facts or earlier session decisions | `ctx_search` |
| Persist a file tree for later queries | `ctx_index` |
| Inspect savings and activity | `ctx_stats` |

Short, fixed output and file edits remain on native tools. Context mode does
not broaden the authority granted to those tools.

## Runtime boundary

The replaceable Codex plugin cache contains launchers. The built runtime lives
under:

```text
~/.gestalt/runtime/context-mode/<version>/<platform-architecture-node-abi>/
```

Normal Codex startup only verifies and launches it. Installation, compilation,
and repair happen explicitly during `gestalt install` or `gestalt update`.

## Health check

Start a new `gestalt cli` session and run `ctx-doctor`. If it reports
`CONTEXT_MODE_NOT_PREPARED`, run:

```sh
gestalt update
```

If the failure remains, reinstall with the upstream setup's `--force` option as
described in the [source Agents guide](../reference/upstream/gestalt-agents-readme.md).
