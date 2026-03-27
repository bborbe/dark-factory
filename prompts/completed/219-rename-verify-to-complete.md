---
status: completed
container: dark-factory-219-rename-verify-to-complete
dark-factory-version: v0.68.1-dirty
created: "2026-03-27T20:36:28Z"
queued: "2026-03-27T20:36:28Z"
started: "2026-03-27T20:36:38Z"
completed: "2026-03-27T21:00:44Z"
---

<summary>
- The `prompt verify` subcommand is renamed to `prompt complete`
- `prompt complete` accepts prompts in `pending_verification`, `failed`, `in_review`, or `executing` status
- Users can force-complete prompts that failed due to unrelated issues (e.g. pre-existing CVEs)
- The help text and usage errors reflect the new command name
- The CLI still triggers the same commit/push/PR workflow as before
- Backward compatibility: no alias for `verify` — clean rename
</summary>

<objective>
Rename `prompt verify` to `prompt complete` and widen the accepted status gate from only `pending_verification` to also include `failed`, `in_review`, and `executing`. This lets users accept prompts that failed due to unrelated blockers (e.g. pre-existing OSV vulnerabilities) without manually editing frontmatter.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Key files to read before making changes:
- `main.go` — `runPromptCommand` switch statement routes `"verify"` to `factory.CreatePromptVerifyCommand` (~line 107)
- `main.go` — help text string contains `"prompt verify <id>"` (~line 152)
- `pkg/cmd/prompt_verify.go` — `PromptVerifyCommand` interface + `promptVerifyCommand` struct + `Run` method with status check on line 79
- `pkg/cmd/prompt_verify_test.go` — tests including "not in pending verification" error assertions for `approved` and `failed` states
- `pkg/factory/factory.go` — `CreatePromptVerifyCommand` wires dependencies (~search for `NewPromptVerifyCommand`)
- `mocks/prompt-verify-command.go` — counterfeiter mock for `PromptVerifyCommand`
</context>

<requirements>
1. Rename `pkg/cmd/prompt_verify.go` → `pkg/cmd/prompt_complete.go`
2. Rename `pkg/cmd/prompt_verify_test.go` → `pkg/cmd/prompt_complete_test.go`
3. Rename interface `PromptVerifyCommand` → `PromptCompleteCommand` (all references)
4. Rename struct `promptVerifyCommand` → `promptCompleteCommand`
5. Rename constructor `NewPromptVerifyCommand` → `NewPromptCompleteCommand`
6. Rename factory method `CreatePromptVerifyCommand` → `CreatePromptCompleteCommand` in `pkg/factory/factory.go`
7. Update `main.go` switch case from `"verify"` to `"complete"` and update the factory call
8. Update help text in `main.go` from `"prompt verify <id>"` to `"prompt complete <id>"`
9. Update the status check in `Run` method: accept `pending_verification`, `failed`, `in_review`, and `executing` — reject all other states (approved, completed, idea, draft, cancelled)
10. Update usage error message from `"dark-factory prompt verify"` to `"dark-factory prompt complete"`
11. Update error message from `"prompt is not in pending verification state"` to `"prompt cannot be completed"` (or similar)
12. Update counterfeiter generate directive to produce `PromptCompleteCommand` mock
13. Regenerate mock: `go generate ./pkg/cmd/...`
14. Delete old mock file `mocks/prompt-verify-command.go` if the new one has a different name
15. Update tests:
    - `failed` state test: change from expecting error to expecting success (mock `MoveToCompleted`, `CommitCompletedFile`, etc.)
    - `approved` state test: keep as error case but update error message
    - Add test for `executing` state expecting success
    - Keep all `pending_verification` tests passing
16. Update `CHANGELOG.md` with entry under `## Unreleased`: `- feat: rename \`prompt verify\` to \`prompt complete\`, accept failed/completed prompts`
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- No new dependencies
- Keep the same commit/push/PR workflow logic — only the status gate and naming changes
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
