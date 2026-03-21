---
status: completed
summary: Removed prompts/ideas/ directory concept by dropping IdeasCount from Status struct, ideasDir from checker, and the Ideas display line from formatter, updating all callers and tests accordingly
container: dark-factory-211-remove-ideas-dir
dark-factory-version: v0.63.0
created: "2026-03-21T19:11:31Z"
queued: "2026-03-21T19:11:31Z"
started: "2026-03-21T19:11:33Z"
completed: "2026-03-21T19:19:25Z"
---

<summary>
- The separate `prompts/ideas/` directory is removed
- Prompts with `status: idea` live in the normal `prompts/` inbox alongside `status: draft`
- Status display no longer has a separate "Ideas" count — ideas are part of the inbox count
- One fewer directory to manage, simpler mental model
</summary>

<objective>
Remove the `prompts/ideas/` directory concept. Ideas are just prompts with `status: idea` in the normal `prompts/` inbox. The separate directory adds unnecessary complexity — status field already distinguishes idea from draft.
</objective>

<context>
- Read `CLAUDE.md` and `docs/dod.md` for project conventions
- Read `docs/prompt-writing.md` — defines prompt lifecycle, inbox is `prompts/`
- `pkg/factory/factory.go` defines `defaultIdeasDir = "prompts/ideas"` and passes it to status collector
- `pkg/status/status.go` has `ideasDir` field and `IdeasCount` in status struct
- `pkg/status/formatter.go` displays ideas count separately
</context>

<requirements>
1. Remove `defaultIdeasDir` constant from `pkg/factory/factory.go` and all references passing it to status constructors.

2. Remove `ideasDir` field from `pkg/status/status.go` `checker` struct. Remove `IdeasCount` from `Status` struct. Remove the ideas counting logic in `Get()` method.

3. Update `pkg/status/formatter.go` to remove the "Ideas" line from display output.

4. Update `pkg/server/server_test.go`, `pkg/status/status_test.go`, `pkg/status/formatter_test.go`, `pkg/cmd/status_test.go` to remove all `IdeasCount` and `ideasDir` references.

5. Update `NewChecker` constructor signature — remove `ideasDir` parameter. Update all callers in `pkg/factory/factory.go`.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass after removing ideas-related tests
- The `prompts/` inbox directory continues to work as before
- Prompts with `status: idea` still work — they just live in `prompts/` not `prompts/ideas/`
</constraints>

<verification>
make precommit
</verification>
