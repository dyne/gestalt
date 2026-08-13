# Using Gestalt Mobile

## Choose a workspace

The Sessions tab presents a recursive filesystem tree rooted at `--cwd`.
Directories containing `.git` are terminal nodes. Directory symlinks are shown
only when their real targets remain inside the root; cycles and duplicate real
targets are omitted.

Keyboard controls follow tree conventions: Left/Right collapse or expand,
Up/Down move, Home/End jump, and Enter or Space select.

<MobileScreenshot
  src="03-workspace-selection.png"
  alt="Sessions tab with an expanded filesystem tree and a Git repository selected."
  caption="The Sessions tree stops at repository boundaries and keeps the selected working directory visible."
  eager
/>

## Start and reopen sessions

At startup, Mobile asks Codex app-server for available models. New sessions use
`gpt-5.6-terra` by default unless you choose another discovered model. Durable
threads are resumed after a relay restart.

Use **Open** for a released, stopped, or attention-required session. If a Codex
upgrade removed the saved rollout, Mobile can bind a replacement thread while
preserving the relay session and settings; it reports that earlier Codex
history is unavailable.

<MobileScreenshot
  src="04-session-setup.png"
  alt="New-session form with skill profile, sandbox, and approval policy controls."
  caption="Session policy is explicit before Codex starts; Chat stays disabled until a session is ready."
/>

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

<MobileScreenshot
  src="05-skill-profiles.png"
  alt="Skill profile editor with Playwright and verification skills enabled."
  caption="Save complete skill selections as named profiles for repeatable sessions."
/>

## Chat and approvals

Chat restores the canonical Codex thread timeline and follows live output. It
keeps command approvals, file-change approvals, and bounded questions beside
the turn that requested them. Pending interactions remain durable across a
browser reload.

<div class="mobile-shot-grid">
  <MobileScreenshot
    src="07-chat.png"
    alt="Chat tab with a user prompt, collapsed commentary, and final answer."
    caption="Prompts, commentary, final answers, timing, and relay readiness stay together."
  />
  <MobileScreenshot
    src="09-file-approval.png"
    alt="Chat tab listing files requested by a pending file-change approval."
    caption="File approvals name every target before the user accepts or denies the write."
    :height="880"
  />
</div>

## Supervised plans

The Plan tab belongs to the selected session. It presents ordered milestones,
nested steps, the current action, skill assignments, progress, and review
status. Completing implementation does not hide the distinction between work
that is awaiting review and work whose evidence has been accepted.

<div class="mobile-shot-grid">
  <MobileScreenshot
    src="12-plan-progress.png"
    alt="Plan tab showing a nested current step and one of three steps complete."
    caption="The active milestone stays readable while nested work advances."
  />
  <MobileScreenshot
    src="13-plan-review.png"
    alt="Plan tab showing completed implementation that is still awaiting review."
    caption="Review status remains explicit after implementation reaches done."
  />
</div>

## Git operations

The Git tab has an independent filesystem selection. It shows branch
divergence, dirty counts, recent commits, and allowed repository actions.

- Pull uses `git pull --rebase`.
- Push is offered only for an upstream branch that is ahead and not behind.
- Mobile never creates an upstream and never force-pushes.
- Clone targets must be ordinary directories, not existing repositories.

<MobileScreenshot
  src="15-git.png"
  alt="Git tab showing branch divergence, dirty counts, recent commits, and safe repository actions."
  caption="The Git tab separates repository inspection from the Sessions workspace selection."
/>

## Scratchpad

Scratchpad is a browser-local note surface reached from configuration. Notes
persist without becoming prompts or modifying the repository.

<MobileScreenshot
  src="16-scratchpad.png"
  alt="Gestalt Mobile scratchpad with a local note editor."
  caption="Keep temporary context close without adding it to the Codex thread."
  :width="375"
  :height="964"
/>
