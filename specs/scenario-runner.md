---
status: idea
---

## Summary

- Dark-factory automatically runs all scenarios after spec completion
- Scenario runner executes each scenario in a YOLO container and reports pass/fail
- Scenario failure blocks spec from reaching `completed` status
- Docker mount excludes `scenarios/` during prompt execution (holdout isolation)

## Problem

Scenarios exist as manual checklists — a human reads the markdown and follows the steps. This doesn't scale: as scenario count grows, manual verification becomes slow, error-prone, and easy to skip. The value of scenarios as a regression suite depends on actually running them.

## Goal

Automated scenario execution that produces a pass/fail signal for the entire system after any change.

## Non-goals

- Not changing the scenario format — scenarios stay as markdown checklists
- Not LLM-as-judge — boolean pass/fail only
- Not replacing `make precommit` — scenarios complement existing checks

## Desired Behavior

1. `dark-factory scenario run` executes all scenarios in `scenarios/` that have no `status: idea` frontmatter.
2. Each scenario runs in a YOLO container with full workspace access (including `scenarios/`).
3. Runner reports pass/fail per scenario and overall pass/fail.
4. During prompt execution, `scenarios/` is excluded from the Docker mount (holdout isolation).
5. After spec completion, scenario runner executes automatically. Failure blocks spec from `completed`.
6. Adding a new scenario requires no code changes — just a new file in `scenarios/`.

## Isolation — the critical constraint

The implementation agent must never see scenario files during prompt execution. If it can read them, it will optimize for them (overfit).

| Phase | Scenarios dir mounted? |
|-------|----------------------|
| Prompt execution (implement) | **No** — excluded from mount |
| Scenario validation (verify) | **Yes** — full workspace |

Enforced at the container level, not by convention.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Scenario fails after spec completion | Spec stays in `verifying`, blocked | Fix implementation or adjust scenario |
| Implementation agent accesses scenarios | Overfitting risk | Fix Docker mount exclusion |
| New feature breaks old scenario | Regression detected | Fix regression before completing spec |

## Acceptance Criteria

- [ ] `dark-factory scenario run` executes all non-idea scenarios
- [ ] `scenarios/` excluded from Docker mount during prompt execution
- [ ] Scenario runner reports per-scenario and overall pass/fail
- [ ] Scenario failure blocks spec completion
- [ ] Adding a new scenario requires no code changes
- [ ] `make precommit` passes

## Do-Nothing Option

Keep manual scenario execution. Human reads checklist, follows steps, checks boxes. Works at 3 scenarios, won't scale to 10+.
