# Git Workflows

Use these steps to keep changes clean and reviewable.

## Safe sync
1. `git fetch --all --prune` to update remote refs.
2. `git rebase origin/main` (or your target branch) to keep history linear.

## New branch setup

git checkout -b feat/<short-name> (or fix/<short-name>)

## New Commit

Commit in small, reviewable chunks, all files you modify or
add. Commit message should prefix with (all downcase, include colon)

- fix: (just fixing anything)
- feat: (adding a feature)
- build: (improving the build system)
- chore: (minor cleaning up, fix typos, etc.)
- ci: (continuous integration changes, github actions etc.)
- docs: (documentation)
- test: (testing framework changes)

after the colon just a title on one line and below a short
description. In case of more complex commits, reuse a compact
reelaboration of the L2 instructions.

Then proceed with git commands:

git add path/to/file
git commit

## Check history

If for any reason you need to trace back changes, check commit history

git log --oneline --decorate -n 10

## Recover tips

To find previous HEAD positions:

git reflog

## Prepare for push

Squash or fixup commits if needed.

Don't push commits yourself, notice your work is finished.
