---
status: idea
tags:
  - dark-factory
  - spec
  - idea
---

## Summary

- The spec lifecycle auto-transition `prompted → verifying` (and onward to `completed`) is wired into `AutoCompleter.CheckAndComplete`, which is called ONLY from `workflow_executor_direct.go` and `workflow_executor_branch.go`.
- Clone and worktree executors do NOT call `CheckAndComplete`, so prompts that ran via `workflow: clone` or `workflow: worktree` never trigger the spec's prompted → verifying transition.
- The `dark-factory prompt complete <id>` CLI command (used for manual closure when a prompt has failed/partial status) also does NOT call `CheckAndComplete`, so manually-closed prompts never trigger the transition either.
- Net effect: a spec whose linked prompts were processed exclusively under clone/worktree (or were closed manually via `prompt complete`) will sit at `status: prompted` forever, even after every linked prompt is at `prompts/completed/`. The spec is invisible to `dark-factory:verify-spec` workflows that filter on `status: verifying`.
- This document describes the problem. It does NOT prescribe a solution. The right call could be (a) extend CheckAndComplete to clone+worktree executors, (b) hook the transition into `prompt complete`, (c) add a daemon background sweeper that re-checks specs whose prompts are all complete, or (d) some combination. Each has different trade-offs around when in the lifecycle the transition fires and how races between concurrent prompt completions are handled.

## Problem

`AutoCompleter.CheckAndComplete(ctx, specID)` (in `pkg/spec/spec.go`) does the lifecycle work:
1. Scans linked prompts for the spec.
2. If all prompts are `completed`, sets the spec's status to `verifying` and emits a `spec_verifying` notification.

This function is the only path through which `prompted → verifying` fires. Today it is invoked from exactly two call sites:

- `pkg/processor/workflow_executor_direct.go:90` — after `direct` workflow's work commit succeeds.
- `pkg/processor/workflow_executor_branch.go:130` — after `branch` workflow's work commit succeeds.

Neither `cloneWorkflowExecutor.Complete` nor `worktreeWorkflowExecutor.Complete` calls `CheckAndComplete`. Neither does the `dark-factory prompt complete <id>` CLI command.

### Observed evidence

Spec 087 (`bug-clone-worktree-move-not-applied-to-original`):

```
$ dark-factory spec show 087
File:    087-bug-clone-worktree-move-not-applied-to-original.md
Status:  prompted                       ← stuck here
Linked Prompts: 2/2                     ← both at prompts/completed/
```

Both linked prompts (407, 408) are at `prompts/completed/`. The condition for `verifying` is met by content; the transition simply never fired because neither prompt went through a direct/branch executor at completion time. Prompt 407 completed via `dark-factory prompt complete` (because the agent had reported `partial`). Prompt 408 also completed via `dark-factory prompt complete` (because the Mac crashed mid-execution and the work was finished by hand).

Compare to spec 086, which transitioned to `verifying` cleanly — its prompts (405, 406) ran through the direct executor's happy path which DOES call CheckAndComplete.

### Observable failure modes today

| Trigger | What happens | Why it matters |
|---|---|---|
| Spec's prompts run only via clone/worktree | Spec stays at `prompted` forever | `dark-factory:verify-spec` workflows don't pick the spec up; operator must manually flip frontmatter |
| Any linked prompt finished via `dark-factory prompt complete <id>` | Spec stays at `prompted` forever | Same as above; common on protected-master projects where clone is the natural workflow |
| Spec's prompts mix direct/branch + clone/worktree | Last prompt's executor determines whether transition fires | Non-deterministic spec status — depends on prompt ordering and which executor ran last |
| Operator restores a prompt from `completed/` back to `in-progress/` (manual recovery) | Spec stays at `verifying`/`completed` because the prompt's restoration doesn't re-evaluate the spec | Out-of-band recovery doesn't propagate to spec status |

### What "fully wired" would mean

A spec should reliably transition `prompted → verifying` as soon as ALL its linked prompts are at `prompts/completed/`, regardless of:
- Which workflow mode the prompts ran in (direct, branch, clone, worktree).
- Whether the prompts completed via the daemon's happy path or via `dark-factory prompt complete <id>`.
- Whether the daemon was running continuously or was restarted between prompt completions.

The check is cheap (scan a directory of files; load frontmatter). It can run defensively at multiple points in the lifecycle without correctness risk.

## Constraints (any future solution must respect)

- The transition MUST be idempotent — running CheckAndComplete on a spec that is already `verifying` or `completed` is a no-op (already implemented at `pkg/spec/spec.go:422-426`).
- The transition MUST NOT regress for direct/branch workflows — they currently work and any new code path must not introduce duplicate-transition races or notification noise.
- The transition MUST happen against the daemon's view of the original repo, not against an isolated clone/worktree (the spec file lives in the original; clone/worktree are disposable).
- No new external dependencies.
- The transition MUST NOT fire while a prompt is still mid-execution (e.g., during `executing` or `committing` status). It MUST only fire when ALL linked prompts are at `completed`.

## Solution surface (descriptive, not prescriptive)

This section enumerates DIRECTIONS, not a recommendation. Each has open questions a draft spec would have to answer.

- **Extend CheckAndComplete to clone/worktree.** Call `e.deps.AutoCompleter.CheckAndComplete(ctx, specID)` from both `cloneWorkflowExecutor.Complete` and `worktreeWorkflowExecutor.Complete` after the sync mirror runs. Open: in clone/worktree, the AutoCompleter's view of `prompts/completed/<id>.md` exists only AFTER the sync mirror has updated the original repo — does the call happen before or after that? (Probably after, to avoid races.)
- **Hook the transition into `dark-factory prompt complete`.** When the CLI command finishes the manual move-and-commit, also invoke CheckAndComplete for any spec linked to the prompt. Open: which AutoCompleter instance does the CLI access? It's a long-running daemon component; the CLI may have its own short-lived process that doesn't share the same instance.
- **Background sweeper.** A periodic daemon job that scans all specs in `specs/in-progress/`, computes the linked-prompt completion state, and fires the transition where needed. Open: scan frequency vs. cost; race with concurrent prompt completions.
- **Spec-watcher.** When the spec file's frontmatter is touched (e.g., a new linked prompt is generated), re-evaluate transition eligibility. Open: this requires a filesystem watcher with the right scope and ordering guarantees.
- **Hybrid:** "best effort" call from every executor + a periodic sweeper as the safety net. Most robust but also the largest change.

The right next step is a draft spec that picks ONE direction (or hybrid), names the trade-offs, and writes ACs against it.

## Non-goals

- Choosing a solution. This is `status: idea` because the solution surface is wide and the trade-offs are non-obvious.
- Changing the `prompted → verifying → completed` state machine itself. It is correct; only the trigger surface is incomplete.
- Adding new spec lifecycle states or merging existing ones.
- Auto-transitioning `verifying → completed` (separate concern; today that step is intentionally human-gated per `docs/spec-verification.md`).

## Related

- `docs/spec-verification.md` — describes the verification workflow that depends on `status: verifying` to surface specs needing review.
- `specs/completed/086-bug-prompt-move-not-pushed.md` — its prompts ran direct → transitioned cleanly; demonstrates the happy path.
- `specs/in-progress/087-bug-clone-worktree-move-not-applied-to-original.md` — its prompts completed via `dark-factory prompt complete` → stuck at `prompted` until manual frontmatter flip. The motivating example for this idea.
- `pkg/spec/spec.go` — `AutoCompleter` interface + `MarkVerifying`.
- `pkg/processor/workflow_executor_direct.go:90` and `pkg/processor/workflow_executor_branch.go:130` — the only two CheckAndComplete call sites today.
