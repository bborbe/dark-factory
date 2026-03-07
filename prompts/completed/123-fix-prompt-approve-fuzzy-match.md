---
status: completed
summary: Added FindPromptFile to pkg/cmd supporting fuzzy matching (exact, no-extension, numeric prefix) and updated prompt approve, requeue, and queue commands to use it
container: dark-factory-123-fix-prompt-approve-fuzzy-match
dark-factory-version: v0.21.1
created: "2026-03-07T13:05:00Z"
queued: "2026-03-07T12:45:06Z"
started: "2026-03-07T12:46:30Z"
completed: "2026-03-07T12:55:17Z"
---
<summary>
- `prompt approve`, `prompt requeue`, and `prompt queue` accept ID without `.md` extension
- Also accepts numeric prefix alone (e.g. `122`) to match `122-some-name.md`
- Same fuzzy matching already implemented in `FindSpecFile` — apply same logic to prompt commands
- No change to behaviour when exact filename is given
- Tests cover: exact filename, no extension, numeric prefix, not found
</summary>

<objective>
Make prompt commands accept a filename without the `.md` extension or with just a numeric prefix, mirroring how `FindSpecFile` works for specs. Users should be able to type `dark-factory prompt approve update-go-version` instead of `update-go-version.md`.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` and `go-testing.md` for patterns.
Read `pkg/cmd/spec_finder.go` — `FindSpecFile` already implements fuzzy matching (exact path, filename with/without `.md`, numeric prefix). Apply the same pattern to prompt finding.
Find the prompt approve, requeue, and queue command implementations in `pkg/cmd/` — locate where they resolve the prompt filename from the user-provided ID.
</context>

<requirements>
1. Create or extend a `FindPromptFile(ctx context.Context, inboxDir string, id string) (string, error)` function in `pkg/cmd/` that mirrors `FindSpecFile`:
   - If `id` is an absolute path or contains `/` — check directly
   - If `id` matches a file in `inboxDir` exactly — return it
   - If `id` + `.md` matches a file in `inboxDir` — return it
   - If a file in `inboxDir` has `id` as a numeric prefix (e.g. `122-` prefix) — return it
   - Otherwise — return error "file not found: <id>"

2. Update `prompt approve`, `prompt requeue`, and `prompt queue` commands to use `FindPromptFile` instead of direct filename lookup.

3. Add tests covering:
   - Exact filename with `.md` → found
   - Filename without `.md` → found
   - Numeric prefix only → found
   - No match → error

<constraints>
- Do NOT change the file format or frontmatter
- Do NOT commit — dark-factory handles git
- `make precommit` must pass
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
Run `go test ./pkg/cmd/... -v` — all tests pass.
</verification>
