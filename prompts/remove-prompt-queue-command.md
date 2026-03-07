---
status: ""
created: "2026-03-07T19:34:00Z"
---
<summary>
- Removes the `dark-factory prompt queue` command entirely
- `prompt approve` is the only way to move prompts from inbox to in-progress
- `prompt queue` was a duplicate of `prompt approve` — both did the same thing
- Removes the risk of accidentally queueing all inbox prompts (queue without args)
</summary>

<objective>
Remove the duplicate `prompt queue` command. It does the same thing as `prompt approve` and has a dangerous no-arg mode that silently queues all inbox prompts.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/cmd/queue.go` — the command to remove.
Read `pkg/cmd/queue_test.go` — tests to remove.
Read `pkg/cmd/approve.go` — the command that stays (already requires an arg).
Read `main.go` — routing and help text.
Read `pkg/factory/factory.go` — `CreateQueueCommand` factory function to remove.
Read `mocks/queue-command.go` — generated mock to remove.
</context>

<requirements>
1. Delete `pkg/cmd/queue.go`, `pkg/cmd/queue_test.go`, and `mocks/queue-command.go`.
2. Remove `CreateQueueCommand` from `pkg/factory/factory.go`.
3. Remove `case "queue"` from `runPromptCommand` in `main.go`.
4. Remove the `prompt queue` line from `printHelp` in `main.go`.
5. Remove any imports that become unused after these deletions.
</requirements>

<constraints>
- Do NOT modify `prompt approve`, `prompt requeue`, or `prompt retry`
- Do NOT change any prompt statuses — that is a separate prompt
- Do NOT touch `pkg/server/queue_helpers.go` — the REST API uses its own queue logic
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
