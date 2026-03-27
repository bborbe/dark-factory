---
status: failed
container: dark-factory-216-wrap-bare-return-err-spec-show
dark-factory-version: v0.67.8
created: "2026-03-27T14:23:18Z"
queued: "2026-03-27T14:23:18Z"
started: "2026-03-27T14:29:10Z"
completed: "2026-03-27T14:35:51Z"
---

<summary>
- Error from spec file lookup in spec-show command now includes context about which operation failed
- Consistent error wrapping pattern across the entire spec-show command handler
</summary>

<objective>
All error returns in the spec-show command use contextual wrapping for consistent diagnostics.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Reference file: `pkg/cmd/spec_show.go` — the `FindSpecFileInDirs` call returns a bare `return err` while all other error returns in this file use `errors.Wrap`.
</context>

<requirements>
1. In `pkg/cmd/spec_show.go`, wrap the bare `return err` after the `FindSpecFileInDirs` call with `errors.Wrap(ctx, err, "find spec file")`. The `errors` package is already imported.
</requirements>

<constraints>
- Only change the single bare `return err` after `FindSpecFileInDirs`
- Do not modify any other files
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
```bash
make precommit
```
</verification>
