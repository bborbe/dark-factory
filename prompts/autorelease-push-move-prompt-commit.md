---
status: draft
---

# autoRelease: push the post-release "move prompt to completed" commit

<summary>
- When `autoRelease: true` is set, the release commit and tag are correctly pushed via `CommitAndRelease` in `pkg/git/git.go`.
- However, the subsequent "move prompt to completed" commit (Phase 4 in the direct-workflow executor) is committed locally but never pushed.
- Operators must run `git push` manually after every prompt completes, defeating the point of `autoRelease`.
- Empirically observed: after the `bborbe/run` migration prompt completed under `autoRelease=true`, the release commit `release v1.9.23` and tag `v1.9.23` were pushed, but commit `3ad2fa2 move prompt to completed` remained local until manually pushed.
</summary>

<objective>
When `autoRelease: true`, the post-release "move prompt to completed" commit must be pushed to the remote automatically. After this change, no manual `git push` is required for any phase of a successful direct-workflow run with `autoRelease=true`.
</objective>

<context>
The relevant code path:

- `pkg/processor/workflow_executor_direct.go:60-103` (`directWorkflowExecutor.completeCommit`) — runs four phases: (1) commit work files, (2) move prompt file, (3) auto-complete specs, (4) commit the prompt-file move.
- Phase 1's commit goes through the autoRelease path (`CommitAndRelease` in `pkg/git/git.go:158`) which pushes both the commit and the tag.
- Phase 4 calls `e.deps.Releaser.CommitCompletedFile` which lives at `pkg/git/git.go:208-234`. This function does `git add <path>` + `git status --porcelain` + `git commit -m "move prompt to completed"`, but does NOT push.
- The `gitPush` helper exists at `pkg/git/git.go:445` and is currently called from `CommitAndRelease` (lines 192-194) and `gitPushTag` (used for tag push).
- `pkg/committingrecoverer/recoverer.go:127` ALSO calls `CommitCompletedFile` from a different code path (recovery after partial failure). The push behavior should be consistent there too — when autoRelease is on, the recovered commit should also be pushed.
- Configuration: `pkg/config/config.go:103` has `AutoRelease bool`. The releaser interface at `pkg/git/git.go:95` declares `CommitCompletedFile(ctx, path) error`. The releaser is constructed via the factory and may need to know about the autoRelease flag.

The existing test infrastructure: `pkg/git/git_test.go` (or similar) tests `CommitCompletedFile`. There should be table-driven tests for both autoRelease=true (push happens) and autoRelease=false (push does NOT happen).

Workflow context: `autoRelease=false` (default) means "commit locally only" — neither the release nor the prompt-move commit gets pushed. `autoRelease=true` means "push everything after each prompt completes". The current behavior pushes the release commit but strands the prompt-move commit, which is internally inconsistent.
</context>

<requirements>
1. **`CommitCompletedFile` must push when autoRelease is on.** Modify the releaser's `CommitCompletedFile` so that, after a successful local commit, it pushes the current branch to the remote. The push must be conditional on `autoRelease: true` — when `autoRelease: false`, behavior is unchanged (local commit only, no push), preserving the documented contract that autoRelease=false produces local-only changes.

2. **Surface `autoRelease` to the releaser.** The releaser at `pkg/git/git.go` is constructed in the factory; thread the `AutoRelease` config field into the constructor so `CommitCompletedFile` can branch on it. A clean approach is to store `autoRelease bool` on the `releaser` struct and check it inside `CommitCompletedFile`.

3. **Apply the same fix to the recovery path.** `pkg/committingrecoverer/recoverer.go:127` calls `CommitCompletedFile` after recovering from a partial failure. The push behavior must be consistent — if autoRelease is on, the recovered prompt-move commit must also be pushed.

4. **Reuse the existing `gitPush` helper.** Don't duplicate the push subprocess logic. Add a `gitPush(ctx)` call inside `CommitCompletedFile` after the successful commit, gated on `r.autoRelease`. If `gitPush` is currently package-private and not on the releaser, expose it appropriately or wrap it in a `releaser.push` method.

5. **Tests.** Add table-driven tests covering:
   - `autoRelease=true` + clean dirty state → commit + push both happen
   - `autoRelease=true` + nothing to commit (empty `git status`) → no commit, no push (early return preserved)
   - `autoRelease=false` + clean dirty state → commit happens, push does NOT happen
   - Push failure surfaces as a wrapped error from `CommitCompletedFile` (use `bborbe/errors.Wrap` consistent with the rest of the file)

6. **No behavior change for `autoRelease=false`.** Existing tests for the false case must continue to pass without modification.

7. **No changes to `CommitAndRelease`.** That path already pushes correctly. Only `CommitCompletedFile` needs the fix.
</requirements>

<verification>
- After running a prompt to completion in a test repo with `autoRelease: true`, `git status` shows the working tree clean AND `git rev-list @{u}..HEAD --count` returns `0` (no unpushed commits).
- After running a prompt to completion with `autoRelease: false`, `git status` shows clean tree AND there are local commits NOT pushed (preserving existing behavior — the operator pushes manually when ready).
- Existing tests pass: `make test`.
- New tests for the four cases listed in requirement 5 pass.
- `make precommit` succeeds end-to-end.
- Manual smoke: pick any bborbe library that already has the migration prompt completed (e.g. `bborbe/run`) and confirm — by inspecting `pkg/git/git.go` and the test file — that the conditional-push logic is present and tested.
</verification>

<constraints>
- Don't change the public `Releaser` interface signature beyond what's necessary to thread `autoRelease` (constructor change is fine; method signatures should stay stable).
- Don't push tags from `CommitCompletedFile` — the prompt-move commit has no tag. Only push the branch.
- Don't add retry logic to the push — `CommitWithRetry` wraps the call site; consistency with the existing release flow (which doesn't retry the push either, only the commit) is the bar.
- Don't touch unrelated workflows (branch, PR) — only the direct workflow needs the conditional-push gate. The branch/PR workflows already push the feature branch via `Brancher.Push` and the merge happens on the remote.
- Don't remove or weaken the `len(strings.TrimSpace(string(output))) == 0` early-return — when there's nothing to commit, neither commit nor push should happen.
</constraints>
