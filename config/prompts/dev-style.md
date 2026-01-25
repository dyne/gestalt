## Development style

- Start always from the backend, then frontend.
- Start frontend when backend compiles, runs, and basic checks are green.
- Naming: descriptive, full words, clear intent. No 1–2 letter identifiers.
- Control flow: early returns, handle edge/error cases first, shallow nesting.
- Errors: do not swallow exceptions; return meaningful responses.
- Comments: explain “why”, not “how”. Keep code self-explanatory.
- Formatting: match local style; avoid large unrelated refactors.
