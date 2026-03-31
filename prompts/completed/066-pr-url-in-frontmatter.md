---
status: completed
spec: [014-pr-url-in-frontmatter]
summary: Added pr-url field to prompt frontmatter with amend/force-push support
container: dark-factory-066-pr-url-in-frontmatter
dark-factory-version: v0.15.1
created: "2026-03-04T21:36:04Z"
queued: "2026-03-04T21:36:04Z"
started: "2026-03-04T21:36:04Z"
completed: "2026-03-04T21:47:22Z"
---
<objective>
Store the PR URL in prompt frontmatter when using the PR workflow, so completed prompts link back to their pull request.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read ALL markdown files in ~/Documents/workspaces/coding/docs/ for Go patterns.
Spec: specs/015-pr-url-in-frontmatter.md
Key files: pkg/prompt/prompt.go (Frontmatter struct, PromptFile methods), pkg/processor/processor.go (handlePRWorkflow)
</context>

<requirements>
1. Add field to `Frontmatter` in `pkg/prompt/prompt.go`:
   ```go
   PRURL string `yaml:"pr-url,omitempty"`
   ```

2. Add method to `PromptFile` in `pkg/prompt/prompt.go`:
   ```go
   func (pf *PromptFile) SetPRURL(url string) {
       pf.Frontmatter.PRURL = url
   }
   ```

3. Update `handlePRWorkflow` in `pkg/processor/processor.go`:
   After `p.prCreator.Create()` succeeds and before `p.brancher.Switch()`:
   - Load the prompt file
   - Call `SetPRURL(prURL)`
   - Save the prompt file
   - Commit the updated frontmatter (amend the previous commit)

   ```go
   prURL, err := p.prCreator.Create(gitCtx, title, "Automated by dark-factory")
   if err != nil {
       return errors.Wrap(ctx, err, "create pull request")
   }
   slog.Info("created PR", "url", prURL)

   // Save PR URL to frontmatter
   if err := p.promptManager.SetPRURL(ctx, promptPath, prURL); err != nil {
       slog.Warn("failed to save PR URL to frontmatter", "error", err)
       // Non-fatal — PR was already created
   }
   ```

4. Add `SetPRURL` to `Manager` interface in `pkg/prompt/prompt.go`:
   ```go
   SetPRURL(ctx context.Context, path string, url string) error
   ```

5. Implement `SetPRURL` on `manager`:
   - Load prompt file
   - Set PRURL field
   - Save prompt file

6. Pass `promptPath` to `handlePRWorkflow` (currently not available — check how to thread it through).

7. Update tests in `pkg/prompt/prompt_test.go`:
   - `SetPRURL` sets the field correctly
   - Save/Load roundtrip preserves `pr-url`
   - Existing files without `pr-url` load without error

8. Regenerate counterfeiter mocks: `go generate ./...`
</requirements>

<constraints>
- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Backward compatible — existing frontmatter without pr-url must parse
- PR URL save failure is non-fatal (log warning, don't fail the workflow)
- Follow existing patterns (SetSummary, SetContainer, etc.)
</constraints>

<verification>
Run `go generate ./...` -- must succeed.
Run `make test` -- must pass.
Run `make precommit` -- must pass.
</verification>

<success_criteria>
- pr-url field appears in frontmatter of PR-workflow prompts
- Direct/worktree workflow prompts have no pr-url field
- Existing prompts without pr-url load without error
- make precommit passes
</success_criteria>
