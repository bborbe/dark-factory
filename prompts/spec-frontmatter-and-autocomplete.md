<objective>
Add an optional `spec` field to prompt frontmatter so prompts can link to their parent spec. When all prompts linked to a spec are completed, automatically mark the spec as `status: completed`. This closes the loop between prompt execution and spec lifecycle.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/prompt/prompt.go for the Frontmatter struct and existing field patterns.
Read pkg/spec/manager.go for the Manager interface and Approve pattern.
Read pkg/processor/processor.go — specifically the section that moves a prompt to completed, to find where to add the auto-complete check.
Read pkg/factory/factory.go for dependency injection patterns.
</context>

<requirements>
1. Add `Spec string` field to `Frontmatter` struct in `pkg/prompt/prompt.go` (yaml: "spec,omitempty").
   Add `Spec() string` getter on PromptFile.

2. Create `pkg/spec/autocomplete.go` with:
   - `AutoCompleter` interface (with counterfeiter directive):
     ```go
     CheckAndComplete(ctx context.Context, specID string) error
     ```
   - `NewAutoCompleter(specsDir string, promptDirs []string) AutoCompleter` constructor
   - `CheckAndComplete` implementation:
     - Load all specs from specsDir, find the spec whose filename contains specID (e.g. "017")
     - If not found → log Debug "spec not found, skipping", return nil
     - If spec status is already "completed" → return nil
     - Scan all promptDirs (inbox, queue, completed) for prompts with `spec == specID`
     - If ALL found prompts have status "completed" AND at least one prompt was found → call Manager.Approve equivalent to set status "completed" on the spec file
     - Otherwise → return nil (not all done yet)

3. In `pkg/processor/processor.go`, after a prompt is successfully moved to completedDir:
   - Read the `Spec()` field from the prompt frontmatter
   - If non-empty, call `p.autoCompleter.CheckAndComplete(ctx, spec)`
   - This is best-effort: log warning on error, do not fail the prompt

4. Add `autoCompleter spec.AutoCompleter` field to the `processor` struct.
   Update `NewProcessor` to accept and inject it.
   Update `pkg/factory/factory.go` to construct and inject `AutoCompleter`.

5. Add tests:
   - `pkg/spec/autocomplete_test.go`: CheckAndComplete marks spec completed when all linked prompts are done; skips if any prompt not completed; skips if spec not found
   - `pkg/processor/processor_test.go`: after successful prompt completion with `spec` field set, AutoCompleter.CheckAndComplete is called with correct specID; non-empty spec with error from AutoCompleter → warning logged, prompt not failed

6. Regenerate mocks with `go generate ./...`
</requirements>

<constraints>
- AutoCompleter failure is non-fatal — log warning, never fail the prompt
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Coverage ≥ 80% for pkg/spec new files
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
