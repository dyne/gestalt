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
`agent-capacity-recovery`, `canonical-agent-identity`, and `org-plan-contract`.
Mobile must publish those plus `controller-status`, `session-verdict`, and
`acknowledgement-safe-composer`. The doctor reads at most 16 KiB, accepts only
this schema and string capability names, and compares contract versions rather
than inferring compatibility from package versions.

Missing metadata is **UNAVAILABLE**. Invalid metadata, a stale contract, or a
missing named capability is **INCOMPATIBLE**. The usual upgrade recovery is
`gestalt update`, followed by a Mobile restart. The check is intentionally
offline, read-only, and incapable of reading plan bodies, prompts, transcripts,
credentials, or lease/control IDs.

## L1 model-policy agreement

The manager also negotiates the optional `l1-model-policy-v1` capability. A
compatible Agents/Mobile pair declares `l2-report-boundary` and the following
additional field (the Mobile scope is `schema-and-retention`):

```json
"modelPolicyContract": {
  "version": 1,
  "scope": "schema-and-local-contracts",
  "runtimeEnforcement": false
}
```

`gestalt doctor` distinguishes:

- **SUPPORTED**: both packages declare the v1 capability, expected component
  scopes, report boundary, and `runtimeEnforcement: false`. This establishes
  schema/configuration diagnostics and Mobile retention of policy metadata.
  Unsupported automatic policy launches remain rejected by the owning package.
- **LEGACY**: neither package declares model policy or its contract. Existing
  base supervision remains available; no policy support is inferred from package
  presence or version.
- **INCOMPATIBLE**: only one package was updated or rolled back, a contract or
  scope is unsupported, a report boundary is missing, or runtime enforcement is
  claimed. Update or roll back Agents and Mobile together, then restart Mobile.
  A capability without its typed contract, or a contract without its capability,
  is malformed metadata.

These are offline package diagnostics. They do not attest effective runtime
settings, enable automatic launches, or prove native child admission enforcement.
The manager delegates policy validation and launch decisions to the owning
packages; it does not evaluate plans or rewrite role profiles here. Missing or
malformed base manifests retain the diagnostics above. A failed doctor check
reports the affected contract while ordinary commands retain their existing
behavior.

## Setup ownership and provenance

`gestalt install` and `gestalt update` delegate role generation to the Agents
marketplace setup. The manager forwards its supported `--extra-skills` option;
model-policy defaults are not a manager setup option. Unknown options fail
before setup. Plan policy validation remains in Agents, and launch decisions
remain with the owning runtime package.

Setup serializes writers with `<CODEX_HOME>/.gestalt-setup.lock`. A competing
installer fails without changing config. After a killed installer, confirm it
has exited before removing a stale lock. Ordinary launches do not run setup or
rewrite profiles and can share a read-only installed Codex home.

The manager restores the previous `config.toml` bytes and mode when its setup
stage fails, including a missing previous config. This rollback covers manager
configuration; it does not roll back downloaded packages or replace the delegated
owner's profile transaction. Diagnose a partial package update with
`gestalt doctor`, then update or roll back the affected packages together.

A successful setup writes `<CODEX_HOME>/.gestalt-setup-provenance.json`, containing
the manager version and SHA256, delegated setup script SHA256, and digests of
config, hooks, and the two managed role profiles. Missing outputs are `null`.
The record is deterministic and contains no config or instruction bodies. It
identifies setup artifacts with `scope: setup-artifacts-only` and
`runtimeEnforcement: false`; it neither attests current runtime settings nor
claims later user edits still match. Repeated setup keeps one manager permission
block, preserves unrelated model/config/skill choices, and retains delegated
ownership of managed role instructions. Explicit install/update regenerates
`org-plan-executor.toml` and `org-plan-reviewer.toml` through Agents setup; manual
model/effort edits inside those generated files are not preserved. Preserving
those edits is deferred to separate Agents work. User configuration and unmanaged
profiles remain intact, and ordinary launches preserve all installed profiles.
