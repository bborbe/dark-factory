---
status: draft
---

## Summary

- Scenarios are end-to-end paths through the system that describe how dark-factory behaves as a whole
- Each scenario is a complete user journey — from setup through execution to verifiable outcome
- Scenarios accumulate over time — each new feature adds scenarios, old ones never disappear
- Together, all scenarios form the living behavioral contract of the system
- The implementation agent never sees scenarios — holdout set principle prevents overfitting
- Scenarios run in a YOLO container after spec completion, using the same infrastructure as prompts

## Problem

Dark-factory's specs and prompts are immutable snapshots — they describe what was intended at a point in time. Nothing describes how the current system behaves as a whole. As the system grows, it becomes hard to verify that new features don't break existing behavior. There is no regression suite and no single source of truth for "what does dark-factory actually do right now?"

## Goal

A cumulative set of end-to-end scenarios that describe the complete behavior of dark-factory. Each scenario exercises a real user journey from start to finish. Running all scenarios after any change produces a pass/fail signal for the entire system. Reading the scenario list is equivalent to reading the system documentation.

## Non-goals

- Not unit tests — scenarios don't test individual functions or structs
- Not per-spec validation — scenarios describe system behavior, not feature acceptance criteria
- Not a replacement for `make precommit` — scenarios complement, not replace, existing checks
- No LLM-as-judge in the initial implementation — boolean pass/fail only

## Assumptions

- A YOLO container can execute scenarios the same way it executes prompts
- Scenarios can create temporary git repos, run dark-factory commands, and verify outcomes
- The implementation agent has no access to scenario files during prompt execution (enforced by Docker mount exclusion)

## Desired Behavior

1. Scenarios live in a dedicated directory (e.g. `scenarios/`) separate from specs and prompts.
2. Each scenario is a markdown file describing a complete end-to-end path through the system — setup, action, and verifiable outcome.
3. Scenarios are ordered by complexity — simpler scenarios first, complex ones build on earlier ones.
4. Scenarios accumulate — adding a new feature means adding new scenarios, never removing old ones.
5. Running all scenarios produces a pass/fail for each, plus an overall pass/fail for the system.
6. The implementation agent never sees scenario files — dark-factory excludes the scenarios directory from the Docker mount during prompt execution.
7. Scenario execution uses the same YOLO container infrastructure as prompts — no new tooling required.
8. A scenario failure after spec completion blocks the spec from reaching `completed` status.

## Example Scenario Progression

Each scenario is a complete path, not a fragment:

| # | Scenario | What it proves |
|---|----------|---------------|
| 1 | One prompt, direct workflow → new tag in repo | Basic pipeline works end-to-end |
| 2 | Custom config directories → prompts process from non-default folders | Config system works |
| 3 | PR workflow → prompt creates branch, opens PR | Workflow modes work |
| 4 | Three prompts execute in dependency order, all complete | Queue ordering works |
| 5 | Approve spec → prompts auto-generated → executed → spec completes | Spec-to-completion lifecycle works |
| 6 | Prompt with verification gate → pauses → human verifies → completes | Verification gate works |
| 7 | Failed prompt → requeue → succeeds on retry | Error recovery works |
| 8 | PR review comment → fix prompt auto-generated → executed → PR updated | Review-fix loop works |

Scenario 1 is the foundation. Every subsequent scenario implicitly depends on scenario 1 passing — if the basic pipeline is broken, nothing else matters.

## Scenario Format

A scenario describes observable behavior, not implementation detail:

```markdown
# Scenario: Basic direct workflow produces a tagged release

## Setup

- Initialize a git repo with a CHANGELOG.md and a Makefile with `precommit` target
- Create `.dark-factory.yaml` with `workflow: direct`
- Place one prompt file in `prompts/` inbox

## Action

- Run `dark-factory run` (or simulate the daemon lifecycle)

## Expected Outcome

- Prompt moved from `prompts/` to `prompts/in-progress/` to `prompts/completed/`
- A new git tag exists matching the version in CHANGELOG.md
- The completed prompt has `status: completed` in frontmatter
- The git log contains a commit with the prompt's changes

## Pass Condition

All expected outcomes verified. Exit 0.

## Fail Condition

Any expected outcome not met. Exit non-zero with explanation of which check failed.
```

## Isolation — the critical constraint

The implementation agent must never have access to scenario files. If it can read them, it will optimize for them (overfit) rather than solving the actual problem. This is the holdout set principle from ML.

Dark-factory controls the Docker mount:

| Phase | Scenarios dir mounted? |
|-------|----------------------|
| Prompt execution (implement) | **No** — excluded from mount |
| Scenario validation (verify) | **Yes** — full workspace |

This exclusion must be enforced at the container level, not by convention.

## Constraints

- Scenarios are additive only — never delete or modify a passing scenario
- Scenario files are human-written, never generated by the implementation agent
- Scenarios test observable behavior (files, git state, CLI output), never internal implementation
- Existing `make precommit` and prompt verification continue unchanged
- `make precommit` must pass

## Security

No external input surface — scenarios are authored by the project owner and stored in the repo. The Docker mount exclusion is the only security-relevant mechanism (prevents data leakage to the implementation agent).

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Scenario fails after spec completion | Spec stays in `verifying`, blocked | Fix implementation or adjust scenario |
| Scenario references internal implementation detail | Scenario is brittle, breaks on refactors | Rewrite scenario to test observable behavior only |
| Implementation agent somehow accesses scenarios | Overfitting risk — agent optimizes for scenario checks | Fix Docker mount exclusion |
| New feature breaks an old scenario | Regression detected — exactly what scenarios are for | Fix the regression before completing the spec |

## Acceptance Criteria

- [ ] Scenarios directory exists, excluded from implementation agent's Docker mount
- [ ] At least 3 scenarios covering basic direct workflow, config, and multi-prompt ordering
- [ ] Scenario runner executes all scenarios in a YOLO container and reports pass/fail
- [ ] Scenario failure blocks spec completion
- [ ] `dark-factory prompt list` / `spec list` unaffected by scenario files
- [ ] Adding a new scenario does not require code changes to dark-factory
- [ ] `make precommit` passes

## Verification

```
make precommit
```

## Do-Nothing Option

Keep current behavior. Rely on `make precommit` and manual testing. No regression suite — human must remember what features exist and verify them manually after changes. As the system grows, this becomes increasingly error-prone and slow.
