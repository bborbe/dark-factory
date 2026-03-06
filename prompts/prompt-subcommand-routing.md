<objective>
Add `dark-factory prompt <cmd>` two-level routing so existing commands are also accessible as `dark-factory prompt list`, `dark-factory prompt status`, etc. The flat commands (list, status, approve, queue, requeue, retry) remain as-is — this adds aliases under the `prompt` namespace without removing anything.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read main.go for the current parseArgs() and run() switch structure.
The existing flat commands (list, status, approve, queue, requeue, retry) must continue to work unchanged.
</context>

<requirements>
1. In `parseArgs()` in `main.go`, recognize "prompt" as a known top-level command.

2. In the `run()` switch in `main.go`, add case "prompt" that dispatches on args[0]:
   - "list" → CreateListCommand (same as flat "list")
   - "status" → CreateStatusCommand (same as flat "status")
   - "approve" → CreateApproveCommand (same as flat "approve")
   - "queue" → CreateQueueCommand (same as flat "queue")
   - "requeue" → CreateRequeueCommand (same as flat "requeue")
   - "retry" → CreateRequeueCommand with args ["--failed"] (same as flat "retry")
   - unknown subcmd → error "unknown prompt command: <subcmd>"
   - no subcmd (args empty) → error "usage: dark-factory prompt <list|status|approve|queue|requeue|retry>"

3. Update the help text in main.go to include:
   ```
     prompt   Manage prompts (list, status, approve, queue, requeue, retry)
   ```

4. Add tests to `main_test.go` (or create it if it doesn't exist) for the new routing:
   - "prompt list" dispatches to list command
   - "prompt status" dispatches to status command
   - "prompt unknown" returns error
   - "prompt" with no subcmd returns usage error
   - Existing flat "list", "status" commands still work
</requirements>

<constraints>
- Do NOT remove or change any existing flat commands
- Do NOT modify pkg/cmd or pkg/factory
- Do NOT commit — dark-factory handles git
- Keep parseArgs() and run() clean — extract helpers if needed
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
