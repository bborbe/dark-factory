---
status: draft
created: "2026-03-30T18:55:33Z"
---

<summary>
- Users can undo prompt approval, moving the prompt back to inbox with draft status
- Unapprove is only allowed for prompts with `approved` status
- After removing a prompt, higher-numbered approved prompts are renumbered to close the gap
- Help text and CLI routing updated for the new subcommand
- Tests cover unapprove, renumbering, and error cases
</summary>

<objective>
Add `dark-factory prompt unapprove` command that reverses a prompt approval — moving the file back to inbox, resetting status to draft, and renumbering remaining approved prompts to close the gap.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-factory-pattern.md` — factory wiring.
Read `/home/node/.claude/docs/go-testing.md` — Ginkgo/Gomega test conventions.
Read `/home/node/.claude/docs/go-error-wrapping.md` — error wrapping.

Key files to read before making changes:
- `main.go` — command routing, `runPromptCommand()`
- `pkg/cmd/approve.go` — prompt approve (reverse this for unapprove)
- `pkg/factory/factory.go` — factory wiring for commands
- `pkg/prompt/prompt.go` — PromptStatus constants, `MarkApproved()`, `StripNumberPrefix()`, `performRename()`
</context>

<requirements>

## 1. Add `prompt unapprove` command

Create `pkg/cmd/unapprove.go`:
- Reverse of `approve.go`
- Finds prompt in queue dir — only `approved` status allowed (error if executing, completed, or not found)
- Strips numeric prefix via `prompt.StripNumberPrefix()`
- Moves file back to inbox dir
- Sets status to `draft`
- Calls `NormalizeFilenames()` on the queue dir to renumber remaining prompts and close the gap (e.g., remove 010 → 011 becomes 010, 012 becomes 011) — same approach as `approve.go`
- Prints `unapproved: <filename>`

## 2. Wire command

In `main.go`:
- Add `case "unapprove"` to `runPromptCommand()` → `factory.CreateUnapproveCommand(cfg).Run(ctx, args)`
- Update `printHelp()` — add `prompt unapprove <id>` line

In `pkg/factory/factory.go`:
- Add `CreateUnapproveCommand(cfg) → cmd.NewUnapproveCommand(...)`

## 3. Tests

- `pkg/cmd/unapprove_test.go` — test unapprove from queue, renumbering of remaining prompts, error if not found, error if executing
- Follow existing test patterns in `approve_test.go`

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Use `github.com/bborbe/errors` for error wrapping
- New code must have >= 80% test coverage
- Existing tests must still pass
- Follow existing approve command patterns exactly (constructor returns interface, struct unexported)
</constraints>

<verification>
```bash
make precommit
```
</verification>
