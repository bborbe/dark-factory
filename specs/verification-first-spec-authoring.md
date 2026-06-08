---
tags:
  - dark-factory
  - spec
status: draft
---

## Summary

- Dark-factory enforces verification quality at *audit* time but not at *authoring* time — `spec-creator` walks the author top-down Goal → Desired Behavior → Acceptance Criteria.
- This spec reorders both the `spec-creator` `<spec_template>` block AND `<workflow>` step 2 so the author writes Goal → **Acceptance Criteria + Verification** → Desired Behavior — proof before behavior.
- `docs/rules/spec-writing.md` gets a guide-level statement of the verification-first ordering principle in its intro, surfacing what is currently buried in the per-AC evidence-shape subsection.
- Recursive demonstration: this spec itself is authored in the new order. Section order below is the new order.
- Auditor (`spec-auditor.md`) checks section *presence*, not order — reorder does not require auditor changes.

## Problem

The mature audit-time discipline in dark-factory (`Evidence Shape per AC` table, `Adversarial Laziness Test`, `Hedge Words to Avoid`, YAGNI pass, "Every AC must declare evidence shape" hard constraint in `spec-creator`) catches under-specified ACs *after* the spec is drafted. The authoring-time order is bottom-up — the author writes behaviors first, then thinks about how to verify them, then runs adversarial-laziness as a last-pass corrective. An author can write 8 fully shaped ACs that measure the *wrong observable*; evidence shape catches "vague," not "wrong target." Reordering so the proof drives the behavior surfaces wrong-target specs at write time, before the auditor sees them and before any prompt-creator wastes research cycles.

## Goal

After this work is done, the `spec-creator` `<spec_template>` and `<workflow>` order Goal → **AC (with evidence shapes)** → **Verification** → Desired Behavior → rest. `docs/rules/spec-writing.md` opens its "Spec Structure" section with the verification-first ordering principle. New specs authored via `/dark-factory:create-spec` ask the verification question between "what end state" and "what behaviors" — proof comes before behavior in every fresh spec authored after this change ships.

## Non-goals

- Reorganizing the *content* of the Verification section (the 12-shape evidence table, post-deploy markers, deploy-check semantics). Order changes, content stays.
- Adding per-AC role categorization (functional / tests / quality / regression / operational). Possibly a follow-up if the reorder alone doesn't close the wrong-target gap.
- Changing `spec-auditor.md` enforcement rules — the auditor's existing checks already cover evidence-shape compliance.
- Changing `spec-verifier.md` or the `verify-spec` command — verification execution path unchanged.
- Touching `prompt-writing.md` or `scenario-writing.md` — downstream of specs; discipline propagates transitively without their text changing.
- Plan-task (vault-cli) parity — that's [[Plan-Task Verification Design Gate]]'s territory; tracked separately.

## Acceptance Criteria

- [ ] `git diff agents/spec-creator.md` shows the `<spec_template>` block with `## Acceptance Criteria` and `## Verification` sections placed immediately after `## Non-goals` and before `## Desired Behavior` — evidence: `grep -n '^## ' agents/spec-creator.md` (within the `<spec_template>` heredoc) returns the section headers in order `Summary → Problem → Goal → Non-goals → Acceptance Criteria → Verification → Desired Behavior → Constraints → Failure Modes → Security / Abuse Cases → Suggested Decomposition → Do-Nothing Option`.
- [ ] `git diff agents/spec-creator.md` shows `<workflow>` step 2 (Gather requirements) with an explicit verification-ask bullet between "what end state" and "what behaviors / what can go wrong" — evidence: `grep -nB1 -A4 'Gather requirements' agents/spec-creator.md` returns a bullet matching `For that goal, what observable proof would convince you it's reached? Each proof needs an evidence shape`.
- [ ] `git diff docs/rules/spec-writing.md` shows the verification-first ordering principle in the guide's "Spec Structure" section preamble — evidence: `grep -n 'Write the proof before the behavior' docs/rules/spec-writing.md` returns line ≥1.
- [ ] **Recursive demonstration**: the spec at `specs/verification-first-spec-authoring.md` (this file) opens with the new section order — evidence: `awk '/^## /{print NR": "$0}' specs/verification-first-spec-authoring.md` returns `## Summary`, `## Problem`, `## Goal`, `## Non-goals`, `## Acceptance Criteria`, `## Verification`, `## Desired Behavior` in that line order with no other section preceding `## Acceptance Criteria`.
- [ ] **Post-Deploy (Rung-2):** `/dark-factory:create-spec` invoked in `~/.claude-verify` (isolated env, dark-factory marketplace clone on `feat/verification-first-spec-authoring`, cache cleared) against a fresh fixture goal asks the verification question before the desired-behavior question — evidence: stream-json output captures the AskUserQuestion sequence with the verification question appearing before any desired-behavior question.
    - `deploy_check:` `git -C ~/.claude-verify/plugins/marketplaces/dark-factory rev-parse HEAD`
    - `deploy_target:` `$(git rev-parse HEAD)`
- [ ] `/dark-factory:audit-spec specs/verification-first-spec-authoring.md` (this file, run against the reordered spec) reports score ≥ 8 with no Critical Issues — evidence: stdout contains `**Score**: [89]/10` (or `**Score**: 10/10`) and stdout does NOT contain `## Critical Issues` followed by a non-empty body. Confirms the new section order does not trip any existing auditor rule.

## Verification

```bash
# Artifact checks (run from worktree root)
grep -nB1 'Acceptance Criteria' agents/spec-creator.md | head -5    # AC section appears before Desired Behavior in template
grep -nB1 -A4 'Gather requirements' agents/spec-creator.md          # workflow step 2 has the verification ask
grep -n 'Write the proof before the behavior' docs/rules/spec-writing.md
awk '/^## /{print NR": "$0}' specs/verification-first-spec-authoring.md

# Behavioral check (Post-Deploy — Rung-2)
CLAUDE_CONFIG_DIR=~/.claude-verify \
  claude -p '/dark-factory:create-spec fixture goal: a tiny logging field' \
  --output-format=stream-json --verbose --permission-mode=acceptEdits \
  2>&1 | jq -c 'select(.type == "assistant") | .message.content // [] | map(select(.type == "tool_use" and .name == "AskUserQuestion") | .input.questions[0].question) | .[]'
# Expected: the verification question ("What observable proof would convince you...") appears before any desired-behavior question

# Auditor recursive check
/dark-factory:audit-spec specs/verification-first-spec-authoring.md
# Expected: Score ≥ 8, no Critical Issues
```

## Desired Behavior

1. The `spec-creator` agent's `<spec_template>` heredoc emits sections in the new order: Summary → Problem → Goal → Non-goals → Acceptance Criteria → Verification → Desired Behavior → Constraints → Failure Modes → Security / Abuse Cases → Suggested Decomposition → Do-Nothing Option. Human-narrative top-of-spec (Summary / Problem / Goal / Non-goals) stays; verification promotes from the end of the document to immediately after Non-goals.
2. The `spec-creator` agent's `<workflow>` step 2 (`Gather requirements`) inserts a new bullet between "What should the end state look like?" and "What must NOT change?" — explicit: *"For that goal, what observable proof would convince you it's reached? Each proof needs an evidence shape — exit code / log line / file diff / HTTP status / kafka message / metric / cluster state."* The author writes Acceptance Criteria before Desired Behavior.
3. `docs/rules/spec-writing.md` opens its `## Spec Structure` section with a one-paragraph statement of the verification-first ordering principle: *"Write the proof before the behavior. If you can't describe how you'd verify the goal, you don't yet know what the spec is asking for."* The principle is surfaced at guide-level before the section-by-section template is presented.
4. The spec at `specs/verification-first-spec-authoring.md` (this file) demonstrates the new order live — section headings appear in the new sequence, with `## Acceptance Criteria` and `## Verification` placed before `## Desired Behavior`.

## Constraints

- The 12-shape `Evidence Shape per Acceptance Criterion` table in `docs/rules/spec-writing.md` MUST NOT change — same shapes, same examples, same Post-Deploy marker rules.
- The Adversarial Laziness Test, Hedge Words to Avoid, YAGNI Pass, and Preflight Checklist sections in `docs/rules/spec-writing.md` MUST NOT lose content — they may be referenced from the new intro paragraph, but their existing depth stays intact.
- `spec-auditor.md` enforcement rules MUST NOT change — the auditor's "Required Sections" list is presence-based and order-agnostic; existing in-progress and completed specs (authored in the old order) must continue to pass audit after this change ships.
- `spec-verifier.md` and `verify-spec.md` MUST NOT change — verification execution path is unaffected.
- The `<spec_template>` heredoc inside `agents/spec-creator.md` MUST remain syntactically valid markdown that renders as the new template when read by Claude Code at session load.
- All existing slash commands (`/dark-factory:create-spec`, `/dark-factory:audit-spec`, `/dark-factory:verify-spec`, `/dark-factory:generate-prompts-for-spec`) MUST continue to function identically; only the authoring-question order and template output change.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| `spec-creator` agent emits the reordered template but Claude Code's session cache still serves the old version after merge | New specs authored in the same session use the old order until the user restarts Claude Code (cache is version-keyed per `[[Verify Claude Code Marketplace Plugin Before Merge]]`). | Bump the dark-factory plugin version (CHANGELOG `## Unreleased` → tagged release) so the marketplace cache refreshes; or instruct users to `rm -rf ~/.claude/plugins/cache/dark-factory` between sessions. |
| Author manually writes Desired Behavior before Acceptance Criteria despite the new workflow asking for proof first | The spec is structurally still valid (sections-present), but loses the verification-first authoring benefit. Auditor catches missing evidence shapes downstream. | The reordered workflow asks the verification question first; if the author types behavior before answering the verification prompt, the spec-creator agent surfaces the gap in the report's preflight check ("Q7: How will we know it's done?"). |
| Existing in-progress specs (authored in the old order) continue execution but their template ordering doesn't match the new convention | No effect on behavior — the auditor accepts both orders (presence-based check). | No recovery needed; legacy specs remain valid. New specs use the new order. |
| The recursive-demonstration spec (this file) fails `/dark-factory:audit-spec` because some auditor rule we didn't anticipate triggers on the reordered structure | The reorder is silently breaking existing audit gates. | Stop merge; investigate which auditor rule trips; either fix the spec's content (preferred) or add an explicit allowance in the auditor (rejected — auditor is out of scope per Non-goals). |
| The `/dark-factory:create-spec` invocation in `~/.claude-verify` does not pick up the new order (cache not cleared, marketplace not on feature branch, etc.) | E2E verification falsely passes or falsely fails. | Re-follow [[Verify Claude Code Marketplace Plugin Before Merge]] step-by-step; confirm `git -C ~/.claude-verify/plugins/marketplaces/dark-factory branch --show-current` is the feature branch and the cache directory was deleted. |

## Security / Abuse Cases

Not applicable — change is markdown-only, no HTTP / file / user-input surface introduced. Existing security boundaries (write credentials in containers, audit gates on spec content) unaffected.

## Suggested Decomposition

Single-layer change, one prompt sufficient. Skip the Suggested Decomposition table.

## Do-Nothing Option

If we do nothing, the audit-time enforcement continues catching under-specified ACs after authoring — current pain point: occasional wrong-target ACs that are technically shaped correctly but measure the wrong observable; these slip through to prompt generation and surface only at `spec-verifier` execution time, where rework is most expensive. Current state is acceptable in absolute terms (the safety nets work), but each wrong-target spec costs ~30-60 min of rework that verification-first authoring would prevent at the 5-minute spec-draft stage. The cost compounds with spec volume.
