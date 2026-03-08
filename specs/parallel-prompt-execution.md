---
status: draft
---

## Summary

- Prompts can declare dependencies on other prompts via frontmatter
- Independent prompts execute in parallel, each in its own git clone
- Dependent prompts wait until their prerequisite completes
- No dependency = run immediately (current behavior preserved for sequential projects)

## Problem

Dark-factory processes prompts strictly sequentially. A batch of 10 prompts where only 3 depend on each other takes 10x the time of one prompt. The 7 independent prompts could run in parallel.

## Goal

After this work, dark-factory builds a dependency graph from prompt frontmatter and executes independent prompts concurrently. Prompts with dependencies wait until their prerequisites complete successfully. A batch of 10 prompts with 3 sequential dependencies and 7 independent ones completes in ~4x (3 sequential + 1 parallel batch) instead of ~10x.

## Non-goals

- No automatic dependency detection (human declares dependencies)
- No cross-project parallelism (each project runs its own factory)
- No shared state between parallel prompts (each gets its own clone)
- No priority or scheduling beyond dependency ordering

## Assumptions

- Git clone (from spec 027) provides isolation per prompt
- Each clone gets its own Docker container
- System has enough resources (CPU, memory, API quota) for N concurrent containers
- Human correctly declares dependencies (no cycle detection beyond basic validation)

## Desired Behavior

1. Prompts declare dependencies via `depends` frontmatter field:
   ```yaml
   ---
   depends: ["003-add-interface"]
   ---
   ```
   Value is a list of prompt base names (without `.md`). Empty or missing = no dependencies.

2. On startup, dark-factory builds a DAG from all queued prompts and their `depends` fields.

3. Prompts with no dependencies (and no unmet dependencies) start immediately, each in its own git clone under `/tmp/dark-factory/`.

4. When a prompt completes successfully, its dependents become eligible. If all of a dependent's prerequisites are complete, it starts.

5. When a prompt fails, all prompts that depend on it (transitively) are marked `blocked` and skipped.

6. Circular dependencies are detected at queue time and reported as an error. No prompts execute.

7. Maximum concurrency is configurable via `maxParallel` in `.dark-factory.yaml` (default: 1 = current sequential behavior).

8. Each parallel prompt merges the completed changes from its prerequisites before starting (so it sees their code changes).

## Example

```
prompts/in-progress/
├── 001-add-interface.md          # depends: []
├── 002-implement-interface.md    # depends: ["001-add-interface"]
├── 003-add-tests.md              # depends: ["002-implement-interface"]
├── 004-update-docs.md            # depends: []
├── 005-fix-linting.md            # depends: []
├── 006-add-config.md             # depends: []
```

Execution timeline:
```
t=0: 001, 004, 005, 006 start (all independent)
t=1: 001 completes → 002 starts (004, 005, 006 may still run)
t=2: 002 completes → 003 starts
```

## Constraints

- `maxParallel: 1` must behave identically to current sequential mode
- `workflow: direct` with parallel needs careful merge strategy (all push to same branch)
- `workflow: pr` with parallel: each prompt gets its own PR (natural isolation)
- Existing prompts without `depends` field work unchanged (treated as no dependencies)

## Open Questions

- How do parallel `workflow: direct` prompts merge to master without conflicts?
- Should `maxParallel` auto-detect from available resources or always be explicit?
- Should blocked prompts auto-retry when the blocking prompt is fixed and requeued?
- Does the `depends` field reference prompt numbers (fragile) or names (stable)?
- How does this interact with `autoMerge`? Wait for all parallel PRs before merging?

## Do-Nothing Option

Keep sequential execution. For 10 prompts at ~2 min each, wait ~20 min instead of ~8 min. Acceptable for small batches. Becomes painful for large specs (15+ prompts) or when running multiple specs.
