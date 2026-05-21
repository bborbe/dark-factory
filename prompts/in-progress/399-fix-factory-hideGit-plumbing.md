---
status: committing
spec: [084-fail-fast-on-worktree-without-hidegit]
summary: Replaced hardcoded false with cfg.Workflow == config.WorkflowWorktree || cfg.HideGit for hideGit parameter in CreateSpecGenerator, matching prompt executor pattern at line 891
container: dark-factory-exec-399-fix-factory-hideGit-plumbing
dark-factory-version: v0.164.0
created: "2026-05-21T21:45:01Z"
queued: "2026-05-21T21:33:35Z"
started: "2026-05-21T21:34:20Z"
branch: dark-factory/fail-fast-on-worktree-without-hidegit
---

<summary>
- `pkg/factory/factory.go:654` now passes the resolved `hideGit` value to the spec generator's Docker executor instead of hardcoded `false`
- The expression at line 654 mirrors line 891: `workflow == config.WorkflowWorktree || hideGit`
- The misleading comment `// hideGit — spec generators never need .git masking` is removed
</summary>

<objective>
Fix the spec generator's `hideGit` plumbing in `pkg/factory/factory.go`. Line 654 hardcodes `false` for the `hideGit` parameter to `executor.NewDockerExecutor`, but the correct value is the same resolved expression used at line 891 for the prompt executor: `workflow == config.WorkflowWorktree || hideGit`.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/factory/factory.go` — line 654 (the line to fix) and line 891 (the pattern to mirror).
Read `pkg/config/config.go` — the `Config` struct fields and methods including `Workflow` and `HideGit` to confirm the accessor pattern.
</context>

<requirements>
1. In `pkg/factory/factory.go`, modify `CreateSpecGenerator` (line ~654):
   - Change `false, // hideGit — spec generators never need .git masking` to `cfg.Workflow == config.WorkflowWorktree || cfg.HideGit,`
   - Remove the comment entirely (do not replace it)
   - The expression must be identical in semantics to line 891

2. Verify that `cfg.Workflow` and `cfg.HideGit` are the correct accessors by checking the `config.Config` struct definition in `pkg/config/config.go`. The field names are `Workflow` (type `config.Workflow`) and `HideGit` (type `bool`).

3. Confirm that `config.WorkflowWorktree` is a valid enum value by checking `pkg/config/config.go`.

4. Run `make precommit` to confirm the change compiles and all tests pass.
</requirements>

<constraints>
- Do NOT change anything else at line 654 — only replace `false` with the correct expression and remove the comment
- Do NOT change line 891 or any other call site
- Do NOT add new imports — the `config` package is already imported in factory.go
- Do NOT commit — dark-factory handles git
- `make precommit` must pass
</constraints>

<verification>
```bash
make precommit
```

Additional evidence:
```bash
# Confirm the misleading comment is gone
grep -n 'spec generators never need' pkg/factory/factory.go

# Confirm the old hardcoded false is gone
grep -n 'false, // hideGit' pkg/factory/factory.go
```
</verification>
