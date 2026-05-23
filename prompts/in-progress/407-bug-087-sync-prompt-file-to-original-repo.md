---
status: approved
spec: [087-bug-clone-worktree-move-not-applied-to-original]
created: "2026-05-23T00:00:00Z"
queued: "2026-05-23T09:14:52Z"
branch: dark-factory/bug-clone-worktree-move-not-applied-to-original
---

<summary>
- Clone and worktree workflows now mirror the prompt-file move into the ORIGINAL repo AFTER the combined commit pushes successfully. Direct and branch workflows are NOT touched (they already work correctly).
- The mirror operation is filesystem-only: a `Rename` of `prompts/in-progress/<id>.md` → `prompts/completed/<id>.md` in the original, plus a frontmatter update to `status: completed`. No `git` invocations, no commit, no push.
- Idempotent: if the original already has the file at `prompts/completed/<id>.md`, the operation is a no-op.
- Source-absent-and-destination-absent is treated as a true divergence and emits a `clone-sync-mismatch` WARN naming both paths — it is NOT silently swallowed.
- On any rename failure after a successful remote push, the workflow returns success-with-warning: log `clone-sync-mismatch` at WARN level, do not propagate the error to fail the prompt. The remote already has the rename; the operator can `git pull` to catch up.
- `savePRURLToFrontmatter` (the downstream step that fails today with "no such file or directory") finds the file at the new path and updates frontmatter without error after this fix.
- `docs/workflows.md` gets one paragraph describing the post-push mirror operation for clone/worktree.
- `CHANGELOG.md` gets a new entry under `## Unreleased`.
</summary>

<objective>
After this prompt lands, every successful clone-workflow or worktree-workflow run ends with the ORIGINAL repo's prompt file at `prompts/completed/<id>.md` with `status: completed` in frontmatter, AND with zero local commits ahead of `origin/master` introduced by the mirror operation, AND with no `failed to save PR URL to frontmatter` errors in the daemon log.
</objective>

<context>
Read first (no edits yet):

- `CLAUDE.md` — project conventions.
- `specs/in-progress/087-bug-clone-worktree-move-not-applied-to-original.md` — the spec this prompt implements.
- `specs/completed/086-bug-prompt-move-not-pushed.md` — the prior, immutable spec. This spec is ADDITIVE to 086; the order of operations established by 086 (rename inside isolated tree → work commit → push) MUST NOT change.
- `docs/workflows.md` — the lifecycle doc that this prompt updates.
- `docs/bug-workflow.md` — bug-fix methodology.

Code anchors (functions, not line numbers — line numbers drift):

- `pkg/processor/workflow_executor_clone.go` — `(*cloneWorkflowExecutor).Complete`. After spec 086 the sequence inside is: MoveToCompleted (in clone) → CommitOnly → Push → chdir back → Cloner.Remove → handleAfterIsolatedCommit.
- `pkg/processor/workflow_executor_worktree.go` — `(*worktreeWorkflowExecutor).Complete`. Same shape: MoveToCompleted (in worktree) → CommitOnly → chdir back → Worktreer.Remove → handleAfterIsolatedCommit.
- `pkg/processor/workflow_helpers.go` — `handleAfterIsolatedCommit` and `savePRURLToFrontmatter`. `savePRURLToFrontmatter` opens the prompt at the new `completed/` path in the ORIGINAL — it is the function that crashes today because of the missing mirror.
- `pkg/prompt/prompt.go` — `Manager`, `MoveToCompleted`, `FileMover` interface. `MoveToCompleted` already does exactly what the mirror operation needs (rename + frontmatter), but it operates on the CWD. Calling it from inside the ORIGINAL repo (after chdir back) reuses this logic.

Direct + branch workflows MUST NOT be modified by this prompt. They operate in the daemon's own repo, so spec 086's move-before-commit is already correct for them.

Coding guides:

- `~/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `errors.Wrap` / `errors.Wrapf`, never `fmt.Errorf`.
- `~/.claude/plugins/marketplaces/coding/docs/go-doc-best-practices.md` — exported types/functions need godoc.
</context>

<requirements>

### 1. Add `syncPromptFileToOriginalRepo` to `pkg/processor/workflow_helpers.go`

Unexported helper function. Signature and semantics:

```go
// syncPromptFileToOriginalRepo mirrors the in-progress → completed rename into the
// ORIGINAL repo AFTER the combined commit has already been pushed from an isolated
// clone/worktree. It is filesystem-only: no git calls, no remote operations.
//
// Idempotent:
//   - If the file is already at completedPath, this is a no-op (debug log).
//   - If the file is missing at promptPath AND missing at completedPath, the original
//     repo's view has truly diverged from the pushed remote; the function returns a
//     wrapped "clone-sync-mismatch" error so the caller can WARN.
//
// On rename failure, returns the wrapped error so the caller can log
// "clone-sync-mismatch" and continue with success-with-warning semantics.
func syncPromptFileToOriginalRepo(
    ctx context.Context,
    promptMgr PromptManager,    // pkg/processor's PromptManager interface (MoveToCompleted is on it)
    promptPath, completedPath string,
) error
```

Behaviour, step by step:

1. If `os.Stat(completedPath) == nil`: file already at destination → `slog.DebugContext(ctx, "sync-already-at-completed", "path", completedPath)` → return `nil`.
2. If `os.Stat(promptPath) == nil`: file present at source → call `promptMgr.MoveToCompleted(ctx, promptPath)` (reuses the existing rename + frontmatter update logic from spec 086). Wrap any error with `errors.Wrap(ctx, err, "sync prompt file to original repo")` and return it.
3. If both source AND destination are missing: return `errors.Errorf(ctx, "clone-sync-mismatch: prompt absent at both %s and %s", promptPath, completedPath)`. This is the divergence case described in spec 087 — it MUST surface to the caller, not be silently swallowed.

Implementation constraints:

- The helper MUST be filesystem-only. It MUST NOT import `pkg/git`, MUST NOT use `os/exec` to call `git`, and MUST NOT touch any remote. The only operations it performs are `os.Stat` and (via `promptMgr.MoveToCompleted`) `os.Rename` + a frontmatter write.
- The helper MUST NOT change CWD. Callers are responsible for the CWD being the original repo before calling.

### 2. Wire the helper into `cloneWorkflowExecutor.Complete`

In `pkg/processor/workflow_executor_clone.go`, after `e.cleanedUp = true` (i.e. after `Cloner.Remove` succeeded and the CWD is the original repo) and BEFORE `return handleAfterIsolatedCommit(...)`, insert:

```go
if syncErr := syncPromptFileToOriginalRepo(
    ctx,
    e.deps.PromptManager,
    promptPath,
    completedPath,
); syncErr != nil {
    slog.WarnContext(ctx,
        "clone-sync-mismatch",
        "promptPath", promptPath,
        "completedPath", completedPath,
        "remoteBranch", e.branchName,
        "error", syncErr.Error(),
        "hint", "remote has the rename; run `git pull` on the original repo to catch up",
    )
    // success-with-warning: do not propagate; remote is already correct.
}
```

Do NOT change the signature of `handleAfterIsolatedCommit`. Do NOT add new parameters. Do NOT alter the order of operations established by spec 086.

### 3. Wire the helper into `worktreeWorkflowExecutor.Complete`

Same pattern as req 2, in `pkg/processor/workflow_executor_worktree.go`, after `e.cleanedUp = true` (post `Worktreer.Remove`) and BEFORE `return handleAfterIsolatedCommit(...)`. Use the same `slog.WarnContext` call with key `"clone-sync-mismatch"` (the log line label is shared; the executor identity is implicit in the surrounding log context).

### 4. Do NOT touch `workflow_executor_direct.go` or `workflow_executor_branch.go`

These workflows already operate in the same repo the daemon sees. Spec 086's move-before-commit is already correct for them. The fix is scoped to clone + worktree only. Any change to direct or branch executors in this prompt is a regression and MUST be rejected.

### 5. Do NOT modify `handleAfterIsolatedCommit` or `savePRURLToFrontmatter`

These functions are correct as-is once the mirror runs. The mirror makes `prompts/completed/<id>.md` exist in the original BEFORE `savePRURLToFrontmatter` opens it. No changes to those functions, their signatures, or their callers other than the new sync call BEFORE them.

### 6. Update `docs/workflows.md`

Add one paragraph (or extend the existing post-spec-086 section) explaining the post-push mirror for clone/worktree workflows. The text must state:

- The mirror runs only for clone and worktree workflows.
- It runs AFTER the combined commit is pushed and AFTER the isolated working tree is destroyed.
- It is filesystem-only (no git commit, no remote operations).
- On failure the workflow returns success-with-warning and emits `clone-sync-mismatch`; the operator recovers via `git pull`.

### 7. Add a `CHANGELOG.md` entry

Under `## Unreleased` add a bullet:

```
- fix: clone and worktree workflows now mirror the in-progress → completed rename into the original repo after push, so the daemon's local view matches `origin/master` and `savePRURLToFrontmatter` no longer errors (spec 087, follow-up to spec 086)
```

If no `## Unreleased` section exists, add one above the most recent `## vX.Y.Z` heading.

</requirements>

<constraints>
- The mirror operation MUST be filesystem-only. No `git` invocations of any kind in `syncPromptFileToOriginalRepo` or in the new code added to the executors.
- The mirror MUST NOT add any commit to the original repo: `git -C <original> log origin/master..HEAD --oneline` MUST return empty output after a clone/worktree run.
- Direct and branch workflows MUST be completely untouched by this prompt. No edits to `workflow_executor_direct.go` or `workflow_executor_branch.go`. No edits to their test files.
- `handleAfterIsolatedCommit`'s signature MUST NOT change. No new parameters added.
- The mirror MUST be idempotent: re-running against an already-mirrored prompt MUST succeed without error and MUST NOT log at WARN.
- The "both source and destination absent" case MUST NOT be silently swallowed. It MUST surface as `clone-sync-mismatch` WARN.
- `savePRURLToFrontmatter` is not modified. The mirror runs before it; that is sufficient.
- The order established by spec 086 (move inside isolated tree → commit → push) is preserved. This spec adds a step AFTER push completes, never before.
- Errors wrap via `errors.Wrap` / `errors.Wrapf`. Never `fmt.Errorf`.
- All new code (helper, log lines) lives in `pkg/processor/workflow_helpers.go`.
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
```bash
make precommit

# Helper exists, no git in its body
grep -n "^func syncPromptFileToOriginalRepo" pkg/processor/workflow_helpers.go              # ≥1 match
grep -n "exec\|pkg/git\|Brancher\|Releaser" pkg/processor/workflow_helpers.go | grep -i syncPromptFileToOriginalRepo  # zero matches (helper does no git)

# Mirror is wired into clone + worktree
grep -n "syncPromptFileToOriginalRepo" pkg/processor/workflow_executor_clone.go             # ≥1 match
grep -n "syncPromptFileToOriginalRepo" pkg/processor/workflow_executor_worktree.go          # ≥1 match

# Mirror is NOT wired into direct or branch
grep -n "syncPromptFileToOriginalRepo" pkg/processor/workflow_executor_direct.go            # zero matches
grep -n "syncPromptFileToOriginalRepo" pkg/processor/workflow_executor_branch.go            # zero matches

# handleAfterIsolatedCommit signature unchanged
grep -n "^func handleAfterIsolatedCommit" pkg/processor/workflow_helpers.go                 # ≥1 match
# (manual review: argument list matches the version at HEAD before this prompt)

# WARN log line is present in BOTH clone and worktree
grep -n "clone-sync-mismatch" pkg/processor/workflow_executor_clone.go pkg/processor/workflow_executor_worktree.go pkg/processor/workflow_helpers.go  # ≥3 matches total

# Docs + CHANGELOG updated
grep -n "clone-sync-mismatch\|mirror\|original repo" docs/workflows.md   # ≥1 match
head -20 CHANGELOG.md | grep -iE "spec 087|clone.*worktree.*mirror|in-progress.*completed.*original"  # ≥1 match
```

`make precommit` MUST exit 0. Sibling prompt `2-bug-087-sync-prompt-file-tests.md` provides the ginkgo tests that exercise the helper + both executors end-to-end; this prompt only asserts the structural greps above plus the make-precommit baseline.
</verification>
