# Native CLI migration contract

Baseline: manager `9d44b4d70634ef1be371e7b9f215b958bb93b154` (0.16.0).
`public/gestalt` remains the released implementation. `npm/gestalt` resolves
symlinks and execs it with `GESTALT_MANAGER_DISTRIBUTION=npm`; global npm
postinstall runs only `agents install`, with npm global/prefix settings removed.
Local installs and inspection do not provision profiles. Linux/macOS remain the
package OS allowlist. The TypeScript contracts are migration seams; adapters and
command handlers are introduced in subsequent milestones, then replace Bash.

Development commands (Node 24+):

```sh
npm run typecheck:cli
npm run build:cli
npm run test:contracts
npm run dev:cli -- --help
node dist/cli/gestalt.mjs --version
```

`build:cli` bundles TypeScript into a standalone Node-shebang ESM file. That
artifact needs only Node at runtime, with no source tree, tsc, tsx or installed
dependencies. The build reads the existing manager's single version declaration
through `scripts/manager-metadata.mjs`, also used by npm package generation.
Help is extracted from the existing usage block to avoid a second command/help
inventory becoming a permanent implementation. Development version prints the
manager line only; component discovery belongs to command migration.

This entry currently implements help/version aliases and unknown/retired-command
errors. All other known commands, including internal hooks, fail explicitly with
`development handler is not implemented` and exit 1. Displayed usage describes
the released manager's full scope, not completed development handlers. No Bash
fallback silently provisions a profile. Later milestones inject real handlers
through `runCli`; the command boundary preserves argv tokens, cwd, caller
environment, binary streams and numeric exit without shell interpretation.
Full `npm test` runs the new typecheck/build/contracts on its second shard.

## Command inventory

`src/cli/contracts.ts` inventories every top-level dispatch token. It does not
parse arguments or change the current permissive/strict behavior of handlers.

| Command | Current argument contract |
| --- | --- |
| help, --help, -h, no command | Usage on stdout; exit 0. Trailing tokens currently ignored. |
| version, --version, -V | Manager first, then detected component versions; trailing tokens ignored. |
| agents | No args/help prints help; exactly `install` or `update` reconciles Agents/context-mode. |
| install | `--extra-skills`, help; maintains optional collection, Agents and Mobile. |
| update, update-restart | Zero/one component (`all`, `gestalt`, `serena`, `playwright`), positional or `--component NAME`; `--extra-skills` only for all/gestalt. Restart requires proven live Mobile ownership. |
| cli | Strip one leading `--`; preserve remaining Codex argv, native stdin and approvals/selectors. `resume` stays a Codex argument, not a manager command. |
| mobile | Strip one leading `--`; preserve remaining Mobile argv; reject `--xerj`/`--xerj=...` before setup. |
| scratch | help; `inspect`, `recover`, `inspect-tool-leases`, `recover-tool-leases`; `--cwd ROOT --owner ID`; recovery additionally requires matching `--expect TOKEN --confirm-quiescent`. |
| serena | help; `install`; `update [VERSION]`; `version`/`-V`/`--version`; `doctor [--cwd ROOT] [--json]`; `index --cwd ROOT`; `mcp --cwd ROOT`. MCP context/mode/project cannot be overridden. |
| playwright | help; `install [VERSION]`, `update [VERSION]`, `version [--json]`, `doctor [--json] [--launch] [--executable-path PATH]`, `mcp --cwd ROOT`; reject direct `__backend`. Embedded runtime owns detailed validation. |
| path | No args/text, `--json`, or one managed name: bin, org-plan, context-mode-cli, mobile, context-mode/context-mode-runtime, context-mode-plugin, gestalt-plugin, codex-home, gestalt-home. |
| skills | help; `install`, `update`, `status [--json]`; collection maintenance is separate from profile activation. |
| skills-export, skills-import | Export optional archive path; import requires archive; preserve profile and archive validation/transaction rules. |
| doctor | No args or `--cwd EXISTING_ROOT`; diagnostics can fail even when a launch may continue. |
| xerj | Always exit 1 with retirement/update guidance and retained-data notice; never resolve/install a runtime. `update xerj` and `--component xerj` also fail. |

Internal hooks are compatibility boundaries, not public features:
`__updated-manager` consumes a one-time leading continuation token before normal
dispatch; `__update-restart-worker` consumes the proven restart descriptor;
`__codex-app-server` owns an app-server lifecycle; `__cli-owned` resumes the
session owner; `__scratch-hook` forwards hook arguments to the scratch runtime.
The existing lifecycle, startup, scratch and retirement suites characterize
these contracts; do not turn them into generic public passthrough commands.

## Streams, processes and environment

Unknown commands and validation errors write `gestalt:` diagnostics to stderr
and exit 1. Help/path/machine JSON use stdout. MCP stdout contains protocol only;
logs and startup diagnostics stay on stderr. Child argv includes empty strings,
spaces, quotes, literal dollar signs and everything after `--` without shell
evaluation. Preserve binary stdin/stdout/stderr, child exit status and signal
delivery; distinguish spawn errors, signals and numeric exits. Existing launch
tests cover SIGTERM forwarding; a native Windows adapter must prove its own
termination behavior. PID alone never proves process ownership.

`HOME` is currently required; `CODEX_HOME` defaults to `~/.codex-gestalt` and
`GESTALT_HOME` to `~/.gestalt`. Neither may be HOME or `/`. The user collection
is `~/.agents/skills`, managed collection `$GESTALT_HOME/.agents/skills`, Codex
links `$CODEX_HOME/skills`, profiles `$GESTALT_HOME/skill-profiles`.
Preserve `PATH` precedence for managed bins and caller environment/approvals.
Public source overrides: `GESTALT_INSTALL_BASE_URL`, `GESTALT_MARKETPLACE`,
`GESTALT_MARKETPLACE_NAME`, `GESTALT_MOBILE_VERSION`, `GESTALT_SKILLS_SOURCE`,
`GESTALT_SKILLS_REF`. Preserve inherited `GESTALT_SESSION_TMP_CONTEXT`, `TMPDIR`,
`TMP`, `TEMP` as one validated owner context, never replace it with a new owner.

Manager/Mobile handshake metadata includes `GESTALT_MANAGER_BIN`,
`GESTALT_MANAGER_VERSION`, `GESTALT_AGENTS_VERSION`, `GESTALT_AGENTS_PLUGIN_ROOT`,
`GESTALT_CONTEXT_MODE_VERSION`, `GESTALT_CONTEXT_MODE_PLUGIN_ROOT`,
`GESTALT_CONTEXT_MODE_RUNTIME`, `GESTALT_MOBILE_BIN`, `GESTALT_MOBILE_PID`,
`GESTALT_MOBILE_RESTART_STATE`, `GESTALT_SERENA_VERSION`, `GESTALT_UV_VERSION`,
`GESTALT_SERENA_PYTHON_VERSION`, `GESTALT_STARTUP_DIAGNOSTICS_DONE`.
Helper-specific state remains private to its handler; never log the entire
environment, config, credentials or protocol payloads as diagnostics.

## Platform seams and invariants

The consumer-owned ports in `src/cli/contracts.ts` specify the migration boundary:

| Port | Required adapter evidence before cutover |
| --- | --- |
| CommandHandler/CommandStreams | Exact argv/cwd/env, binary streams, diagnostic separation and exit propagation. |
| PathPort | Native drive/UNC/case semantics; canonical containment; reject lexical escape and redirected write targets. |
| ExecutablePort | PATH/PATHEXT and npm shims, spaces/Unicode; no shell interpolation of caller tokens. |
| ProcessPort | Distinct spawn/exit/signal results, creation identity and executable checked before signalling; no orphan children. |
| PrivateStoragePort | User ownership, private Unix modes or Windows ACLs, no redirected writes/hardlinks; exclusive locks, staged durable atomic replacement. Reject unverifiable privacy. |
| ArchivePort | Validate every member before extraction; reject traversal, absolute paths, links and unexpected roots; preserve existing manifests/checksums and rollback. |
| IpcPort | Private Unix socket/Windows named pipe with owner verification and protocol-only streams; no implicit TCP fallback. |

Preserve user permission choices, profile settings and unrelated MCP servers.
Keep managed capability schema/contract versions coherent across Manager and
Agents; do not infer support from a tool catalog. Updates stage/verify/promote
and retain working installations on failure. Retirement removes only entries
with complete manager executable/argument ownership proof; ambiguous config,
old binaries, indexes, processes, interrupted stages and user data remain.

The CLI and Agents/context-mode are required. Serena/browser are optional,
installed only explicitly or updated if already present. Mobile host support
is independently optional: an explicit request must report a concrete missing
prerequisite, and unsupported Mobile hosting must not prevent CLI/Agents setup.
This is the target behavior; current `install` still includes Mobile and current
`doctor` checks it. The optional host change belongs to its later milestone.

## Evidence and target matrix

Node 24+ and compatible external Codex are prerequisites. No runtime TypeScript
compiler may be required by the final distribution. Current Bash helpers are
not evidence of portability; no OS restriction changes are authorized here.

| OS | x64 | ARM64 |
| --- | --- | --- |
| Linux | Current workspace regression execution only; packaging/argv/stdin/exit fixtures | Not executed here |
| macOS | Native execution pending | Native execution pending |
| Windows | Native standard-user PowerShell/cmd package/ACL/process/MCP execution pending | Native execution pending |

Linux tests and mocked platform values cannot establish native Windows/macOS
or architecture support. Native coverage must use the final unmodified package,
without Bash/WSL/Git Bash, administrator privileges or Developer Mode on Windows.
