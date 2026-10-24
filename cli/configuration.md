# CLI configuration

The manager has conservative defaults and uses environment variables for
repeatable overrides.

| Variable | Default | Purpose |
| --- | --- | --- |
| `CODEX_HOME` | `~/.codex-gestalt` | Isolated Codex profile |
| `GESTALT_HOME` | `~/.gestalt` | Runtime, Mobile package, and skill profile root |
| `GESTALT_MARKETPLACE` | `dyne/gestalt-agents` | Marketplace source passed to Codex |
| `GESTALT_MARKETPLACE_NAME` | `dyne-gestalt-agents` | Codex checkout directory name |
| `GESTALT_MOBILE_VERSION` | `latest` | npm version or tag installed by the manager |
| `GESTALT_BIN_DIR` | `~/.local/bin` | Installer destination for the manager |
| `GESTALT_INSTALL_BASE_URL` | `https://dyne.github.io/gestalt` | Manager install and self-update source |

## Alternate isolated profile

```sh
CODEX_HOME="$HOME/.codex-gestalt-lab" gestalt install
CODEX_HOME="$HOME/.codex-gestalt-lab" gestalt cli
```

Use the same override on later updates. Paths must be absolute and cannot be
`/` or your home directory.

## Workspace and Git permissions

`gestalt install` and `gestalt update` configure the isolated Codex profile to
use `workspace-git` by default. This custom permission profile extends Codex's
workspace policy, keeps workspace `.agents` and `.codex` read-only, and makes
`.git` writable so development sessions can commit without full host access.
It also grants read-only access (including directory traversal and executable
use, but no writes) to `CODEX_HOME`, `GESTALT_HOME`, `~/.agents`, `~/.local`,
and `~/.config`. This includes the default `~/.codex-gestalt`, `~/.gestalt`, and
`~/.agents` locations. The profile defaults to `approval_policy = "never"`
and pairs it with `default_tools_approval_mode = "approve"` for apps, configured
MCP servers, and plugin MCP overrides. Explicit server defaults and per-tool
approval rules are preserved. Tool approval does not change sandbox permissions.

The manager migrates the dedicated profile away from legacy `sandbox_mode` and
`sandbox_workspace_write` settings while preserving unrelated configuration.
Gestalt Mobile selects `workspace-git` for new Codex sessions by default and
sends it to app-server as a named permission profile.

## Pin Mobile

```sh
GESTALT_MOBILE_VERSION="0.1.0" gestalt update
```

The Agents marketplace follows the version selected by Codex marketplace
upgrade. Gestalt Agents and its adapted context-mode runtime share one release
version.

## Supervision compatibility

Before launching Codex or Mobile, `gestalt doctor` reads two small local release
manifests: the Agents marketplace manifest and Mobile's installed package
manifest. The probe is offline and read-only. It checks supervision contract
version 1 and the capabilities needed for `supervision-start`, wait and
checkpoint tools, stuck-agent capacity recovery, controller status, canonical
agent identity, session verdicts, acknowledgement-safe drafts, and the Org Plan
contract. It never opens a session, prompt, Org Plan body, transcript,
credential store, or lease ID.

`ready (v1; offline manifests)` means the installed components agree. `UNAVAILABLE`
means a release did not publish its manifest; `INCOMPATIBLE` means a manifest is
malformed, stale, or lacks a named capability. The usual upgrade recovery is:

```sh
gestalt update
# then restart Gestalt Mobile
```

The manager continues its normal download and validation path; doctor never
downloads or silently changes a component.

The optional L1 model-policy check adds **SUPPORTED**, **LEGACY**, or
**INCOMPATIBLE** diagnostics. Supported means schema/configuration agreement,
with runtime enforcement unavailable; legacy packages retain base supervision.
New matched packages report **configured compatibility**, which lets existing
v1 plans run with explicit validated launch settings and controller ownership.
Their optional `MODEL_POLICY_ASSURANCE: runtime-attested` declaration remains
strict and blocked on adapters without receipts. Missing attestation does not
become another ordinary execution approval. Doctor validates declarations only;
it cannot establish child execution, review or checkpoint success.
An upgrade or rollback of only one package can be incompatible. See the
[manifest contract](../reference/supervision-capabilities.md) for exact scope,
setup rollback, and generated-role limitations.

## Verify the installation you will run

Run these read-only commands with the same `CODEX_HOME` and `GESTALT_HOME`
overrides used for installation:

```sh
gestalt version
gestalt path --json
gestalt doctor
```

Version output and resolved paths identify installed artifacts, rather than a
sibling source checkout. Doctor inspects installed package declarations; it does
not prove runtime model enforcement or production behavior. Source tests and
documentation builds validate their own candidate only. To establish installed
behavior, use a fresh isolated installation of the pinned candidate, verify
package identities and capabilities there, and retain its actual checks.

Successful setup records manager version/source digest, delegated setup digest,
and config/hooks/role-file digests in
`<CODEX_HOME>/.gestalt-setup-provenance.json`. This record describes setup-time
artifacts. Compare current bytes before reuse; it does not attest a running
session or later edits. Its `runtimeEnforcement` field remains false.

## Compatible rollback

Before an intentional update, close affected sessions and retain a known-good
copy of the manager executable plus the dedicated Codex and Gestalt homes with
their modes. Keep the matched Agents/context-mode and Mobile revisions together.
A backup can contain credentials and session data; keep it private.

If an update fails, distinguish the manager's config rollback from package
rollback: the setup stage restores previous config, but does not undo downloaded
packages. There is no manager rollback subcommand. Restore the known-good
manager and matched isolated homes together using your normal backup procedure,
then run `gestalt doctor` and restart Mobile/new sessions. Pinning only
`GESTALT_MOBILE_VERSION` does not pin Agents and cannot establish compatible
rollback. Verify the restored manifests; do not edit them to claim support.
Legacy pairs keep base supervision without silently acquiring model policy.

## Transfer skills between installations

Export user skills, Gestalt-managed extra skills, Codex-profile skills, and
global skill profiles into one portable archive:

```sh
gestalt skills-export ~/gestalt-skills.tar.gz
```

On another installation, import the archive with the target installation's
normal `HOME`, `CODEX_HOME`, and `GESTALT_HOME` values:

```sh
gestalt skills-import ~/gestalt-skills.tar.gz
```

Import validates the archive before writing, rejects links and unsafe paths,
merges archived skill directories without deleting unrelated skills, and
atomically replaces matching profile files. Absolute profile paths under the
three exported skill roots are rewritten for the destination installation.
Codex system skills and versioned plugin caches are not archived because
`gestalt install` or `gestalt update` recreates them; profiles referring to
plugin skills retain their paths for Mobile's normal version-rebinding logic.
