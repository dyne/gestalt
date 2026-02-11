# Prompts, skills, and auth

## Prompt files

Prompt files are loaded from `.gestalt/config/prompts` and support `.tmpl`, `.md`, and `.txt`.

## Skills

Skill metadata lives under `.gestalt/config/skills` and is available to agents when listed in the profile.

## Auth token

`GESTALT_TOKEN` enables API auth checks.

- REST: `Authorization: Bearer <token>`
- WS/SSE: `?token=<token>` when needed
