---
status: approved
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T14:51:20Z"
---

<summary>
- Replaced fmt.Errorf with errors.Errorf across 6 files
- Added context-wrapped returns (errors.Wrap) for bare return err across 14 files
- 19 error handling violations fixed total
</summary>

<objective>
Fix fmt.Errorf usage and bare return err statements across multiple pkg/ files.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-error-wrapping-guide.md` for error handling patterns.

Files to read before making changes (all need fmt.Errorf → errors.Errorf):
- `pkg/prompt/prompt.go` — line 145 (CanTransitionTo fmt.Errorf), line 236 (UnmarshalYAML fmt.Errorf)
- `pkg/cmd/reject.go` — lines 111, 120 (fmt.Errorf in flag parsing)
- `pkg/spec/spec.go` — line 109 (CanTransitionTo fmt.Errorf)

Files with bare return err (need errors.Wrap):
- `pkg/processor/workflow_executor_worktree.go` — lines 40, 75
- `pkg/processor/workflow_executor_branch.go` — lines 40, 79
- `pkg/processor/workflow_executor_clone.go` — lines 40, 75
- `pkg/server/queue_action_handler.go` — line 154 (handleQueueError needs ctx added)
- `pkg/cmd/scenario_show.go` — line 57
- `pkg/cmd/prompt_complete.go` — lines 67, 108
- `pkg/cmd/spec_approve.go` — line 58
- `pkg/cmd/spec_complete.go` — line 60
- `pkg/cmd/spec_unapprove.go` — lines 66, 84
- `pkg/cmd/kill.go` — line 44
</context>

<requirements>
1. In each file with fmt.Errorf, replace with errors.Errorf(ctx, ...) — may need to add ctx parameter to the function

2. In each file with bare return err, replace with errors.Wrap(ctx, err, "descriptive message")

3. For functions that need ctx added (CanTransitionTo, parseReasonFlag, handleQueueError), add ctx as first parameter and update all callers.

4. In `pkg/runner/worktree.go` line 33: change errors.Wrapf to errors.Wrap (no format verbs).
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
make precommit
</verification>
