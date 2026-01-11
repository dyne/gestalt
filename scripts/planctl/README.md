# planctl

`planctl` is a small Go CLI for reading and updating `PLAN.org` safely.
It is designed for deterministic, LLM-friendly automation.

## Build

```
cd scripts/planctl
GOMODCACHE=/tmp/gomodcache GOPATH=/tmp/gopath GOCACHE=/tmp/gocache /usr/local/go/bin/go test ./...
```

## Output format (machine-friendly)

Commands that list headings emit tab-delimited rows:

```
<level>\t<status>\t<priority>\t<title>\t<line>
```

Fields are empty if not present. `line` is 1-based.

## Common commands

Show current WIP L1/L2 (if any):
```
planctl current --file PLAN.org
```

List L1 headings:
```
planctl list --level 1 --status WIP
```

Find headings by substring:
```
planctl find --query "touch" --level 2
```

Set a heading status:
```
planctl set --level 2 --title "Implement pointer event handlers with type detection" --status DONE
```

Mark an L1 DONE and clear L2 statuses:
```
planctl complete-l1 --title "Fix terminal natural touch scrolling"
```

Insert a new TODO L2 under an L1:
```
planctl insert-l2 --l1-title "Fix terminal natural touch scrolling" --title "Follow-up tweak" --priority B
```

Print a full L1 section:
```
planctl show --title "Fix terminal natural touch scrolling"
```

Lint for multiple WIP entries:
```
planctl lint
```

## LLM usage notes

- Prefer `list` and `find` to discover exact titles before `set` or `complete-l1`.
- Use `current` when you need the active WIP L1/L2 quickly.
- Use `lint` to verify that the PLAN has exactly one WIP L1 and L2.
- For `.gestalt/PLAN.org`, pass `--file .gestalt/PLAN.org`.
