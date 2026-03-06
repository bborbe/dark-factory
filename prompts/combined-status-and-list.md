<objective>
Update the top-level `dark-factory status` and `dark-factory list` commands to include spec output below prompt output. This gives a single combined view of all work in progress.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/cmd/status.go and pkg/cmd/list.go for the existing implementations.
Read pkg/cmd/spec_commands.go for SpecStatusCommand and SpecListCommand (just added).
Read pkg/factory/factory.go for how commands are wired.
Read main.go for how status and list are dispatched.
</context>

<requirements>
1. Update `pkg/cmd/status.go`:
   - Add `specStatus SpecStatusCommand` field to `statusCommand` struct
   - Update `NewStatusCommand` to accept `specStatus SpecStatusCommand` as a second parameter
   - In `Run`, after outputting prompt status, call `specStatus.Run(ctx, args)` to append spec status below
   - Separate the two sections with a blank line
   - Human output format:
     ```
     Prompts:
     <existing prompt status output>

     Specs:
     <spec status output>
     ```
   - JSON output: add `"specs": [...]` field to the existing JSON structure

2. Update `pkg/cmd/list.go`:
   - Add `specList SpecListCommand` field to `listCommand` struct
   - Update `NewListCommand` to accept `specList SpecListCommand` as a last parameter
   - In `Run`, after outputting prompt entries, call `specList.Run(ctx, args)` to append spec list below
   - Human output format:
     ```
     Prompts:
     <existing prompt list output>

     Specs:
     <spec list output>
     ```

3. Update `pkg/factory/factory.go`:
   - Update `CreateStatusCommand` to construct and inject `SpecStatusCommand`
   - Update `CreateListCommand` to construct and inject `SpecListCommand`

4. Update tests in `pkg/cmd/status_test.go` and `pkg/cmd/list_test.go`:
   - Combined output includes both prompt and spec sections
   - Spec section is skipped gracefully if spec command returns error (non-fatal — log warning, continue)

5. Regenerate mocks with `go generate ./...` (NewStatusCommand and NewListCommand signatures changed)
</requirements>

<constraints>
- Do NOT break existing status/list behavior — prompt output must appear first, unchanged
- Spec section failure is non-fatal (log warning, do not return error)
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
