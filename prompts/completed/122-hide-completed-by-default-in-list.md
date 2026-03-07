---
status: completed
container: dark-factory-122-hide-completed-by-default-in-list
dark-factory-version: v0.20.6
created: "2026-03-07T11:00:00Z"
queued: "2026-03-07T10:34:18Z"
started: "2026-03-07T10:35:17Z"
completed: "2026-03-07T10:52:27Z"
---
<summary>
- `prompt list`, `spec list`, and `list` hide completed items by default
- Add `--all` flag to show everything including completed
- Active items (non-completed) always shown without any flag
- Behaviour mirrors `gh pr list` (open by default, `--all` for everything)
- Tests updated to cover default and `--all` behaviour
</summary>

<objective>
Hide completed prompts and specs from list commands by default. The default view shows only active items. Pass `--all` to include completed.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` and `go-testing.md` for patterns.
Find the list command implementations — search for `prompt list`, `spec list`, and the top-level `list` command in `pkg/cmd/`.
</context>

<requirements>
1. Add `--all` flag to `dark-factory prompt list`:
   - Default: hide prompts with `status: completed`
   - `--all`: show all prompts including completed

2. Add `--all` flag to `dark-factory spec list`:
   - Default: hide specs with `status: completed`
   - `--all`: show all specs including completed

3. Add `--all` flag to `dark-factory list` (top-level):
   - Passes `--all` behaviour through to both prompt and spec listing

4. Update or add tests covering:
   - Default output excludes completed items
   - `--all` output includes completed items

<constraints>
- Do NOT change status values or frontmatter fields
- Do NOT commit — dark-factory handles git
- `make precommit` must pass
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
Run `go test ./pkg/cmd/... -v` — all tests pass.
</verification>
