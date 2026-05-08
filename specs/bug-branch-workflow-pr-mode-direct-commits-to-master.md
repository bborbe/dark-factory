---
status: idea
---

# Branch workflow with `pr=true` silently downgrades to direct when prompt was queued under a previous daemon

## Status: ROOT CAUSE NARROWED 2026-05-08

**Initial framing was wrong.** Branch+pr=true DOES work — it successfully opened PR #5 on `bborbe/maintainer` for prompt 106 the next day, using the SAME daemon (PID 15029) and SAME flags (`--set workflow=branch --set pr=true --set autoRelease=false`).

**Difference between the failing case (105) and the succeeding case (106)**: prompt 105 was approved while the OLD direct-mode daemon was still running. The old daemon picked it up and called `Setup` in direct mode (no `branch:` field set on the prompt frontmatter). Then the daemon was killed and restarted with branch+pr flags. The new daemon re-attached to the still-running container, but `Setup` had already completed under direct semantics — re-attaching does NOT re-run `Setup` with the new config. So 105 finalized as a direct-mode commit even though the active daemon's effective config said branch+pr.

**Evidence:**

- Prompt 105 `prompts/completed/105-make-race-optional-disable-in-ci.md` frontmatter has NO `branch:` field — i.e. `BranchWorkflowExecutor.Setup` never ran:
  ```yaml
  status: completed
  created: "2026-05-08T14:49:18Z"
  queued: "2026-05-08T14:49:18Z"
  started: "2026-05-08T14:49:20Z"  ← OLD direct-mode daemon picked it up
  completed: "2026-05-08T14:51:41Z"
  # no `branch:` field — Setup ran in direct workflow
  ```
- New daemon started at `2026-05-08T14:50:14Z` (per its own `effective config` log). 105 had already started 54 seconds earlier under the old direct-mode daemon.
- Prompt 106 `prompts/completed/106-make-filename-len-caps-env-configurable.md` (on `dark-factory/106-…` branch) DOES have `branch:`:
  ```yaml
  status: completed
  branch: dark-factory/106-make-filename-len-caps-env-configurable  ← Setup ran in branch workflow
  ```
  106 was approved AFTER the new daemon was already running in branch+pr mode → fresh Setup → PR opened correctly.

## Severity: operational footgun, not code bug

The branch workflow code is correct. `BranchWorkflowExecutor.Setup` would have created the branch if it had run for 105 — but it didn't, because the OLD daemon's direct workflow handled Setup, then the daemon was swapped mid-flight.

This is a **state-survives-daemon-restart** issue: workflow mode is decided at first-pick-up time, not at the daemon's current effective-config time. Restarting the daemon with new flags does not retroactively change in-flight prompts' workflow.

## Recommended fix

Two options, ranked by cost:

1. **Warn on re-attach to a container started under a different workflow** (cheap): when daemon re-attaches to a running container after restart, compare the workflow used when Setup ran (frontmatter `branch:` presence vs. absence) against the daemon's current effective workflow. If they don't match, log a clear WARN like "re-attaching to container that was started under direct workflow; current daemon config (branch+pr) will not apply to this prompt — finishing in original workflow". Operator sees the issue immediately.

2. **Refuse to re-attach when workflow has changed** (stricter): kill the running container and re-queue the prompt under the new workflow. More invasive but eliminates the silent-downgrade. Costs token budget for the prompt's re-execution.

3. **Not recommended**: try to "convert" mid-flight from direct to branch (e.g. cherry-pick the agent's commits onto a feature branch and force-push). Too magical, too many edge cases.

## Original problem statement (preserved for context)

`dark-factory daemon --set workflow=branch --set pr=true` is documented in `--help` as a valid combination ("Workflow example: --set workflow=branch --set pr=true"). The daemon log confirms both overrides take effect:

```
effective config ... workflow=branch workflowSource=arg pr=true prSource=arg autoRelease=false autoReleaseSource=arg
```

But the executed prompt commits **directly to master** — no `dark-factory/<basename>` feature branch created locally or remotely, no PR opened. `BranchWorkflowExecutor.Setup` (`pkg/processor/workflow_executor_branch.go:42-48`) is supposed to derive a branch name from baseName when frontmatter has none and switch to it; in practice this never engaged.

Either:
1. The override propagation breaks somewhere between `effective config` logging and `CreateWorkflowExecutor` selection (the wrong executor wired in); or
2. The branch executor is wired in but `Setup` doesn't actually switch branches before agent execution; or
3. `Setup` switches but the agent's container commits land on master via host-side `git` somehow; or
4. `autoRelease=false` interaction silently downgrades to direct workflow.

Investigation needed.

## Reproduction

dark-factory version: `v0.156.1-1-g04f3863-dirty` (built locally).

1. Project: `~/Documents/workspaces/maintainer` (`.dark-factory.yaml` has `workflow: direct, autoRelease: true`).
2. Stop any running daemon: `kill $(cat .dark-factory.lock)`.
3. Approve a small prompt (e.g. a single-file edit + CHANGELOG entry, exit code 0 trivially).
4. Start daemon with overrides:
   ```bash
   dark-factory daemon --set workflow=branch --set pr=true --set autoRelease=false
   ```
5. Wait for completion.

**Expected:**
- Local: `git branch -a` shows `dark-factory/<prompt-basename>`
- Remote: PR opened against master via `gh` or equivalent
- `prompts/completed/<prompt>.md` frontmatter has `pr_url:` populated
- master untouched (until human merges the PR)

**Actual:**
- Local: no `dark-factory/*` branch
- No PR opened (`gh pr list` shows no new PRs)
- Master directly received the commit
- `prompts/completed/<prompt>.md` exists but no `pr_url:` frontmatter

## Evidence (from this incident, 2026-05-08)

**Project:** `~/Documents/workspaces/maintainer`
**Prompt:** `prompts/completed/105-make-race-optional-disable-in-ci.md`

**Daemon log line confirming overrides applied:**
```
time=2026-05-08T16:50:14.679+02:00 level=INFO msg="effective config"
  maxContainers=8 ...
  workflow=branch workflowSource=arg
  pr=true prSource=arg
  autoRelease=false autoReleaseSource=arg
  autoMerge=false autoMergeSource=default
  ...
```

**Git history after run:**
```
05b8662 Default-on path                                       <- agent's commit, on master
7da3cd2 move prompt to completed                              <- dark-factory's bookkeeping commit, on master
```

**Local branches:** no `dark-factory/*` branches present.

**Open PRs on bborbe/maintainer:** zero new PRs (only an unrelated 2026-04-23 test PR).

**Note on the commit message:** "Default-on path" is verbatim a markdown header from the prompt's `<verification>` section. The agent inside the claude-yolo container picked it up as the commit message — suggests the agent invoked `git commit` itself rather than letting dark-factory's workflow executor stage and commit. If that's true, then `Setup` may not have switched to a feature branch before launching the container.

## Source pointers

- `pkg/factory/factory.go:769-778` — workflow switch dispatch (Clone/Worktree/Branch/Direct)
- `pkg/processor/workflow_executor_branch.go:33-94` — `BranchWorkflowExecutor.Setup` (creates feature branch)
- `pkg/processor/workflow_executor_branch.go:122-125` — `Complete` checks `e.deps.PR` and routes to `handleBranchPRCompletion`
- `pkg/processor/workflow_executor_branch.go:200ish` — `handleBranchPRCompletion` (calls `findOrCreatePR`)

If the bug is in dispatch, the symptom would be `BranchWorkflowExecutor` never instantiated despite `workflow=branch` in effective config. If the bug is in `Setup`, the container would run on master. If the bug is in `Complete`, the container would commit to a feature branch but no PR would open (and we'd see the branch locally — we don't).

The "no feature branch exists" evidence points to either dispatch-level routing or `Setup` not engaging.

## Hypotheses (ordered by likelihood — REVISED 2026-05-08)

**Hypothesis 3 (state baked at start time, not approve time) confirmed by prompt 106 success**: workflow is decided when `BranchWorkflowExecutor.Setup` first runs (immediately after the daemon picks up an approved prompt). For 105, that was the OLD direct-mode daemon. For 106, it was the NEW branch+pr daemon. Same daemon binary, same flags after the restart, but pre-existing in-flight work continued under its original workflow. ✅ Root cause.

Other hypotheses ruled out:

1. ~~CLI override applied to logging but not to workflow-executor wiring~~ — disproved: 106 succeeded with the same flags.
2. ~~autoRelease=false short-circuits the PR codepath~~ — disproved: 106 had autoRelease=false and still opened PR #5.
4. ~~Agent commits inside container bypass the workflow executor entirely~~ — disproved: 106's commits landed on the feature branch correctly.

## Acceptance Criteria (REVISED 2026-05-08)

- [x] Root cause identified — workflow is bound to the prompt at `Setup` time, not at completion time. Daemon restart with new workflow flags does not retroactively re-route in-flight prompts.
- [ ] Daemon detects re-attachment to a container started under a different workflow than its current effective config. Logs a clear WARN naming both the original-Setup workflow and the current effective workflow.
- [ ] WARN message points to the operational pattern: "drain in-flight prompts before changing workflow flags, OR explicitly cancel + requeue affected prompts".
- [ ] (Optional, stretch) Daemon refuses re-attachment with `--strict-workflow` flag — kills the running container and re-queues the prompt fresh under the new workflow.
- [ ] Add a section to `docs/workflows.md` documenting the daemon-restart pitfall: changing workflow flags only affects newly-Setup prompts.
- [ ] Regression test: a Ginkgo test that simulates daemon-restart-mid-execution and asserts the WARN logs.
- [ ] `make precommit` clean.

## Non-goals

- Fixing the unrelated `autoRelease+pr+autoMerge` validator combination (separate concern; the validator is *correct* in flagging that combo, just may be over-restrictive).
- Changing the default workflow.
- Changing how `pr_url:` is persisted in frontmatter (separate feature).

## Verification

After the fix, repeat the reproduction steps. The acceptance-criteria checklist must all pass. Specifically:

```bash
# Cleanup any prior state
cd ~/Documents/workspaces/dark-factory-sandbox  # or any clean test project
git checkout master && git pull
git branch -D dark-factory/test-branch-pr-mode 2>/dev/null

# Drop a trivial prompt and approve
cat > prompts/<NNN>-test-branch-pr.md <<'EOF'
---
status: draft
---
<summary>
trivial prompt — write a single line to /tmp/test.txt
</summary>
# Test
Run `echo hi > /tmp/test-branch-pr.txt`. No file changes in the repo are needed.
EOF
dark-factory prompt approve <NNN>-test-branch-pr

# Run with overrides
dark-factory daemon --set workflow=branch --set pr=true --set autoRelease=false

# After completion:
git branch -a | grep dark-factory  # expect: dark-factory/<NNN>-test-branch-pr (local + remote)
gh pr list --state open             # expect: open PR for that branch
git log master..origin/master       # expect: master untouched (no merge)
grep pr_url prompts/completed/<NNN>-test-branch-pr.md  # expect: pr_url: https://...
```

## Do-Nothing Option

Without this fix, `--set workflow=branch --set pr=true` is broken/misleading. Users wanting PR-mode for safety (avoiding direct-to-master commits in repos with broken CI, contentious changes, or formal review requirements) cannot use it. Workaround: open the PR manually after each daemon run and revert the master commit — adds toil and risks losing the change. The `--help` text gives a wrong example, eroding trust in the tool's documentation.

If we don't fix this, the docs should be corrected to say PR mode is unsupported and remove the `--set workflow=branch --set pr=true` example from `--help`.
