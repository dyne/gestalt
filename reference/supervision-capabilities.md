# Supervision capability manifests

Release artifacts publish a bounded JSON manifest so `gestalt doctor` can check
supervised continuity without starting a session or contacting a service.

Agents publishes:

```text
<CODEX_HOME>/.tmp/marketplaces/<GESTALT_MARKETPLACE_NAME>/plugins/gestalt/gestalt-supervision-capabilities.json
```

Mobile publishes:

```text
<GESTALT_HOME>/mobile/node_modules/gestalt-mobile/gestalt-supervision-capabilities.json
```

Both use this public, non-secret shape:

```json
{
  "schemaVersion": 1,
  "component": "agents-or-mobile",
  "supervisionContract": 1,
  "capabilities": ["named-capability"]
}
```

Agents must publish `supervision-start`, `wait-lease-tool`, `checkpoint-tool`,
`canonical-agent-identity`, and `org-plan-contract`. Mobile must publish those
plus `controller-status`, `session-verdict`, and
`acknowledgement-safe-composer`. The doctor reads at most 16 KiB, accepts only
this schema and string capability names, and compares contract versions rather
than inferring compatibility from package versions.

Missing metadata is **UNAVAILABLE**. Invalid metadata, a stale contract, or a
missing named capability is **INCOMPATIBLE**. The one operator action is
`gestalt update`, followed by a Mobile restart. The check is intentionally
offline, read-only, and incapable of reading plan bodies, prompts, transcripts,
credentials, or lease/control IDs.
