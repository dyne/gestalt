# Prompt Templating (Human Guide)

Gestalt supports a small templating system for prompt files. It is designed to
be simple and predictable.

## When templating runs

Templating runs for any prompt file in this directory.

- `.tmpl`, `.md`, `.txt`: directives are expanded.
- Other prompt files also process directives.

When an agent references a prompt name without an extension, Gestalt tries:
`.tmpl`, then `.md`, then `.txt`.

## Directives

Directives must appear on their own line (no other text on the line).

### Include another file

```text
{{include plan-structure}}
```

Behavior:

- If no extension is provided, Gestalt tries `.tmpl`, `.md`, then `.txt`.
- Path includes (for example `{{include ./AGENTS.md}}`) resolve from the repo
  root.
- Includes must be text files. Binary files are skipped.
- Missing includes are skipped silently.
- Depth limit is 3 levels. Cycles and depth overflow return an error.
- The same included file is only rendered once per prompt render.

Search order for bare includes (like `plan-structure`):

1. The prompt directory (`.gestalt/config/prompts` or `config/prompts` in dev).
2. `.gestalt/prompts`.

### Insert a runtime port

```text
{{port backend}}
```

Behavior:

- The line is replaced by the numeric port and a newline.
- Unknown services (or missing port data) are removed silently.

Common services:

- `backend`
- `frontend`
- `temporal`
- `otel`

## Practical tips

- You can use directives in any prompt file, regardless of extension.
- Keep directives on their own lines.
- Prefer small, reusable fragments (for example `decision-making.md`).
- If something does not expand, check the extension and the line formatting
  first.
