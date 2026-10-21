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

## Compact responses and complete evidence

The current package defaults routine execute, file, batch, and search responses
to **2048 serialized UTF-8 bytes**, including metadata. A response can summarize
large captured output without displaying it all. Preserve its exit status,
coverage, first actionable failure, failure count, and authorized overflow
reference before drawing a conclusion.

Check the actual tool schema exposed by your session: an installed session may
have older fields. Where available, `response_bytes` selects a bounded display
budget. Retrieve a returned report through `ctx_search` using its report ID,
returned offset, and the next offset until `nextOffset` is null:

```json
{"response_report":"<returned report id>","response_offset":0,"response_bytes":2048}
```

These are tool arguments, not a shell command. Use the real returned ID and
offset. Retrieval rechecks authorization; a denied, missing, changed, or expired
report leaves evidence incomplete. Search hits cover retrieved chunks, not an
entire log. Recover every failure page before claiming complete failure coverage.
If your installed schema lacks continuation fields, derive a bounded summary
from the authorized saved output or use a bounded native read. Do not restart or
reconfigure a live session to gain new fields.

Reuse a source window only when its repository, path, and digest still match.
Reuse test evidence only for matching code, dirty changes, dependencies/config,
exact command, environment, and CI run/attempt/job; keep its original observation
time. State the unresolved question and required source range before requesting
a larger response. A smaller response alone proves neither complete evidence
nor lower provider usage.

## Reading usage measurements

| Measurement | Meaning |
| --- | --- |
| Visible response bytes | Captured serialized tool envelopes; include overflow retrieval in a comparison |
| Context occupancy | Tokens currently occupying a context window; compaction can reduce this |
| Cumulative provider usage | Observed input/output/cache/reasoning counters over a defined source and interval, including repeated turns |
| Account quota or billed cost | Separate provider observations; unknown unless directly available |

Do not add reasoning tokens to output totals or cached subsets to input totals
again. Counter resets, missing fields, unknown roles, and missing children keep
coverage partial. Compaction does not undo cumulative usage. Bytes, occupancy,
and provider counters cannot be converted into account quota savings.

Synthetic corpus comparisons can check response sizes and recoverable findings
with fixed source/dependency revisions. They do not establish live task quality,
quota, or billing savings. Live model comparisons are opt-in: obtain explicit
trial authorization, freeze the task/tools/acceptance criteria, retain failed
trials and retries, count all agents and overflow retrieval, and report coverage
alongside results. A comparison-manifest validator does not launch models or
authorize a trial. No particular savings are guaranteed.

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
