# Supervised Org Plan workflow

Use an Org Plan when work benefits from ordered milestones, explicit skill
selection, current evidence, incremental review, and a conventional commit per
accepted milestone.

## Plan shape

Top-level headings are L1 milestones. Each L1 contains smaller L2 tasks and has
exactly one skill assignment and a durable review status. The active executor
works only inside the current milestone.

```text
L1 — User-visible outcome
├── L2 — Implement bounded behavior
├── L2 — Test the observable contract
└── L2 — Produce review evidence
```

## Lifecycle

1. The director creates or opens the plan and projects its ordered state.
2. One fresh executor loads context mode plus the L1's declared skills.
3. The executor implements and tests, keeping large output outside chat.
4. The director independently inspects the result and evidence.
5. Rejected work returns to the same executor; accepted work gets one commit.
6. A fresh executor starts the next L1.
7. Final acceptance requires a current full-suite pass and intended scope.

## What you see

Updates are deliberately short, for example:

```text
L1 2/5 — Validate release metadata: in review
```

You are asked for input only when a material requirement is ambiguous or a
prerequisite is unavailable—not for routine internal review decisions.

## Continuity and names

`supervision-start` activates Mobile supervision when the compatible controller
is available. An incomplete plan never ends merely because an executor reports,
becomes idle, errors, is interrupted, or a checkpoint is accepted: the root
continues work, follows up the same executor, reviews/corrects, starts the next
L1, or uses one of the explicit wait, attention, or manual-Off dispositions.

The agent list uses exact positional identities: the root is `l0`, and the
executor for L1 position `a` is `l<a>`. The current milestone title is separate display metadata.
Roles, nicknames, the plan title, and generated labels never replace or extend
those names. An interrupted or older physical replacement retains the same
positional identity and remains visible with its lifecycle state.

## Model-policy capability

A version 1 model-policy plan declares its version and explicit model, reasoning
effort, rationale, fallback IDs, and escalation trigger on each L1. The owning
Agents helper validates that schema; legacy plans remain legacy and are not
silently migrated. The v1 model pool is `gpt-6-luna`, `gpt-6-sol`, and
`gpt-6.1-sol`, with medium effort as the authoring default. Higher effort requires
recorded correctness or reasoning evidence and a return-to-medium condition.
Schema acceptance does not establish runtime model availability.

Mobile retains this policy and diagnoses configured settings. The manager's
[capability check](../reference/supervision-capabilities.md#l1-model-policy-agreement)
reports package agreement with `runtimeEnforcement=false`. Strict runtime
attestation and native child enforcement remain deferred; automatic launches
requiring that enforcement remain unsupported. Package presence, a valid plan,
or a setup provenance record does not authorize such a launch.

Explicit install/update regenerates the two Agents-managed role files. Manual
customizations inside generated role files are not preserved by setup; preserving
them is deferred. User configuration and unmanaged profiles remain intact, and
ordinary launches preserve all installed profiles. See
[setup ownership](../reference/supervision-capabilities.md#setup-ownership-and-provenance).

## When a native plan is enough

Use a lightweight native plan for short, low-risk work that does not need
durable review state or milestone commits. Org Plan is a governance boundary,
not a requirement for every task.
