---
status: idea
tags:
  - dark-factory
  - spec
  - idea
---

## Summary

- On a project with **fully protected master** + `workflow: clone` (or `branch`/`worktree`) + `pr: true`, the PR produced by each prompt run contains only the prompt's *work commit* — i.e. code change + that one prompt's `prompts/in-progress/<id>.md → prompts/completed/<id>.md` rename.
- Everything else that dark-factory writes during the spec/prompt lifecycle never makes it into a PR. It stays as uncommitted changes in the original repo's working tree (or as local-only commits, depending on operator habit).
- On unprotected master those leftovers get auto-committed and pushed directly. On protected master direct push is rejected, so the operator is left with a dirty working tree that grows over time, has to be reconciled by hand, and is impossible to keep clean using dark-factory alone.
- This document describes the problem. It does NOT prescribe a solution. The solution surface is wide (admin-sync pipeline, separate auto-PR, daemon flag to bundle dirty admin into each prompt's PR, periodic janitor agent, etc.) and the trade-offs need their own design pass.

## Problem

dark-factory's lifecycle produces several distinct categories of filesystem writes against the project repo:

1. **Spec creation** — operator (or another agent) writes a spec to `specs/<name>.md`.
2. **Spec approve** — `dark-factory spec approve <id>` flips `status: idea|draft → approved` and renames `specs/<name>.md → specs/in-progress/<NNN>-<name>.md`.
3. **Prompt generation** — daemon writes one or more prompt files to `prompts/<NN>-<name>.md` based on the approved spec.
4. **Prompt approve** — `dark-factory prompt approve <id>` flips `status: draft → approved` and renames `prompts/<name>.md → prompts/in-progress/<NNN>-<name>.md`.
5. **Prompt work commit** — daemon's workflow executor commits code change + this prompt's `in-progress → completed` rename in a single combined commit (spec 086).
6. **Spec lifecycle transitions** — daemon auto-transitions specs `approved → prompted → verifying → completed` and moves the spec file across the matching subdirectories (`specs/in-progress/` → `specs/completed/`).
7. **CHANGELOG / docs updates** — written by individual prompts (in scope of their work commit) OR written by the operator outside any prompt.
8. **autoRelease bookkeeping** — version bump, tag push (incompatible with protected master regardless; out of scope for this idea).

For each workflow mode, only category 5 reliably lands on master via the PR. Today, categories 2, 3, 4, 6 happen as filesystem writes against the *original* repo's working tree. On unprotected master those changes get auto-committed and pushed when the daemon finishes a workflow (or via operator habit). On protected master:

- Direct push to master is rejected.
- A clone-workflow PR's base branch is `origin/<defaultBranch>`, which does NOT include the operator's local-only admin commits → the agent inside the clone may not even see the prompt file it is supposed to execute (the file exists only in the original's working tree, not on origin).
- Even when the prompt file IS on origin (operator pushed it via a manual PR before running the daemon), the next round of admin (spec/prompt status flips, new prompt generation) again diverges.

### Observed evidence

Working tree of `~/Documents/workspaces/dark-factory` after a normal day of spec/prompt iteration (current session, 2026-05-23):

```
$ git status
On branch master
Changes not staged for commit:
   modified:   CHANGELOG.md
   modified:   docs/workflows.md
   modified:   pkg/processor/workflow_executor_clone.go
   modified:   pkg/processor/workflow_executor_worktree.go
   modified:   pkg/processor/workflow_helpers.go
   modified:   prompts/in-progress/407-bug-087-sync-prompt-file-to-original-repo.md
```

Several of those modifications are admin (frontmatter flips, in-progress directory moves). On dark-factory's own repo (unprotected master) the operator manually commits them as bulk "update prompts/specs" commits. On a protected-master project that workflow is not available.

### Observable failure modes today

| Where | What happens | Why it matters |
|---|---|---|
| `dark-factory spec approve` on protected-master project | Spec file moves to `specs/in-progress/` locally, frontmatter status changes. Daemon never commits this — it's not part of any workflow's commit. | After running for a week, the operator has dozens of dirty admin files and no clean way to land them. |
| Daemon-generated prompts | Newly created prompt files in `prompts/` are not added to any commit. | Each generated prompt is "lost" until manually committed. |
| Spec lifecycle transitions (`prompted → verifying → completed`) | Daemon writes the frontmatter change to the spec file, then the spec auto-moves to `specs/completed/`. Neither change is committed by the daemon. | Operator has to bundle these into a manual PR periodically. |
| Workflow PR is created from `origin/<defaultBranch>` | If the prompt file (or its parent spec) lives only in the operator's local working tree, the clone never sees it. | dark-factory cannot run against a protected-master project unless the operator first manually PRs the admin state. |

### What "100% support" would mean

A protected-master project should be operable by `dark-factory` end-to-end with:

- Zero manual `git commit` invocations against master from the operator.
- Every byte written by dark-factory to the project repo eventually landing on master via at least one PR.
- Spec/prompt admin (categories 2, 3, 4, 6 above) reliably finding its way into either the prompt's own PR or a separate, dedicated admin PR.
- No assumption that the operator can push directly to master.

That is not the case today.

## Constraints (any future solution must respect)

- Direct push to master is unavailable. Anything that lands on master MUST go through a PR.
- The daemon may run unattended for extended periods. Admin must accumulate and land safely without operator intervention beyond merging PRs.
- The daemon's auto-commits MUST be reviewable. Bundling N hours of dirty admin into one PR with no narrative is not acceptable for a protected-master project where someone has to review every change.
- The spec-086 invariant (one combined commit per prompt: code + this prompt's rename) MUST NOT regress. Whatever solution we pick CANNOT bundle unrelated admin into the prompt's work commit — the commit must stay scoped to *that* prompt's work.
- The spec-087 invariant (the original repo's view matches `origin/master` after a successful clone/worktree push) MUST NOT regress.
- Branch-protection rules MUST continue to be respected — no new bypass paths.

## Solution surface (descriptive, not prescriptive)

This section enumerates DIRECTIONS, not a recommendation. Each has open questions a draft spec would have to answer.

- **Admin-sync prompt pattern.** Operator (or scheduled job) periodically queues a special prompt whose `<objective>` is "stage and commit all dirty spec/prompt admin files in the original repo". Daemon runs it like any other prompt; the resulting PR bundles admin into a single commit on a feature branch. Open: does this respect the spec-086 invariant? How is it scoped (everything dirty, or only specific paths)? How does the operator triage what's bundled?
- **Daemon flag to auto-bundle admin into the prompt's PR.** When `workflow: clone|branch|worktree` + `pr: true` and the original is dirty in admin paths, the daemon stages the admin alongside the work commit before pushing. Open: this regresses the spec-086 "one focused commit" invariant; how do reviewers distinguish work from admin? Does each admin file get its own commit in the PR, or one bundled admin commit?
- **Separate admin-sync agent on a cron.** A different process (not the workflow executor) periodically scans `~/Documents/workspaces/<project>/` for dirty admin paths and opens a PR via the `gh` CLI. Decoupled from any specific prompt's lifecycle. Open: how does it choose when to bundle (every N minutes? at the end of every prompt run? on git-status threshold)? What identity does it commit as?
- **Janitor prompt at end of each daemon shutdown.** Daemon shutdown hook queues a final admin-sync prompt before exit. Open: makes daemon shutdown side-effectful in a non-trivial way; what if the shutdown is unclean (SIGKILL)?
- **First-class spec/prompt operations through PRs.** Every `dark-factory spec approve` / `prompt approve` opens its own micro-PR for the lifecycle change. Open: PR-noise explosion (N admin PRs per work PR); CI cost; reviewer fatigue.

None of these are recommended yet. The right next step is a draft spec that picks ONE direction, names the trade-offs accepted, and writes ACs against it.

## Non-goals

- Choosing a solution. This is `status: idea` precisely because the solution surface is wide and the trade-offs are non-obvious.
- Fixing `autoRelease` for protected master — that is a separate, well-understood incompatibility (handled by the GitHub Release Agent pipeline; see `docs/workflows.md`).
- Bypassing branch protection. Out of scope for any future spec rooted in this idea.
- Changing how `dark-factory` numbers prompts/specs.

## Related

- `docs/workflows.md` — describes the workflow modes; the "protected master" callout there points back to this file.
- `specs/completed/086-bug-prompt-move-not-pushed.md` — established the one-combined-commit invariant per prompt.
- `specs/in-progress/087-bug-clone-worktree-move-not-applied-to-original.md` — established the local-view-matches-remote invariant for clone/worktree.
- GitHub Release Agent (separate pipeline) — handles release/tagging out-of-band for protected-master projects.
