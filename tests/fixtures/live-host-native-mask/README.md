Native mask-composition candidate

Authorized L9.2 prerequisite for the observed `.codex` directory/file mask collision. `native.patch` is the complete two-file Apache-2.0 patch against the exact upstream commit/tree in `manifest.json`; `fix.patch` isolates the 19 added production lines and one changed line so CI can reproduce the old regression with the new tests. Existing Native permissions and directory cleanup ownership remain unchanged. Candidate build pins must be verified before Manager proof adopts them. No installed runtime or public release is changed.

Regressions create explicitly labeled temporary existing/replacement fixtures. They do not seed the original owning-runtime admission proof or provide provenance evidence for it.
