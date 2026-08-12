# Gestalt Agents

Gestalt Agents is a Codex marketplace containing the `gestalt` workflow plugin
and the adapted `context-mode` runtime plugin. It turns substantial development
work into a durable, reviewable sequence without filling the main conversation
with raw logs.

## The operating model

```text
director (depth 0, read-only reviewer and supervisor)
└── executor (depth 1, the only code writer)
```

The director stays in the user's conversation, launches one fresh executor per
top-level milestone, reviews evidence, and accepts or rejects the uncommitted
result. The executor creates one conventional commit only after acceptance.

Context mode carries large evidence and indexed memory. It does not spawn
agents, grant permissions, or perform side effects by itself.

## What the plugin installs

- Five development workflow skills.
- Eight context-mode routing and maintenance skills.
- Read-only reviewer and writing executor profiles.
- A versioned context-mode runtime under `~/.gestalt/runtime`.
- Codex lifecycle hooks and an MCP server registered by the plugin manifests.

Continue with the [supervised workflow](./workflow.md) or browse the complete
[skill catalog](./skills.md).
