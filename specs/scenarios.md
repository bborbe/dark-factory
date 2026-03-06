---
status: idea
---

# Scenarios: Holdout Validation via Markdown

## Idea

Scenarios are behavioral validation files written by the human from the spec. They run in a YOLO container after all prompts for a spec complete — same infrastructure as prompts, different purpose: verify, not implement.

The implementation agent never sees scenario files. This is the holdout set principle from ML: if the agent trains on its own tests, it overfits.

## Format

A scenario is a markdown file with a goal and verification steps — like a prompt, but read-only. The agent checks behavior and exits 0 (pass) or non-zero (fail).

```markdown
# Scenario: inbox file is never modified by factory

## Goal

Verify that dark-factory never modifies files in the inbox directory.

## Steps

1. Check that no file in `prompts/` (inbox) has a status other than `created` or no status
2. Verify that inbox files are not moved or renamed by the watcher
3. Confirm that only files in `prompts/in-progress/` are actively processed

## Pass Condition

All inbox files untouched. Exit 0.

## Fail Condition

Any inbox file modified, moved, or renamed by dark-factory. Exit non-zero with explanation.
```

## Where scenarios live

```
specs/
  scenarios/
    024-*.md    ← scenario files for spec 024
    025-*.md    ← scenario files for spec 025
```

Naming mirrors spec number. One spec → one or more scenarios.

## Isolation — the critical constraint

The implementation agent must never have access to scenario files. If it can read them, it will optimize for them (overfit) rather than solving the actual problem. This is the holdout set principle.

Dark-factory controls the Docker mount. Two container modes:

| Phase | Scenarios dir mounted? |
|-------|----------------------|
| Prompt execution (implement) | **No** — excluded from mount |
| Scenario validation (verify) | **Yes** — full workspace |

This exclusion must be enforced by dark-factory at the container level, not by convention. Relying on the agent "not looking" is not sufficient.

## Pipeline integration

```
prompted → all prompts completed → run scenarios → pass → completed
                                                 → fail → verifying (blocked, needs attention)
```

The scenario runner is a separate step between `prompted` and `completed`. It uses the same YOLO container as prompts. On failure, the spec stays in `verifying` with a scenario log entry.

## Why markdown over shell scripts

- More expressive — can reason about behavior, not just check exit codes
- Can read and understand code, not just run commands
- Same infrastructure as prompts — no new tooling
- LLM-as-judge built in — probabilistic validation for behaviors hard to check mechanically
- Human-readable — scenarios double as documentation of expected behavior

## Related

- [[Dark Factory - Scenarios as Holdout Validation]]
