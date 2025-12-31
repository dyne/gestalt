---
name: git-workflows
description: Git workflows and safety checks for everyday changes.
license: MIT
compatibility: ">=1.0"
metadata:
  owner: dyne
allowed_tools:
  - bash
---

# Git Workflows

Use these steps to keep changes clean and reviewable.

## Safe sync
1. `git status` to confirm a clean working tree.
2. `git fetch --all --prune` to update remote refs.
3. `git rebase origin/main` (or your target branch) to keep history linear.

## New branch setup
1. `git checkout -b feature/<short-name>`.
2. Commit in small, reviewable chunks.
3. Push with `git push -u origin feature/<short-name>`.

## Before opening a PR
1. `git status` and `git diff` to verify changes.
2. `git log --oneline --decorate -n 10` to check commit history.
3. Squash or fixup commits if needed.

## Recover tips
- `git reflog` to find previous HEAD positions.
- `git reset --hard <sha>` only when you are sure no work will be lost.
