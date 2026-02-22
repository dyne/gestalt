# Prompts, skills, and auth

## Prompt files

Prompt files are loaded from `.gestalt/config/prompts` and support `.tmpl`, `.md`, and `.txt`.

## Skills

Skill metadata lives under `.gestalt/config/skills` and is available to agents when listed in the profile.

## Flow files

Flow automation files are stored at runtime under `.gestalt/config/flows/*.flow.yaml`.

Packaged defaults are shipped from `config/flows/*.flow.yaml` and extracted into `.gestalt/config/flows` on startup.

Legacy JSON flow files are unsupported and ignored. There is no automatic migration path.

## Auth token

`GESTALT_TOKEN` enables API auth checks.

- REST: `Authorization: Bearer <token>`
- WS/SSE: `?token=<token>` when needed
