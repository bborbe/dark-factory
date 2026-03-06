---
spec: ["017"]
status: completed
summary: 'Wired branch frontmatter field into processor: setupWorkflow now checks Branch() and uses FetchAndVerifyBranch+Switch for existing branches, savePRURLToFrontmatter preserves existing pr-url, refactored into helper methods to satisfy nestif linter, added 3 new tests.'
container: dark-factory-086-wire-existing-branch-into-processor
dark-factory-version: v0.17.12
created: "2026-03-06T10:11:01Z"
queued: "2026-03-06T10:11:01Z"
started: "2026-03-06T10:11:01Z"
completed: "2026-03-06T10:19:14Z"
---
<objective>
Wire the `branch` frontmatter field (added in previous prompt) into the processor so that prompts with `branch` set run on an existing branch instead of creating a new one. Also preserve `pr-url` when already set. This completes spec 017 (continue on existing branch).
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/processor/processor.go — focus on setupWorkflow (line ~426) and savePRURLToFrontmatter (line ~768).
Read pkg/prompt/prompt.go for the Branch() getter added in a previous prompt.
Read pkg/processor/processor_test.go for existing test patterns.
The processor receives the prompt file path and reads frontmatter via p.promptManager.
</context>

<requirements>
1. In `setupWorkflow` in `pkg/processor/processor.go`:
   - Before creating a new branch, read the prompt frontmatter to check if `Branch()` is set.
   - To read the prompt, use `p.promptManager` — look at how it's already used elsewhere in the processor for reading frontmatter.
   - If `Branch()` is non-empty (for `WorkflowPR`):
     - Store originalBranch as current branch (same as today)
     - Call `p.brancher.FetchAndVerifyBranch(ctx, branch)` — if it fails, return the error (prompt will be marked failed)
     - Call `p.brancher.Switch(ctx, branch)` to check out the existing branch
     - Set `state.branchName = branch`
     - Do NOT call `CreateAndSwitch`
   - If `Branch()` is empty, behave exactly as today (CreateAndSwitch with generated name).
   - Apply the same logic for `WorkflowWorktree`: if `Branch()` is set, use that branch name instead of generating one.

2. In `savePRURLToFrontmatter` in `pkg/processor/processor.go`:
   - Before calling `p.promptManager.SetPRURL`, read the existing `PRURL()` from the prompt file.
   - If `PRURL()` is already non-empty, skip the SetPRURL call, log at Debug level: `"pr-url already set, preserving existing value"`.
   - This prevents overwriting the PR URL on follow-up prompts that run on an existing branch.

3. `setupWorkflow` needs access to the prompt path to read frontmatter. Look at how `processPrompt` passes the path to other methods and thread it through.

4. Add tests to `pkg/processor/processor_test.go`:
   - WorkflowPR with `branch` set in frontmatter: `FetchAndVerifyBranch` called, `Switch` called, `CreateAndSwitch` NOT called
   - WorkflowPR with `branch` set and `FetchAndVerifyBranch` returns error: prompt fails
   - WorkflowPR with no `branch` field: `CreateAndSwitch` called as before (existing behavior unchanged)
   - `savePRURLToFrontmatter` with `pr-url` already set: `SetPRURL` NOT called, existing value preserved
</requirements>

<constraints>
- Do NOT break any existing passing tests
- Do NOT change behavior when `branch` field is empty
- Do NOT commit — dark-factory handles git
- Follow existing error wrapping patterns: `errors.Wrap(ctx, err, "...")`
- Coverage must not decrease for pkg/processor
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
