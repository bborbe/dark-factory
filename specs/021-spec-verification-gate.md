---
status: completed
---

# Spec Verification Gate

## Problem

Specs auto-complete today when all linked prompts are merged. There is no check that the built system actually matches what the spec described. The `## Verification` section in every spec is advisory text that nobody runs — the spec closes silently regardless of whether acceptance criteria were met.

This leaves the human out of the loop at the end of the pipeline, with no structured moment to evaluate outcomes or plan follow-up work.

## Goal

When all linked prompts are merged, the spec transitions to `verifying` instead of `completed`. The human runs the verification commands, checks the acceptance criteria, and explicitly closes the spec with `dark-factory spec verify`. This creates a clean Level 4 workflow: human defines intent at the start, evaluates outcomes at the end.

## Non-goals

- No automated running of verification commands — human runs them manually
- No change to prompt lifecycle
- No automated follow-up spec creation

## Desired Behavior

1. When all linked prompts reach `completed`, the spec transitions to `verifying` (not `completed`).
2. `dark-factory spec list` shows `verifying` specs prominently — they require human attention.
3. `dark-factory spec verify <file>` transitions the spec from `verifying` to `completed`.
4. Running `spec verify` on a spec that is not `verifying` returns a clear error.
5. Specs without any linked prompts are unaffected — they still auto-complete as today.

## Constraints

- Existing `spec approve` behavior unchanged
- Specs with no linked prompts complete automatically as before (behavior 5)
- `make precommit` must pass

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Human runs `spec verify` before all prompts done | Error: "spec is not in verifying state" | Wait for all prompts to complete |
| Human decides spec failed verification | Leave spec in `verifying`, open new spec for fixes | Manual |

## Acceptance Criteria

- [ ] All prompts merged → spec transitions to `verifying` not `completed`
- [ ] `dark-factory spec list` shows `verifying` status
- [ ] `dark-factory spec verify <file>` transitions `verifying` → `completed`
- [ ] `spec verify` on wrong state returns clear error
- [ ] Specs with no linked prompts still auto-complete
- [ ] `make precommit` passes

## Verification

```
dark-factory spec list        # verifying specs visible
dark-factory spec verify 021-spec-verification-gate.md
dark-factory spec list        # 021 shows completed
```

## Do-Nothing Option

Keep auto-completing specs on prompt merge. Simpler, but no human evaluation gate at the end. Stays at Level 3 — human approves PRs but never explicitly evaluates whether the spec goal was met.
