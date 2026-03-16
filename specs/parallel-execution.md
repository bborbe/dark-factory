---
status: draft
---

## Summary

- The daemon executes up to N prompts simultaneously, each in its own isolated worktree/clone
- A new `concurrency` config field controls how many containers run at once (default: 1)
- Git operations (commit, push, move-to-completed) are serialized through a single mutex so parallel containers never race on git
- Prompts sharing a branch are never executed in parallel — they remain sequential to preserve cumulative changes
- Status tracking supports multiple prompts in `executing` state simultaneously
- With worktree mode, each container gets its own independent git branch and working copy — commit serialization becomes trivial since branches don't overlap

## Problem

Dark-factory processes prompts one at a time. A spec that generates 8 independent prompts (e.g., refactoring 8 separate services) takes 8x the wall-clock time of one prompt. Since each prompt runs in its own Docker container with its own worktree, there is no technical reason they cannot run in parallel. The bottleneck is container resource usage (CPU, memory), not git — so a configurable concurrency limit is the right control.

Real-world example: refactoring factory file organization across 40 trading services takes ~2 hours sequentially at ~3 min each. With concurrency=3, this drops to ~40 min.

## Goal

After this work, the daemon picks up to N queued prompts and runs them simultaneously in separate containers. When any container finishes, its git operations (commit, push, status update, file move) happen under a mutex so they never interleave. The system then picks the next queued prompt to fill the slot. With concurrency=3 and 40 independent prompts, wall-clock time drops from ~2h to ~40min.

## Non-goals

- No automatic dependency detection between prompts
- No cross-project parallelism (each project has its own daemon)
- No dynamic concurrency adjustment based on system resources
- No parallel execution within a single branch (same-branch prompts stay sequential)

## Future Enhancement: Dependency Graph

A future spec may add `depends` frontmatter for DAG-based scheduling:

```yaml
---
depends: ["003-add-interface"]
---
```

This would allow dependent prompts to wait for prerequisites, with transitively blocked prompts marked `blocked` on failure. This is out of scope for the initial implementation — concurrency with same-branch serialization covers the primary use case (independent service refactorings).

## Desired Behavior

1. **Config field**: `.dark-factory.yaml` accepts `concurrency: N` (integer, min 1, max 5, default 1). Values outside this range produce a validation error at startup.

2. **Prompt selection**: The daemon selects up to N eligible prompts from the queue. A prompt is eligible if no other prompt with the same `branch` value is currently executing. Prompts without a `branch` field are always eligible (they operate on the default branch, but each gets its own worktree so there is no conflict).

3. **Parallel container launch**: Each selected prompt gets its own worktree/clone and its own Docker container. Containers run independently and do not share filesystem state.

4. **Serialized git finalization**: When a container completes (success or failure), the daemon acquires a single project-wide mutex before performing any git operations: commit, push, PR creation, changelog update, branch merge, file move to completed directory. Only one finalization runs at a time.

5. **Slot refill**: After finalization releases the mutex, the daemon checks the queue and starts a new prompt if a slot is available and an eligible prompt exists.

6. **Multiple executing status**: The status tracking system supports multiple prompts in `executing` state at the same time. `dark-factory status` and `prompt list` display all currently executing prompts with their container names and durations.

7. **Logging isolation**: Each prompt's execution log goes to its own file in the log directory (already the case today — confirmed as an invariant).

8. **Backward compatibility**: `concurrency: 1` (the default) produces identical behavior to today's sequential execution. No new config field is required to maintain current behavior.

## Constraints

- Same-branch serialization is mandatory. If prompts A and B both have `branch: dark-factory/spec-028`, B must wait for A to complete before starting. This preserves the cumulative-changes guarantee.
- The instance lock remains a single lock per project. Parallel execution happens within one daemon instance, not across multiple daemons.
- The existing worktree/clone mechanism must be used for isolation. In-place execution (`worktree: false`) with concurrency > 1 must produce a validation error at startup — you cannot run multiple prompts in-place on the same repository.
- Container resource limits are outside scope — the user controls concurrency to match their machine's capacity.
- All existing tests must pass.
- `make precommit` must pass.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Container crashes mid-execution | Mutex is never held during container run, so other containers are unaffected. Failed prompt moves to failed state during finalization. | Requeue the failed prompt |
| Daemon crashes with N containers running | OS cleans up containers. On restart, prompts in `executing` state are detected and requeued (existing crash recovery). | Restart daemon |
| Git push conflict during finalization | Finalization fails for that prompt, others unaffected. Prompt marked failed with error details. | Requeue or manual resolution |
| All N slots filled with same-branch prompts queued behind them | Same-branch prompts wait. Other-branch prompts fill remaining slots. If no other-branch prompts exist, effective concurrency drops to 1 for that branch. | Expected behavior, not an error |
| `concurrency: 10` in config | Validation error at startup: max is 5 | User reduces value |
| `worktree: false` with `concurrency: 2` | Validation error at startup: parallel requires worktree isolation | User sets `worktree: true` or `concurrency: 1` |

## Acceptance Criteria

- [ ] `concurrency` field accepted in `.dark-factory.yaml` with validation (1-5, default 1)
- [ ] Daemon runs up to N containers simultaneously when eligible prompts exist
- [ ] Same-branch prompts are never executed in parallel
- [ ] Git finalization (commit, push, move) is serialized — never interleaved
- [ ] `worktree: false` with `concurrency > 1` produces a startup validation error
- [ ] `concurrency: 1` behaves identically to current sequential execution
- [ ] `dark-factory status` shows all currently executing prompts
- [ ] Failed containers do not block other executing containers
- [ ] Worktree cleanup happens for both successful and failed containers
- [ ] Crash recovery handles multiple `executing` prompts on restart
- [ ] `make precommit` passes

## Verification

```bash
make precommit
```

Manual verification:

1. Set `concurrency: 1`. Queue 3 prompts. Observe sequential execution (identical to today).
2. Set `concurrency: 3, worktree: true`. Queue 3 independent prompts (no branch or different branches). Observe all 3 start within seconds of each other.
3. Same as step 2, but two prompts share a branch. Observe: 2 start immediately, third waits for same-branch prompt to complete.
4. Set `worktree: false, concurrency: 2`. Observe: startup validation error.
5. Kill daemon while 2 containers running. Restart. Observe: both prompts requeued.
6. Run `dark-factory status` while 2 prompts execute. Observe: both shown as executing.

## Do-Nothing Option

Keep sequential execution. For 40 independent prompts at ~3 min each, wait ~2h instead of ~40 min (with concurrency=3). Acceptable for small specs (2-3 prompts). Becomes a real bottleneck for large refactoring specs that touch many independent services — the primary use case for dark-factory at scale.
