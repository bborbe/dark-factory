<objective>
Verify that dark-factory correctly handles a failed completion report.
This prompt is INTENTIONALLY IMPOSSIBLE and must report failure.
</objective>

<context>
Read CLAUDE.md for project conventions.
</context>

<requirements>
1. Add a new exported function `ImpossibleFunction` to `pkg/prompt/prompt.go` that returns a value of type `nonexistent.Type` from a package that does not exist
2. Run `make test` — it MUST pass (it won't, because the type doesn't exist)
</requirements>

<constraints>
- You MUST attempt the implementation exactly as described
- Do NOT skip or work around the impossible requirement
- Do NOT invent alternative solutions
</constraints>

<verification>
Run: `make test`
</verification>

<success_criteria>
- `ImpossibleFunction` compiles and tests pass (this is impossible by design)
</success_criteria>
