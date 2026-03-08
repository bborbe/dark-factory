---
spec: ["027"]
status: created
created: "2026-03-08T21:00:00Z"
---

<summary>
- Config validation now rejects `workflow: worktree` at startup with a clear migration message
- `workflow: pr` becomes the only PR-based workflow value — `workflow: worktree` is a hard error
- `autoMerge` validation no longer references `workflow: worktree` — only `workflow: pr` satisfies it
- `autoReview` validation likewise simplified to require only `workflow: pr`
- `WorkflowWorktree` constant is preserved solely to produce a helpful "removed — use 'pr'" error message
- All existing tests updated to reflect the new validation rules
</summary>

<objective>
Remove `workflow: worktree` as a valid configuration value. Any project that still has `workflow: worktree` in its config should get a clear startup error: `"workflow 'worktree' removed — use 'pr' instead"`. This is the first step toward merging the old separate `pr` and `worktree` code paths into a single unified `pr` workflow.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read these files before making changes:
- `pkg/config/workflow.go` — Workflow enum, AvailableWorkflows, Validate
- `pkg/config/config.go` — Config.Validate(), autoMerge and autoReview validators
- `pkg/config/config_test.go` — existing workflow tests (search for WorkflowWorktree)
</context>

<requirements>
1. In `pkg/config/workflow.go`:
   a. Remove `WorkflowWorktree` from `AvailableWorkflows`:
      ```go
      var AvailableWorkflows = Workflows{WorkflowDirect, WorkflowPR}
      ```
   b. Update `Workflow.Validate()` to return a specific, migration-friendly error when the value is `"worktree"`:
      ```go
      func (w Workflow) Validate(ctx context.Context) error {
          if w == WorkflowWorktree {
              return errors.Wrapf(ctx, validation.Error,
                  "workflow 'worktree' removed — use 'pr' instead")
          }
          if !AvailableWorkflows.Contains(w) {
              return errors.Wrapf(ctx, validation.Error, "unknown workflow '%s'", w)
          }
          return nil
      }
      ```
   c. Keep the `WorkflowWorktree` constant — it is still referenced for the error message above and will be removed in a later prompt once the processor no longer uses it.

2. In `pkg/config/config.go`, update the two validators that reference `WorkflowWorktree`:
   a. `autoMerge` validator — simplify to only allow `WorkflowPR`:
      ```go
      if c.AutoMerge && c.Workflow != WorkflowPR {
          return errors.Errorf(ctx, "autoMerge requires workflow 'pr'")
      }
      ```
   b. `validateAutoReview` — simplify to only allow `WorkflowPR`:
      ```go
      if c.Workflow != WorkflowPR {
          return errors.Errorf(ctx, "autoReview requires workflow 'pr'")
      }
      ```

3. Update `pkg/config/config_test.go`:
   - Remove or update tests that assert `WorkflowWorktree` passes validation
     (lines ~576, ~591, ~609 — the "validates worktree workflow" tests)
   - Add a test asserting `WorkflowWorktree` now returns a validation error containing "removed"
   - Update the `autoMerge` test at line ~91 that uses `WorkflowWorktree` — change to `WorkflowPR`
   - Update the `autoReview` test at line ~392 that uses `WorkflowWorktree` — change to `WorkflowPR`
   - Add a test that `autoMerge` with `workflow: worktree` returns an error
</requirements>

<constraints>
- `workflow: direct` must not change
- `WorkflowWorktree` constant must stay (used in the new error message and still used in processor.go which is changed in the next prompt)
- Do NOT change anything in `pkg/processor/` — that is the next prompt's responsibility
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
