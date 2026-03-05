---
status: completed
summary: Implemented auto-setting status to queued when prompts are picked up from queue directory
container: dark-factory-071-auto-queue-status-on-pickup
dark-factory-version: v0.16.0
created: "2026-03-05T13:15:41Z"
queued: "2026-03-05T13:15:41Z"
started: "2026-03-05T13:15:41Z"
completed: "2026-03-05T13:23:39Z"
---
<objective>
Auto-set `status: queued` when dark-factory picks up a prompt from the queue directory, regardless of current status value.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read ALL markdown files in ~/Documents/workspaces/coding-guidelines/ for Go patterns.
Read pkg/processor/processor.go — `processExistingQueued` method, the `ValidateForExecution` call.
Read pkg/prompt/prompt.go — `ValidateForExecution`, `ListQueued`, `PrepareForExecution` methods.
</context>

<requirements>
1. In `processExistingQueued` (`pkg/processor/processor.go`):
   - Before calling `ValidateForExecution`, check if the prompt status is empty or `created`
   - If so, auto-set it to `queued` via `p.promptManager.SetStatus(ctx, pr.Path, "queued")` and log at Info level: `"auto-setting status to queued", "file", basename, "previousStatus", pr.Status`
   - This makes the folder location the source of truth — if a file is in `queue/`, it should be treated as queued

2. Do NOT change `ValidateForExecution` — it should still reject non-queued status as a safety check
3. Do NOT change `ListQueued` — it already correctly picks up files with any non-terminal status
</requirements>

<constraints>
- Only modify pkg/processor/processor.go
- Do not change any public API signatures
- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>

<success_criteria>
- Prompts with `status: created` or empty status auto-transition to `queued` when found in queue dir
- Log message at Info level shows the auto-transition
- `ValidateForExecution` still rejects truly invalid statuses (e.g. `completed`, `failed`)
- make precommit passes
</success_criteria>
