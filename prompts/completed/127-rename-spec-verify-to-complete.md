---
status: completed
summary: Renamed CLI command `dark-factory spec verify` to `dark-factory spec complete` for clarity, updated help text, output message, Go types, mock, factory wiring, and tests
created: "2026-03-07T18:48:00Z"
completed: "2026-03-07T19:10:00Z"
---
<summary>
- Renamed the CLI subcommand from "spec verify" to "spec complete"
- "verify" was misleading — sounded like it triggers verification, but it actually marks a spec as done
- All Go types renamed: SpecVerifyCommand → SpecCompleteCommand
- Files renamed: spec_verify.go → spec_complete.go, matching test and mock
- Output changed from "verified:" to "completed:"
- Help text updated
</summary>

<objective>
Rename `dark-factory spec verify` to `dark-factory spec complete` because "verify" implies triggering verification, while the command actually marks a verified spec as completed.
</objective>

<context>
Read CLAUDE.md for project conventions.
The command was originally added by prompt 113-spec-021-2-verify-command.md.
</context>

<requirements>
1. Rename CLI subcommand from `verify` to `complete` in `main.go`.
2. Rename Go types: `SpecVerifyCommand` → `SpecCompleteCommand`, `specVerifyCommand` → `specCompleteCommand`, `NewSpecVerifyCommand` → `NewSpecCompleteCommand`.
3. Rename files: `spec_verify.go` → `spec_complete.go`, `spec_verify_test.go` → `spec_complete_test.go`, `spec-verify-command.go` → `spec-complete-command.go`.
4. Update factory: `CreateSpecVerifyCommand` → `CreateSpecCompleteCommand`.
5. Update help text and output message.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
