---
spec: ["029"]
status: created
created: "2026-03-10T20:15:00Z"
---
<summary>
- When `pr` is true and a prompt on a feature branch runs, the processor checks whether an open PR already exists for that branch before creating a new one
- If a PR already exists, its URL is logged and reused — no duplicate PR is created
- PR body includes the prompt's `issue` field as a reference line when the field is set
- When `pr` is true and `autoMerge` is enabled, the PR is only merged after the last prompt on the branch completes — earlier prompts push to the branch and update the existing PR body but do not trigger merge
- `PRCreator` gains a `FindOpenPR(ctx, branch) (string, error)` method used for duplicate detection
- `PromptFile` gains an `Issue() string` getter (if not already added by spec 028 prompt 3) to expose the issue field for PR body construction
</summary>

<objective>
Make PR creation idempotent per branch and delay auto-merge until the last prompt on the branch completes. This is the final layer for spec 029, building on the branch-switching (prompt 1) and release-guard (prompt 2) infrastructure.
</objective>

<context>
Read CLAUDE.md for project conventions.

Previous prompts of this spec will have already:
- Added `worktree bool` + `inPlaceBranch`/`inPlaceDefaultBranch` to `workflowState` (prompt 1)
- Added `HasQueuedPromptsOnBranch` to `prompt.Manager` (prompt 2)
- Added `handleBranchCompletion` to processor (prompt 2, but only called when `!p.pr`)

Read these files before making any changes:
- `pkg/processor/processor.go` — `handleCloneWorkflow` (~line 673): this is where PR creation and auto-merge happen
- `pkg/git/pr_creator.go` — `PRCreator` interface and `prCreator` implementation
- `pkg/prompt/prompt.go` — `PromptFile`, `Frontmatter.Issue` (added by spec 028 prompt 3), look for `Issue()` getter
- `pkg/processor/processor_test.go` — existing test patterns
- `mocks/pr_creator.go` — generated mock
</context>

<requirements>
**Step 1: Add `Issue()` getter to `PromptFile` (if missing)**

1. Check `pkg/prompt/prompt.go` for an existing `Issue() string` getter on `PromptFile`. Spec 028 prompt 3 adds `SetIssue` and `SetIssueIfEmpty` but may not add a getter. If a getter is missing, add it now:
   ```go
   // Issue returns the issue tracker reference from the prompt frontmatter (empty string if unset).
   func (pf *PromptFile) Issue() string {
       return pf.Frontmatter.Issue
   }
   ```
   If the getter already exists, skip this step.

**Step 2: Add `FindOpenPR` to `PRCreator`**

2. Add `FindOpenPR(ctx context.Context, branch string) (string, error)` to the `PRCreator` interface in `pkg/git/pr_creator.go`:
   ```go
   // FindOpenPR returns the URL of an open PR for the given branch, or "" if none exists.
   FindOpenPR(ctx context.Context, branch string) (string, error)
   ```

3. Implement on `prCreator`:
   ```go
   func (p *prCreator) FindOpenPR(ctx context.Context, branch string) (string, error) {
       // #nosec G204 -- branch name comes from validated frontmatter
       cmd := exec.CommandContext(
           ctx,
           "gh", "pr", "list",
           "--head", branch,
           "--state", "open",
           "--json", "url",
           "--jq", ".[0].url",
       )
       if p.ghToken != "" {
           cmd.Env = append(os.Environ(), "GH_TOKEN="+p.ghToken)
       }
       output, err := cmd.Output()
       if err != nil {
           return "", errors.Wrap(ctx, err, "list open PRs")
       }
       return strings.TrimSpace(string(output)), nil
   }
   ```
   Returns `""` (empty string) when `jq` returns null (no open PR). Returns the URL string when found.

4. Run `go generate ./...` to regenerate `mocks/pr_creator.go`.

**Step 3: Build PR body with issue reference**

5. Add a `buildPRBody` helper in `pkg/processor/processor.go` that constructs the PR body:
   ```go
   func buildPRBody(issue string) string {
       if issue != "" {
           return "Created by dark-factory\n\nIssue: " + issue
       }
       return "Created by dark-factory"
   }
   ```

**Step 4: Idempotent PR creation in `handleCloneWorkflow`**

6. In `handleCloneWorkflow` in `pkg/processor/processor.go`, before calling `p.prCreator.Create(...)`, check for an existing open PR:
   ```go
   // Check for existing open PR to avoid duplicates
   prURL, err := p.prCreator.FindOpenPR(gitCtx, branchName)
   if err != nil {
       slog.Warn("failed to check for existing PR", "branch", branchName, "error", err)
       // Fall through to create attempt — may result in duplicate, user can resolve
   }

   if prURL != "" {
       slog.Info("open PR already exists for branch — skipping creation", "branch", branchName, "url", prURL)
   } else {
       // No existing PR — create one
       issue := pf.Issue()
       body := buildPRBody(issue)
       prURL, err = p.prCreator.Create(gitCtx, title, body)
       if err != nil {
           return errors.Wrap(ctx, err, "create pull request")
       }
       slog.Info("created PR", "url", prURL)
   }
   ```
   Replace the current hardcoded `"Created by dark-factory"` body with `buildPRBody(pf.Issue())`.

**Step 5: PR auto-merge only after last prompt on branch**

7. In `handleCloneWorkflow`, the auto-merge block currently merges immediately after each prompt. Update it to only merge when this is the last queued prompt on the branch:
   ```go
   if p.autoMerge {
       // Only merge when all other prompts on this branch are done
       featureBranch := branchName
       hasMore, err := p.promptManager.HasQueuedPromptsOnBranch(ctx, featureBranch, promptPath)
       if err != nil {
           slog.Warn("failed to check remaining prompts on branch", "branch", featureBranch, "error", err)
           // Fall through: merge anyway (safe default, avoids blocking forever)
       }
       if hasMore {
           slog.Info("more prompts queued on branch — deferring auto-merge", "branch", featureBranch)
           // Move prompt to completed without merging; next prompt will trigger merge
           if err := p.moveToCompletedAndCommit(ctx, gitCtx, pf, promptPath, completedPath); err != nil {
               return err
           }
           p.savePRURLToFrontmatter(gitCtx, completedPath, prURL)
           return nil
       }
       // Last prompt on branch — proceed with merge
       if err := p.prMerger.WaitAndMerge(gitCtx, prURL); err != nil {
           return errors.Wrap(ctx, err, "wait and merge PR")
       }
       if err := p.moveToCompletedAndCommit(ctx, gitCtx, pf, promptPath, completedPath); err != nil {
           return err
       }
       return p.postMergeActions(gitCtx, ctx, title)
   }
   ```
   Note: `promptPath` here must be the path in the ORIGINAL repo (not the clone). The `handleCloneWorkflow` already calls `os.Chdir(originalDir)` before managing the prompt — make sure `promptPath` is resolved to the original repo path before using it in `HasQueuedPromptsOnBranch`.

8. For the `autoReview` path and default (no auto-merge, no auto-review) path in `handleCloneWorkflow`: no change needed — those paths already save the PR URL and do not trigger merge.

**Step 6: Tests**

9. Add tests to `pkg/processor/processor_test.go`:
    - `handleCloneWorkflow`, `FindOpenPR` returns existing URL: `Create` NOT called, existing URL used
    - `handleCloneWorkflow`, `FindOpenPR` returns empty: `Create` called with `buildPRBody` result
    - `handleCloneWorkflow`, `pf.Issue()` is non-empty: PR body contains `"Issue: <value>"`
    - `handleCloneWorkflow`, `pf.Issue()` is empty: PR body is `"Created by dark-factory"` (no issue line)
    - `handleCloneWorkflow`, `autoMerge=true`, `HasQueuedPromptsOnBranch` returns true: `WaitAndMerge` NOT called, prompt moved to completed with PR URL saved
    - `handleCloneWorkflow`, `autoMerge=true`, `HasQueuedPromptsOnBranch` returns false: `WaitAndMerge` called (existing auto-merge behavior)
    - `handleCloneWorkflow`, `autoMerge=false`: no change in behavior (PR created, no merge attempt)

10. Add tests to `pkg/git/pr_creator_test.go` for `FindOpenPR`:
    - Cannot test against real GitHub; add a unit test that verifies the `gh pr list` command args are constructed correctly (use a fake executor pattern if it exists, or document that this is covered by mock-based processor tests).
    - At minimum: verify `FindOpenPR` returns the trimmed output string on success, and returns `""` on empty output.

11. Add a test to `pkg/prompt/prompt_test.go` for `Issue()` getter (if getter was added in step 1):
    - Load a prompt with `issue: BRO-42` — `Issue()` returns `"BRO-42"`
    - Load a prompt without issue field — `Issue()` returns `""`
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- `FindOpenPR` failure is non-fatal: log a warning and fall through to `Create` (may get duplicate PR — user resolves manually per spec failure-modes table)
- `pr=true`, `autoMerge=false`: PR is always created (idempotent) but never merged — behavior unchanged from current except idempotency
- `pr=true`, `autoMerge=true`, more prompts on branch: prompt moves to completed with PR URL saved, merge deferred
- `pr=false`: `handleBranchCompletion` (from prompt 2) handles the direct-mode merge — this prompt only touches the clone/PR path
- The `issue` field is a literal string — never shell-interpolated or passed as a command argument (it is only appended to the PR body text)
- Follow existing error wrapping: `errors.Wrap(ctx, err, "message")`
- All existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
```bash
# FindOpenPR in PRCreator interface
grep -n "FindOpenPR" pkg/git/pr_creator.go

# Issue() getter exists
grep -n "func.*Issue\(\)" pkg/prompt/prompt.go

# buildPRBody helper
grep -n "buildPRBody" pkg/processor/processor.go

# HasQueuedPromptsOnBranch used in handleCloneWorkflow
grep -n "HasQueuedPromptsOnBranch" pkg/processor/processor.go

make precommit
```
Must pass with no errors.
</verification>
