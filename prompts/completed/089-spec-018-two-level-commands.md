---
status: completed
spec: [018-native-spec-integration]
summary: Refactored CLI to two-level subcommands (prompt <cmd>, spec <cmd>) with updated ParseArgs, nested switch routing, and comprehensive tests.
container: dark-factory-089-spec-019-two-level-commands
dark-factory-version: v0.17.15
created: "2026-03-06T10:57:15Z"
queued: "2026-03-06T10:57:15Z"
started: "2026-03-06T11:22:36Z"
completed: "2026-03-06T11:31:03Z"
---
<objective>
Refactor CLI command routing from flat commands to two-level subcommands: `prompt <cmd>` and `spec <cmd>`. Remove old flat commands. This restructures the entry point for spec 019.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read main.go for current command routing (parseArgs, switch statement).
Read pkg/cmd/ for existing command implementations.
Read pkg/factory/ for Create*Command factory functions.
</context>

<requirements>
1. Update `parseArgs()` in main.go to support two-level commands:
   - `dark-factory prompt list` → command="prompt", subcommand="list"
   - `dark-factory prompt status` → command="prompt", subcommand="status"
   - `dark-factory spec list` → command="spec", subcommand="list"
   - `dark-factory status` → command="status" (top-level combined)
   - `dark-factory list` → command="list" (top-level combined)
   - `dark-factory run` → command="run" (unchanged)
   - No args → command="run" (unchanged)
   - Return (debug bool, command string, subcommand string, args []string)

2. Update the switch statement in `run()`:
   - `case "prompt"`: nested switch on subcommand for list/status/approve/queue/requeue/retry
   - `case "spec"`: nested switch on subcommand for list/status/approve (wire to placeholder for now — just print "not implemented" for spec commands)
   - `case "status"`: placeholder for combined view (for now, call existing prompt status)
   - `case "list"`: placeholder for combined view (for now, call existing prompt list)
   - `case "run"`: unchanged
   - Remove old flat `case "approve"`, `case "queue"`, etc.

3. Update the help text to show the new command structure.

4. Update main_test.go or add tests for the new parseArgs behavior:
   - `["prompt", "list"]` → command="prompt", subcommand="list"
   - `["spec", "approve", "017"]` → command="spec", subcommand="approve", args=["017"]
   - `["status"]` → command="status", subcommand=""
   - `[]` → command="run"
   - `["-debug", "prompt", "list"]` → debug=true, command="prompt", subcommand="list"

5. The `retry` command becomes `prompt retry` (shorthand for `prompt requeue --failed`).
</requirements>

<constraints>
- All existing prompt commands must work under the `prompt` prefix
- `dark-factory run` and default (no args) behavior unchanged
- `make precommit` must pass
- Do NOT commit, tag, or push
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
