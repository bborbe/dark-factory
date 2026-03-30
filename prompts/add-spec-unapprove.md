---
status: draft
created: "2026-03-30T18:55:33Z"
---

<summary>
- Users can undo spec approval, moving the spec back to inbox with draft status
- Unapprove is only allowed for specs with `approved` status
- Approval metadata (approved timestamp, branch) is cleared
- After removing a spec, higher-numbered approved specs are renumbered to close the gap
- Blocked if spec has linked prompts — unapprove prompts first
- Help text and CLI routing updated for the new subcommand
</summary>

<objective>
Add `dark-factory spec unapprove` command that reverses a spec approval — moving the file back to inbox, resetting status to draft, clearing approval metadata, and renumbering remaining approved specs to close the gap.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-factory-pattern.md` — factory wiring.
Read `/home/node/.claude/docs/go-testing.md` — Ginkgo/Gomega test conventions.
Read `/home/node/.claude/docs/go-error-wrapping.md` — error wrapping.

Key files to read before making changes:
- `main.go` — command routing, `runSpecCommand()`
- `pkg/cmd/spec_approve.go` — spec approve (reverse this for unapprove)
- `pkg/factory/factory.go` — factory wiring for commands
- `pkg/spec/spec.go` — Status constants, `SetStatus()`
</context>

<requirements>

## 1. Add `spec unapprove` command

Create `pkg/cmd/spec_unapprove.go`:
- Reverse of `spec_approve.go`
- Finds spec in in-progress dir — only `approved` status allowed (error if executing, prompted, completed, or not found)
- Strips numeric prefix
- Moves file back to inbox dir (specs/)
- Sets status to `draft`
- Sets `Approved` to zero value (`time.Time{}`) — `omitempty` YAML tag removes it from output
- Sets `Branch` to empty string — `omitempty` YAML tag removes it from output
- Renumbers all higher-numbered approved specs in in-progress to close the gap
- Prints `unapproved: <filename>`
- Error if any prompt in queue or in-progress dirs has `spec: ["NNN"]` matching this spec's number — unapprove/remove those prompts first. Scan prompt frontmatter `spec` field for matches.

## 2. Wire command

In `main.go`:
- Add `case "unapprove"` to `runSpecCommand()` → `factory.CreateSpecUnapproveCommand(cfg).Run(ctx, args)`
- Update `printHelp()` — add `spec unapprove <id>` line

In `pkg/factory/factory.go`:
- Add `CreateSpecUnapproveCommand(cfg) → cmd.NewSpecUnapproveCommand(...)`

## 3. Tests

- `pkg/cmd/spec_unapprove_test.go` — test unapprove from in-progress, renumbering of remaining specs, error if not found, error if has linked prompts, clears approved/branch fields
- Follow existing test patterns in `spec_approve_test.go`

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
