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
~/.gestalt/runtime/context-mode/
```

Normal Codex startup only verifies and launches it. Installation, compilation,
and repair happen explicitly during `gestalt install` or `gestalt update`.
Updates prepare and verify a replacement before switching, then remove the old
runtime. Existing version directories are removed during this migration. A
failed build preserves the previous runtime. Restart sessions after updating.

The CLI is available at `~/.gestalt/bin/context-mode`, alongside `org-plan`.
Add the directory to your shell's `PATH`:

```sh
export PATH="$HOME/.gestalt/bin:$PATH"
context-mode doctor
```

With a custom `GESTALT_HOME`, use its `bin` directory instead. Gestalt adds this
directory to the environment of CLI and Mobile sessions automatically.

## Health check

Start a new `gestalt cli` session and run `ctx-doctor`. If it reports
`CONTEXT_MODE_NOT_PREPARED`, run:

```sh
gestalt update
```

If the failure remains, reinstall with the upstream setup's `--force` option as
described in the [source Agents guide](../reference/upstream/gestalt-agents-readme.md).
