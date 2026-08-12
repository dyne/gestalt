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

## When a native plan is enough

Use a lightweight native plan for short, low-risk work that does not need
durable review state or milestone commits. Org Plan is a governance boundary,
not a requirement for every task.
