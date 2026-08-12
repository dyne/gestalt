# Sessions and Git

## Choose a workspace

The Sessions tab presents a recursive filesystem tree rooted at `--cwd`.
Directories containing `.git` are terminal nodes. Directory symlinks are shown
only when their real targets remain inside the root; cycles and duplicate real
targets are omitted.

Keyboard controls follow tree conventions: Left/Right collapse or expand,
Up/Down move, Home/End jump, and Enter or Space select.

## Start and reopen sessions

At startup, Mobile asks Codex app-server for available models. New sessions use
`gpt-5.6-terra` by default unless you choose another discovered model. Durable
threads are resumed after a relay restart.

Use **Open** for a released, stopped, or attention-required session. If a Codex
upgrade removed the saved rollout, Mobile can bind a replacement thread while
preserving the relay session and settings; it reports that earlier Codex
history is unavailable.

## Skill profiles

Global profiles live in `~/.gestalt/skill-profiles/<name>.yml`:

```yaml
version: 1
name: focused
skills:
  - name: typescript-advanced-types
    path: /absolute/path/to/SKILL.md
    enabled: true
```

```sh
gestalt mobile --skills list
gestalt mobile --cwd "$PWD" --skills focused
```

An explicit global profile wins over a workspace `gestalt-skills.yml`, which
wins over Codex-native selection. Mobile never rewrites Codex configuration or
skill files.

## Git operations

The Git tab has an independent filesystem selection. It shows branch
divergence, dirty counts, recent commits, and allowed repository actions.

- Pull uses `git pull --rebase`.
- Push is offered only for an upstream branch that is ahead and not behind.
- Mobile never creates an upstream and never force-pushes.
- Clone targets must be ordinary directories, not existing repositories.
