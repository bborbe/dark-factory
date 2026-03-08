---
status: executing
spec: ["027"]
container: dark-factory-139-spec-027-config-validation
dark-factory-version: v0.26.0
created: "2026-03-08T21:00:00Z"
queued: "2026-03-08T21:06:34Z"
started: "2026-03-08T21:06:37Z"
---

<summary>
- Config validation now rejects `workflow: worktree` at startup with a clear migration message
- `workflow: pr` becomes the only PR-based workflow value — `workflow: worktree` is a hard error
- `autoMerge` validation no longer references `workflow: worktree` — only `workflow: pr` satisfies it
- `autoReview` validation likewise simplified to require only `workflow: pr`
- The removed workflow value is kept internally only to generate a helpful migration error message
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
   a. `autoMerge` validator — simplify to only allow `WorkflowPR` (in `pkg/config/config.go` lines 131–136):

      Before:
      ```go
      validation.Name("autoMerge", validation.HasValidationFunc(func(ctx context.Context) error {
          if c.AutoMerge && c.Workflow != WorkflowPR && c.Workflow != WorkflowWorktree {
              return errors.Errorf(ctx, "autoMerge requires workflow 'pr' or 'worktree'")
          }
          return nil
      })),
      ```

      After:
      ```go
      validation.Name("autoMerge", validation.HasValidationFunc(func(ctx context.Context) error {
          if c.AutoMerge && c.Workflow != WorkflowPR {
              return errors.Errorf(ctx, "autoMerge requires workflow 'pr'")
          }
          return nil
      })),
      ```
   b. `validateAutoReview` — simplify to only allow `WorkflowPR` (in `pkg/config/config.go` lines 155–157):

      Before:
      ```go
      if c.Workflow != WorkflowPR && c.Workflow != WorkflowWorktree {
          return errors.Errorf(ctx, "autoReview requires workflow 'pr' or 'worktree'")
      }
      ```

      After:
      ```go
      if c.Workflow != WorkflowPR {
          return errors.Errorf(ctx, "autoReview requires workflow 'pr'")
      }
      ```

3. Update `pkg/config/config_test.go`:
   - **Line 89–105** (`Config.Validate` / "succeeds for worktree workflow"): this test sets `Workflow: WorkflowWorktree` and expects no error — change it to assert that an error IS returned containing "removed". This is the most important test to fix.
   - **Lines 390–407** (`autoMerge` / "succeeds for autoMerge true with workflow worktree"): sets `Workflow: WorkflowWorktree, AutoMerge: true` and expects no error — convert to an error assertion (the autoMerge validator no longer accepts worktree, and worktree itself will be rejected first by `Workflow.Validate`).
   - **Lines 575–578** (`Workflow.Validate` / "succeeds for worktree workflow"): `WorkflowWorktree.Validate(ctx)` currently expects no error — change to assert error containing "removed".
   - **Lines 606–610** (`Workflows.Contains` / "returns true for valid workflow"): currently asserts `AvailableWorkflows.Contains(WorkflowWorktree)` is `true` — change this assertion to `BeFalse()` or remove it (after removing `WorkflowWorktree` from `AvailableWorkflows`).
   - **Lines 472–494** (`autoReview` / "fails for autoReview true with workflow direct"): the error string assertion at line 493 reads `ContainSubstring("autoReview requires workflow 'pr' or 'worktree'")` — update to `ContainSubstring("autoReview requires workflow 'pr'")`.
   - Add a new test asserting `WorkflowWorktree.Validate(ctx)` returns an error containing `"removed"`.
   - Add a new test asserting `autoMerge: true` with `workflow: worktree` returns a validation error.
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
