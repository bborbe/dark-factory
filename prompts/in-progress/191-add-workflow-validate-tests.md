---
status: approved
created: "2026-03-11T17:44:51Z"
queued: "2026-03-11T18:25:03Z"
---

<summary>
- The `Workflow.Validate` method has explicit test coverage for all branches
- The removed "worktree" workflow value is tested to produce the correct migration error message
- Unknown workflow values are tested to be rejected
- Valid workflow values ("direct", "pr") are tested to pass validation
- Empty string is tested to be rejected as unknown workflow
- `Workflows.Contains` is tested for both known and unknown workflow values
</summary>

<objective>
Add test coverage for the `Workflow.Validate` method in `pkg/config/workflow.go` which has branching logic (worktree guard, unknown value check) that is currently untested.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/config/workflow.go` — the `Validate` method has three paths: worktree (special removed error), unknown (validation error), and valid (nil).
Read `pkg/config/config_test.go` — add the new tests alongside existing config tests.
Follow the existing Ginkgo/Gomega test patterns in the file.
</context>

<requirements>
1. In `pkg/config/config_test.go`, add a new `Describe("Workflow")` block (or add to existing config tests) with the following test cases:

2. Use `DescribeTable` for the validation cases:
   ```go
   DescribeTable("Validate",
       func(w config.Workflow, expectErr bool, errSubstring string) {
           err := w.Validate(ctx)
           if expectErr {
               Expect(err).To(HaveOccurred())
               if errSubstring != "" {
                   Expect(err.Error()).To(ContainSubstring(errSubstring))
               }
           } else {
               Expect(err).NotTo(HaveOccurred())
           }
       },
       Entry("direct is valid", config.WorkflowDirect, false, ""),
       Entry("pr is valid", config.WorkflowPR, false, ""),
       Entry("worktree is removed", config.Workflow("worktree"), true, "removed"),
       Entry("unknown value is rejected", config.Workflow("invalid"), true, "unknown workflow"),
       Entry("empty string is rejected", config.Workflow(""), true, "unknown workflow"),
   )
   ```

3. Also add a test for `Workflows.Contains`:
   ```go
   Describe("Workflows.Contains", func() {
       It("returns true for known workflow", func() {
           Expect(config.AvailableWorkflows.Contains(config.WorkflowDirect)).To(BeTrue())
       })
       It("returns false for unknown workflow", func() {
           Expect(config.AvailableWorkflows.Contains(config.Workflow("unknown"))).To(BeFalse())
       })
   })
   ```
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Use Ginkgo/Gomega — follow the existing test patterns in `config_test.go`.
- Use the existing `ctx` variable from the test setup (check how other tests in the file obtain context).
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
