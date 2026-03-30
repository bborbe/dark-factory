---
status: draft
created: "2026-03-30T18:55:33Z"
---

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-factory-pattern.md` — factory wiring.
Read `/home/node/.claude/docs/go-testing.md` — Ginkgo/Gomega test conventions.
Read `/home/node/.claude/docs/go-error-wrapping.md` — error wrapping.

Key files to read before making changes:
- `main.go` — command routing, `runPromptCommand()`, `runSpecCommand()`
- `pkg/cmd/approve.go` — prompt approve (reverse this for unapprove)
- `pkg/cmd/spec_approve.go` — spec approve (reverse this for unapprove)
- `pkg/factory/factory.go` — factory wiring for commands
- `pkg/prompt/prompt.go` — PromptStatus constants, `MarkApproved()`
- `pkg/spec/spec.go` — Status constants, `SetStatus()`
</context>

<requirements>

## 1. Add `prompt unapprove` command

Create `pkg/cmd/unapprove.go`:
- Reverse of `approve.go`
- Finds prompt in queue dir (approved/queued status)
- Strips numeric prefix via `prompt.StripNumberPrefix()`
- Moves file back to inbox dir
- Sets status to `draft`
- Prints `unapproved: <filename>`
- Error if prompt is already executing, completed, or not found in queue

## 2. Add `spec unapprove` command

Create `pkg/cmd/spec_unapprove.go`:
- Reverse of `spec_approve.go`
- Finds spec in in-progress dir (approved/prompted status)
- Strips numeric prefix
- Moves file back to inbox dir (specs/)
- Sets status to `draft`
- Clears `approved` timestamp from frontmatter
- Clears `branch` from frontmatter
- Prints `unapproved: <filename>`
- Error if spec has linked prompts that are executing or completed

## 3. Wire commands

In `main.go`:
- Add `case "unapprove"` to `runPromptCommand()` → `factory.CreateUnapproveCommand(cfg).Run(ctx, args)`
- Add `case "unapprove"` to `runSpecCommand()` → `factory.CreateSpecUnapproveCommand(cfg).Run(ctx, args)`
- Update `printHelp()` — add `prompt unapprove <id>` and `spec unapprove <id>` lines

In `pkg/factory/factory.go`:
- Add `CreateUnapproveCommand(cfg) → cmd.NewUnapproveCommand(...)`
- Add `CreateSpecUnapproveCommand(cfg) → cmd.NewSpecUnapproveCommand(...)`

## 4. Tests

- `pkg/cmd/unapprove_test.go` — test unapprove from queue, error if not found, error if executing
- `pkg/cmd/spec_unapprove_test.go` — test unapprove from in-progress, error if not found, clears approved/branch fields
- Follow existing test patterns in `approve_test.go` and `spec_approve_test.go`

</requirements>

<verification>
make precommit
</verification>
