---
status: completed
summary: 'Fixed unknown command fallthrough: ParseArgs now returns ''unknown'' for unrecognized commands, run() handles it with a helpful error before config loading, and tests cover the new behavior.'
container: dark-factory-096-fix-unknown-command-shows-help
dark-factory-version: v0.17.27
created: "2026-03-06T13:44:29Z"
queued: "2026-03-06T13:44:29Z"
started: "2026-03-06T13:44:29Z"
completed: "2026-03-06T13:53:09Z"
---
<objective>
Fix dark-factory to return an error on unknown commands instead of silently falling through to `run`. Currently `dark-factory prompts --help` starts the factory instead of showing an error.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read main.go — specifically parseArgs() and the default case in the command switch.
</context>

<requirements>
1. In `parseArgs()` in `main.go`, remove the fallthrough behavior for unknown commands:
   - Current: unknown command → return `"run"`, treat unknown as args
   - Fix: unknown command → return `"unknown"` with the unrecognized command name as args[0]

2. In the `run()` switch in `main.go`, change the `default` case:
   - Current: `return errors.Errorf(ctx, "unknown command: %s", command)`
   - This is already correct — but it is never reached because parseArgs converts unknowns to "run"
   - After fix to parseArgs, this default case will fire correctly

3. The error message should be helpful:
   ```
   unknown command: "prompts"
   Run 'dark-factory help' for usage.
   ```

4. Update tests in `main_test.go` (or wherever parseArgs is tested):
   - `dark-factory prompts` → returns error "unknown command: prompts"
   - `dark-factory foo` → returns error "unknown command: foo"
   - `dark-factory` (no args) → still runs (default to "run")
   - `dark-factory run` → still runs
   - `dark-factory --help` → still shows help
</requirements>

<constraints>
- Do NOT change behavior for known commands
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
