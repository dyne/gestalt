---
name: terminal-navigation
description: Terminal navigation shortcuts and safe command patterns.
license: MIT
compatibility: ">=1.0"
metadata:
  owner: dyne
allowed_tools:
  - bash
---

# Terminal Navigation

Tips for moving quickly and safely in the shell.

## Movement
- `cd -` toggles between the last two directories.
- `pushd` and `popd` manage a directory stack.
- `ls -la` for full directory context.

## Search
- `rg` for fast text search.
- `find . -maxdepth 2 -type f` for quick file discovery.

## Safety
- Use `rm -i` when deleting unfamiliar files.
- Prefer `mv` over `cp` to avoid stale copies.
- Confirm shell expansions with `echo` before running destructive commands.
