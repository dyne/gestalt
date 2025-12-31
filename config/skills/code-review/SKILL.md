---
name: code-review
description: Practical code review checklist and risk assessment prompts.
license: MIT
compatibility: ">=1.0"
metadata:
  owner: dyne
allowed_tools:
  - bash
---

# Code Review

Use this checklist to review changes consistently and flag risks.

## Quick scan
- Identify the purpose of the change in one sentence.
- List the files touched and the expected surface area.
- Note any public API or behavior changes.

## Deep review
1. Verify correctness for normal and edge-case inputs.
2. Check error handling and rollback paths.
3. Look for hidden coupling or assumptions.

## Tests
- Confirm that new behavior is covered by tests.
- Ensure existing tests still pass and remain relevant.
- Suggest minimal test additions when coverage is thin.

## Release readiness
- Call out backward compatibility risks explicitly.
- Summarize potential monitoring or rollback steps.
