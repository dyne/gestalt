# Git Workflows

Use these steps to keep changes clean and reviewable.

## Safe sync
1. `git fetch --all --prune` to update remote refs.
2. `git rebase origin/main` (or your target branch) to keep history linear.

## New branch setup

git checkout -b feat/<short-name> (or fix/<short-name>)

## New Commit

Commit each L2 when done, include all files you modify or add.
Always use conventional commits specification in messages.

Git commands:

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
