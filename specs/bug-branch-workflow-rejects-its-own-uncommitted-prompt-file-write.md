---
status: draft
kind: bug
---

# `branch` workflow Setup rejects its own uncommitted prompt-file write → infinite-loop on retry

## Summary

`workflow: branch` cannot make forward progress on any retry/requeue cycle in a project that lives in the same git working tree as the prompt files. `dark-factory prompt retry` (and `requeue`) writes the prompt frontmatter (`status: failed → queued`, plus `completed:` timestamp updates) to the prompt file inside the project's git tree without committing. The next daemon cycle reaches `branchWorkflowExecutor.setupInPlaceBranch`, which calls `Brancher.IsClean(ctx)`, sees the uncommitted prompt-file change, and bails out with `working tree is not clean; cannot switch to branch "<name>"`. The branch is never created, no commit is ever attempted, and the only way to break the loop is for the user to manually `git commit` the daemon's own bookkeeping write between every retry.

The `worktree` and `clone` workflows mask this because they `git checkout` inside an isolated path, but they fall over later for unrelated reasons (see `bug-pr-create-missing-head-flag-in-isolated-workflows.md`). For `branch` workflow, this is fatal at Setup.

## Reproduction

dark-factory version: `v0.147.2-1-g30ba42f`

1. Project: `~/Documents/workspaces/jira-task-creator` (or any project whose `inboxDir` lives in the same git working tree).
2. `.dark-factory.yaml`:
   ```yaml
   workflow: branch
   pr: true
   autoMerge: true
   autoRelease: true
   autoReview: true
   allowedReviewers: ["bborbe", "pr-review-of-ben"]
   ```
3. Have any prompt that previously failed (or simply approve+queue+force-fail one).
4. From the project root with a clean working tree:
   ```bash
   git status --short      # confirm clean
   dark-factory prompt retry
   git status --short      # now dirty: M prompts/in-progress/009-...md
   dark-factory daemon
   ```
5. Watch `.dark-factory.log`:
   ```
   level=INFO msg="found queued prompt" file=009-test-loglevel-handler.md
   level=INFO msg="preflight: baseline check passed"
   level=INFO msg="executing prompt" title="..."
   level=INFO msg="syncing with remote default branch"
   level=ERROR msg="prompt failed" error="working tree is not clean; cannot switch to branch \"dark-factory/009-test-loglevel-handler\"
       github.com/bborbe/dark-factory/pkg/processor.(*branchWorkflowExecutor).setupInPlaceBranch
            /Users/bborbe/Documents/workspaces/dark-factory/pkg/processor/workflow_executor_branch.go:57
       (*branchWorkflowExecutor).Setup
            /Users/bborbe/Documents/workspaces/dark-factory/pkg/processor/workflow_executor_branch.go:47
       (*processor).ProcessPrompt
            /Users/bborbe/Documents/workspaces/dark-factory/pkg/processor/processor.go:323"
   ```
6. The prompt frontmatter flips to `status: failed` again with `lastFailReason: 'setup workflow: working tree is not clean; cannot switch to branch "dark-factory/009-test-loglevel-handler"'`.
7. `git diff prompts/in-progress/009-test-loglevel-handler.md` shows ONLY frontmatter changes (the `completed:` timestamp). The dirt that triggered the rejection is dark-factory's own bookkeeping write, not user input.
8. Retrying without an intervening manual `git commit` reproduces step 5 indefinitely.

## Expected vs Actual

**Expected** (per `docs/workflows.md` `branch` row):
> Daemon checks out a feature branch in-place, runs the prompt, commits, returns to default branch, opens a PR.

**Actual:**
- `dark-factory prompt retry` writes to the prompt file (legitimate state change: `status` and `completed` timestamps), but does not commit the change.
- On the next scan, `branchWorkflowExecutor.setupInPlaceBranch` (`pkg/processor/workflow_executor_branch.go:51-58`) calls `Brancher.IsClean` and refuses to proceed because the prompt file dark-factory just wrote is uncommitted.
- The daemon never enters the feature branch, never runs the YOLO container, never produces output. The retry loop is unbreakable from the daemon side.

## Goal

After this fix, `dark-factory prompt retry` (or `requeue`) followed immediately by `dark-factory daemon` in a `workflow: branch` project advances past Setup and into the YOLO container, on the first try, with no human-in-the-loop `git commit` between the two CLI invocations. User-side working-tree dirt (uncommitted changes in non-dark-factory paths) still aborts Setup with a clear error. The CLI/daemon's own bookkeeping writes are no longer treated as user-side dirt.

## Why this is a bug

The Setup invariant — "working tree must be clean before `git checkout`" — is correct in principle (avoid clobbering user work). But dark-factory itself is the only writer that dirtied the tree, and it knows that the dirty file is its own state file (the prompt). Treating the daemon's own bookkeeping write as an obstacle to its own forward progress is internally inconsistent.

This is symmetric to spec `bug-pr-create-missing-head-flag-in-isolated-workflows.md` — both bugs are about state writes outside an isolated workspace conflicting with git assumptions made elsewhere in the same workflow.

## Workaround

Between every retry, the user must manually commit dark-factory's frontmatter writes:

```bash
dark-factory prompt retry
git add prompts/in-progress/<name>.md
git commit -m "retry: dark-factory bookkeeping"
dark-factory daemon
```

Even with this workaround, the daemon may still write the prompt file after Setup (`pf.Save` at `processor.go:330`) before the container starts — that write happens AFTER `git checkout` lands on the feature branch, so it dirties the feature branch, not master, and is harmless. But any subsequent failure that flips the prompt back to `status: failed` lands the timestamp on the feature branch, then the next master-side retry restarts the cycle.

## Code pointers

- `pkg/processor/workflow_executor_branch.go:51-77` — `setupInPlaceBranch` is the chokepoint.
- `pkg/processor/processor.go:323` — `Setup` is called before any prompt-state persistence (`pf.Save` is at line 330, after Setup).
- `pkg/cmd/prompt_*.go` (retry, requeue, approve, complete) — the CLI commands that write prompt frontmatter without committing.
- `pkg/prompt/prompt_file.go` (`pf.Save`) — central choke point for prompt-file writes; could be the place that auto-commits to the parent project's git, OR the place that knows its own writes.

Compare with the `worktree`/`clone` executors (`pkg/processor/workflow_executor_worktree.go`, `pkg/processor/workflow_executor_clone.go`) — they sidestep the dirty-master-tree problem by working in a separate path, so this bug is `branch`-only by symptom but the root cause (CLI writes uncommitted state into the project repo) is shared.

## Constraints

- Do NOT change `Brancher.IsClean` semantics for files outside dark-factory's own state directories — user-side dirt in source files must still abort Setup.
- Do NOT change the prompt-file-as-source-of-truth invariant. Prompt frontmatter remains the canonical place to read prompt status; this fix is about *when* it's committed, not *whether*.
- Do NOT regress `worktree`, `clone`, or `direct` workflows. Those paths are out of scope for this spec (sibling spec 065 covers worktree/clone separately).
- Do NOT change existing prompt frontmatter schema. Status values, field names, and YAML structure stay as documented in `docs/prompt-writing.md`.
- Do NOT introduce a `.dark-factory/state/` sidecar directory in this spec — option D in the fix-shape triage is allowed as a follow-up but must not land here without its own spec.
- Reuse existing `Brancher`/`prompt.PromptFile` interfaces; do not introduce a parallel git-aware abstraction.

## Failure Modes

| Trigger | Expected behavior | Recovery / verification |
|---------|-------------------|--------------------------|
| `dark-factory prompt retry` then `dark-factory daemon`, master tree otherwise clean | Setup advances past `IsClean` and switches to feature branch on first daemon cycle | Daemon log shows `switched to branch for in-place execution` without an intervening manual commit |
| `dark-factory prompt requeue <id>` then `dark-factory daemon` | Same as above | Same as above |
| `dark-factory prompt approve <id>` then `dark-factory daemon` (first-run path) | Continues to work as today | No regression in existing `prompt approve` scenario |
| Master tree dirty in a non-dark-factory file (e.g. `pkg/foo/bar.go` modified, uncommitted) | Setup fails with clear error naming the offending file | Error message lists the user-side dirty path, not the dark-factory state path |
| Daemon killed mid-execution; prompt frontmatter left in intermediate state; daemon restarted | Resume picks up cleanly without manual intervention | Existing resume scenario still passes |
| Two prompts queued back-to-back, both via `retry` | Both advance through Setup without manual commits between them | Daemon log shows two `switched to branch` events, no `working tree is not clean` errors |

## Possible fix shapes (for triage only — do NOT bind prompts)

The daemon-generated fix prompts must not anchor on a single option below; the triage decision happens before approval.

- **A. Commit-on-write in CLI commands.** `prompt_retry.go`, `prompt_requeue.go`, etc. commit the prompt-file change immediately. Simple, but pollutes git history with two commits per retry (status flip + actual work).
- **B. Stash-around-Setup.** `setupInPlaceBranch` stashes any change to files inside `inboxDir`/`inProgressDir` before `git checkout`, pops after switch. Keeps history clean but introduces stash-management complexity (stash collisions, partial pops, etc.).
- **C. Filter `IsClean` to user-relevant paths.** Tell `Brancher.IsClean` to ignore changes inside dark-factory's own state directories. Cleanest semantically — the daemon's own bookkeeping is not user dirt — but requires teaching the brancher about dark-factory layout.
- **D. Move CLI state writes outside the project tree.** Keep prompt status in a sibling state dir or `.dark-factory/state/`, leave the prompt files themselves immutable user content. Biggest refactor; cleanest long-term separation.

## Acceptance Criteria

- [ ] Reproduction in `~/Documents/workspaces/jira-task-creator` with `workflow: branch` + `dark-factory prompt retry` + `dark-factory daemon` advances past Setup without a human-in-the-loop `git commit` between retry and daemon-start.
- [ ] User-side dirt (e.g. an uncommitted change to `pkg/foo/bar.go`) still causes Setup to fail with an actionable error message.
- [ ] No regression in `worktree`, `clone`, `direct` workflows (covered by existing scenarios).
- [ ] No regression in the `dark-factory prompt approve` first-run path.
- [ ] Daemon-resume after kill (mid-execution prompt frontmatter state) works without manual cleanup.
- [ ] `docs/workflows.md` `branch`-row commentary documents that dark-factory commits prompt-frontmatter writes before Setup (or the equivalent invariant per the chosen fix shape) so users understand why the working tree is briefly modified by the CLI itself.

## Verification

Per `docs/bug-workflow.md` §Verification, this bug is a runtime symptom — unit tests alone are not sufficient. The repro must be replayed against the built binary and produce the expected behavior.

**Repro replay (must run after fix lands):**

```bash
# In a project configured with workflow: branch
cd ~/Documents/workspaces/jira-task-creator
git status --short    # confirm clean

# Force the failure precondition: dirty the prompt file via retry
dark-factory prompt retry
git status --short    # one M line for the prompt file is acceptable; nothing else dirty

# Run daemon — must NOT fail with "working tree is not clean"
dark-factory daemon &
DAEMON_PID=$!

# Watch the log; expected lines (in order):
#   "found queued prompt"
#   "preflight: baseline check passed"
#   "executing prompt"
#   "syncing with remote default branch"
#   "switched to branch for in-place execution" branch=dark-factory/<name>
# MUST NOT see:
#   "working tree is not clean; cannot switch to branch"

# Cleanup
kill $DAEMON_PID
```

**Negative-control replay (user-side dirt must still fail):**

```bash
# In the same project, with a real source file dirty:
echo "// scratch" >> pkg/handler/list-sprints.go
dark-factory prompt retry
dark-factory daemon &

# Expected: Setup fails with an error that names pkg/handler/list-sprints.go
# (not the prompt file). Confirms the fix didn't blanket-ignore IsClean.
```

**Acceptable evidence for `verifying → completed`:**

| Evidence | Acceptable? |
|----------|-------------|
| Daemon log shows `switched to branch` after `prompt retry` with no manual commit | Yes |
| Negative-control above shows Setup aborts on user-side dirt | Yes |
| Unit test asserting `IsClean` filter logic | Necessary but not sufficient |
| "All tests pass" without runtime replay | No |

## Open Questions

1. Which fix shape (A/B/C/D)? **Must be resolved before `dark-factory spec approve`** — the prompt generator anchors on the spec, so an unresolved triage produces unanchored prompts.
2. Are `prompt approve`, `prompt cancel`, `prompt unapprove`, `prompt complete` affected the same way, or only `retry`/`requeue`? Audit the full CLI surface.
3. Should `pf.Save` (or its callers) be the single place that knows about parent-git semantics, or should each call site decide independently?
4. The companion spec `bug-pr-create-missing-head-flag-in-isolated-workflows.md` is in the same dispatch path. Should both be fixed in one campaign or as separate specs? Likely separate — different files, different test surface — but worth coordinating to avoid merge churn.

## See also

- `bug-pr-create-missing-head-flag-in-isolated-workflows.md` — sibling bug in `worktree`/`clone` workflows. Same root family: dark-factory's own writes interacting badly with git assumptions in the parent repo.
- Spec 063 (`bug-autorelease-overrides-pr-workflow`) — earlier dispatch-path bug in this same area.
- `pkg/processor/workflow_executor_branch.go:47, 57` — the exact rejection site.
- `pkg/processor/processor.go:323-330` — Setup-before-Save ordering.
