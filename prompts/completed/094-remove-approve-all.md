---
status: completed
spec: [018-native-spec-integration]
summary: Removed approveAll from approve.go so no-args returns a usage error, and updated tests accordingly; spec_approve.go already required an argument.
container: dark-factory-094-remove-approve-all
dark-factory-version: v0.17.15
created: "2026-03-06T10:59:53Z"
queued: "2026-03-06T10:59:53Z"
started: "2026-03-06T12:05:23Z"
completed: "2026-03-06T12:10:05Z"
---
<objective>
Remove the approve-all behavior from the approve command. Require an explicit file argument for both prompt and spec approve — approving all at once is too dangerous.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/cmd/approve.go for the current prompt approve implementation (approveAll method).
Read pkg/cmd/approve_test.go for existing tests.
Read pkg/cmd/spec_approve.go if it exists for spec approve implementation.
</context>

<requirements>
1. In `pkg/cmd/approve.go`, change `Run()`:
   - If no args provided, return an error: `"usage: dark-factory approve <file>"`
   - Remove the `approveAll` method entirely

2. If `pkg/cmd/spec_approve.go` exists, verify it also requires an argument (no approve-all).

3. Update tests:
   - Remove tests for approve-all behavior
   - Add test: no args → returns error with usage message
</requirements>

<constraints>
- `approve <NNN>` and `approve <filename>` must still work unchanged
- Applies to both prompt approve and spec approve
- `make precommit` must pass
- Do NOT commit, tag, or push
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
