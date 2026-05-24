---
tags:
  - dark-factory
  - spec
status: draft
---

## Summary

- Five dark-factory agent files (spec-creator, spec-auditor, prompt-creator, prompt-auditor, spec-verifier) consistently drift from the Agent & Command Development Guide that the `coding:audit-agent` tool checks against.
- Each gap is individually minor — missing `color`, `<workflow>` instead of `<critical_workflow>`, no `<contextual_judgment>`, no `<success_criteria>`, an undocumented `effort: high` frontmatter field, audit-agent `<final_step>` blocks with three options instead of four.
- Together they make the fleet look inconsistent with broader Claude Code agent conventions, raise the onboarding cost for new contributors, and make cross-agent diff review noisier than necessary.
- This work brings all five files into structural alignment with the guide while preserving every existing constraint, workflow step, and evaluation criterion verbatim.
- Either documents `effort: high` as an explicit dark-factory extension or removes it — no other path leaves a stable answer for the next auditor run.

## Problem

The Agent & Command Development Guide that ships with the coding plugin defines a canonical structure for Claude Code agents: frontmatter fields including `color`, XML tags including `<critical_workflow>` / `<success_criteria>` / `<contextual_judgment>` (for oversight agents), and at least four user-interaction options in `<final_step>`. Audits run via `coding:audit-agent` against the four creator/auditor agents on 2026-05-23 surface the same seven gaps on every file. Each gap on its own is a one-line fix; together they describe a fleet that has drifted from its broader ecosystem. Every new agent added to dark-factory now faces the choice between matching the existing inconsistent style or matching the guide, and every re-audit re-flags the same recommendations. The cost is ongoing diagnostic noise, not a single broken behavior.

## Goal

After this work, every dark-factory agent file is structurally aligned with the Agent & Command Development Guide. `coding:audit-agent` reports zero critical structural findings against each file and scores each at 8/10 or higher. Dark-factory-specific frontmatter extensions are either adopted from the guide, documented under `docs/` with a one-line note explaining the deviation, or removed if no longer used. Cosmetic ecosystem conventions that the guide marks as advisory (e.g. the `-assistant` filename suffix) remain as-is — established names like `spec-auditor` already deviate by design across the broader ecosystem.

## Non-goals

- Does NOT change any agent's role definition, constraint content, workflow step content, evaluation criteria, severity definitions, or report-format checkboxes — only structural metadata, canonical tag names, and the four-option `<final_step>` expansion are in scope.
- Does NOT rename agent files to add the `-assistant` suffix — the guide marks this convention as advisory; established names like `spec-auditor` / `prompt-auditor` already deviate by design and renaming them would break every existing `Task` invocation that targets them by name.
- Does NOT add a new lint job or CI hook to enforce these conventions going forward — if that's wanted, it's a separate spec.
- Does NOT re-litigate the YAGNI pass logic already shipped in v0.168.4 + commit `143a200`.
- Does NOT merge or restructure the audit reports' Recommendation 5 (the `<final_step>` option count) at the expense of weakening the semantics of the existing three options — the 4th option must add real value or it does not get added.
- Does NOT update the broader Agent & Command Development Guide itself — alignment goes in one direction only (dark-factory → guide).
- Do NOT add `<knob>` to make this fleet alignment opt-out-able — invariant; if a future consumer demands variation per-agent, that's a separate spec.

## Desired Behavior

1. Every agent file under `agents/*.md` carries a `color` field in its frontmatter, with the value picked per the guide's conventions: `yellow` for analysis/audit agents (spec-auditor, prompt-auditor, scenario-auditor, spec-verifier), `blue` for generation/documentation agents (spec-creator, prompt-creator).
2. Every agent file with an ordered multi-phase workflow uses the canonical `<critical_workflow>` tag instead of `<workflow>`. Single-step or non-ordered workflows may use a different tag if the guide names a better-fitting one.
3. Every oversight/audit agent file (spec-auditor, prompt-auditor, scenario-auditor, spec-verifier) contains a `<contextual_judgment>` block explaining how strictness scales with input complexity — at minimum naming the two endpoints (trivial single-file change vs multi-prompt architectural spec).
4. Every agent file contains a `<success_criteria>` block defining when the agent's run is considered complete and successful, 2-4 bullets each.
5. The `effort: high` frontmatter field is resolved one of two ways: documented in `docs/dark-factory-agent-conventions.md` (created for this purpose) as an explicit dark-factory-specific extension with named semantics, OR removed from every agent file. No mixed state.
6. Auditor-style `<final_step>` blocks in spec-auditor and prompt-auditor offer at least four user-interaction options, and the fourth option carries distinct semantics from the existing three (e.g. "show before/after examples" or "explain a specific area in detail") — not a synonym of an existing option.
7. After the changes, an audit-agent run against each of the five files reports `None.` under Critical Issues, and each file's report header reads `Score: 8/10`, `Score: 9/10`, or `Score: 10/10`.

## Constraints

- All edits via the `Edit` tool, preserving exact existing formatting. No wholesale rewrites; no reflow of unrelated text.
- Every existing constraint bullet, workflow step text, evaluation criterion, scoring adjustment, and report-format checkbox stays verbatim. Only metadata is added and tag names are renamed.
- The BSD-style license header on every modified file (if present today) must survive the edit; verify on each file after editing.
- A `CHANGELOG.md` entry is added under `## Unreleased` describing the alignment work as a single bullet, referencing this spec.
- Apply the change set across all five agent files in a single coherent change; partial alignment defeats the purpose and leaves the auditor with mixed signal.
- The decision on `effort: high` (document vs remove) is made once for the whole fleet — both branches are acceptable; the mixed state is not.
- If `effort: high` is removed: every agent file loses the field in the same change; no agent ends up with it while others don't.
- If `effort: high` is documented: the new doc lives at `docs/dark-factory-agent-conventions.md`, is referenced from at least one existing doc in `docs/` (so it's discoverable), and names the runtime semantics (does dark-factory's daemon read it? does it affect token budget? does it affect retry behavior? — if none, that's the answer the doc records).
- The `<final_step>` 4th option, where added, must be load-bearing: an audit-agent run AFTER the change must classify the option as distinct from the existing three. A re-flag of "still only 3 distinct options" by the auditor means the change failed.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection |
|---------|-------------------|----------|-----------|
| Renaming `<workflow>` to `<critical_workflow>` breaks an external parser | Out of scope: dark-factory's own runtime is tag-agnostic and Claude Code reads agent files as freeform context. No parser exists. | Revert the specific rename if a regression appears post-change. | Future agent invocation produces unexpected behavior; bisect against the rename commit. |
| `color` value chosen wrong (e.g. `blue` assigned to an auditor) | Cosmetic only; `coding:audit-agent` flags as Recommendation on next run. | Operator changes the value per audit guidance. | Re-run `coding:audit-agent`; report cites the wrong value with the correct one suggested. |
| `<contextual_judgment>` content too vague to be actionable | Audit may pass structurally but agents don't actually use the guidance at runtime. | Iterate over time as real audit findings expose the gap. | Operator observes auditor over- or under-flagging complex specs after the change ships. |
| `effort: high` removed but dark-factory runtime expects the field | Possible warning in dark-factory logs at agent invocation; or silent no-op if the field was never read. Verify before removal. | Re-add the field to all five agents; document the semantics in the new doc instead. | `dark-factory` startup or per-agent invocation logs include a warning about the missing field; OR agent invocation behavior measurably changes. |
| New `<success_criteria>` block conflicts with an existing report-format checkbox | The two say the same thing in different words; auditor sees both and flags as redundant. | Choose one: either trim the report-format checkbox to point at `<success_criteria>`, or scope the `<success_criteria>` to non-report-format concerns. | Audit recommends consolidation. |
| Fourth `<final_step>` option chosen as a synonym of existing three | Audit re-flags Recommendation 5 unchanged. | Pick a semantically distinct fourth option per the guide's template. | Re-run `coding:audit-agent`; the Recommendation 5 finding persists. |
| BSD-style license header accidentally dropped during edit | Repository convention violated. | Re-add the header. | `grep -L 'BSD-style' agents/*.md` returns one or more file names. |

## Security / Abuse Cases

Not applicable. This spec modifies only structural metadata (frontmatter fields), section ordering, and XML tag names in agent definition files. No new credentials, no new code paths, no new external interactions, no new file or network I/O at runtime.

## Acceptance Criteria

Rung 1 — apply structural changes:

- [ ] Every agent file under `agents/` carries a `color` field in frontmatter — evidence: `grep -l '^color:' agents/*.md | wc -l` returns the same count as `ls agents/*.md | wc -l`.
- [ ] No agent file uses the `<workflow>` tag for an ordered multi-phase workflow — evidence: `grep -l '^<workflow>$' agents/*.md` returns zero matches in files whose workflow was renamed; `grep -l '^<critical_workflow>$' agents/*.md` returns ≥1 match in each such file.
- [ ] Each oversight/audit agent file contains a `<contextual_judgment>` block — evidence: `grep -l '<contextual_judgment>' agents/spec-auditor.md agents/prompt-auditor.md agents/scenario-auditor.md agents/spec-verifier.md` returns all four file paths.
- [ ] Every agent file contains a `<success_criteria>` block — evidence: `grep -l '<success_criteria>' agents/*.md | wc -l` equals `ls agents/*.md | wc -l`.
- [ ] The `effort: high` frontmatter field is in a resolved state — evidence: either (a) `grep -l 'effort: high' agents/*.md | wc -l` returns 0 (removed across the fleet), OR (b) `grep -n 'effort' docs/dark-factory-agent-conventions.md` returns ≥1 line AND `grep -l 'dark-factory-agent-conventions' docs/*.md` returns ≥2 file paths (the new doc plus one discoverability reference).
- [ ] Auditor `<final_step>` blocks contain ≥4 numbered options — evidence: in each of `agents/spec-auditor.md` and `agents/prompt-auditor.md`, the `<final_step>...</final_step>` block contains four lines matching `^[1-9]\. \*\*`.
- [ ] BSD-style license headers survive the edits — evidence: for every modified agent file that had a BSD header before, `grep -l 'BSD-style' <file>` still returns the file path after the change.
- [ ] `CHANGELOG.md` has an entry under `## Unreleased` describing the alignment — evidence: `awk '/^## Unreleased/,/^## v/' CHANGELOG.md` includes a bullet line containing the literal string `align agents with Agent & Command Development Guide` (or a phrasing that includes both `align` and `agent`).

Rung 2 — verification via the audit tool:

- [ ] `coding:audit-agent` on each of the five agent files reports zero critical structural issues — evidence: invoke `coding:audit-agent` against each of `agents/spec-creator.md`, `agents/spec-auditor.md`, `agents/prompt-creator.md`, `agents/prompt-auditor.md`, `agents/spec-verifier.md`; each report's `## Critical Issues` section is empty or reads `None.`.
- [ ] Each of the five files scores 8/10 or higher — evidence: per-file audit report header line matches the regex `Score: (8|9|10)/10`.

**Scenario coverage — NO new scenario.** Structural agent-metadata changes do not touch any runtime behavior. They are verified by re-running the auditor against the edited files; a scenario file would duplicate that assertion without exercising any production code path. The four-condition test from `docs/rules/scenario-writing.md` fails on conditions 1 and 4 (unit/grep-level verification is sufficient; no concrete named runtime regression risk).

## Verification

```
# Per-file structural assertions
grep -l '^color:' agents/*.md | wc -l
ls agents/*.md | wc -l    # both lines must match

grep -l '<critical_workflow>' agents/*.md
grep -l '<workflow>$' agents/*.md    # must be empty for renamed files

grep -l '<contextual_judgment>' agents/spec-auditor.md agents/prompt-auditor.md \
  agents/scenario-auditor.md agents/spec-verifier.md

grep -l '<success_criteria>' agents/*.md | wc -l    # must equal file count

# effort: high resolution (one branch must hold)
grep -l 'effort: high' agents/*.md | wc -l
ls docs/dark-factory-agent-conventions.md 2>/dev/null

# Final-step option count (≥4 in auditor files)
awk '/<final_step>/,/<\/final_step>/' agents/spec-auditor.md | grep -cE '^[1-9]\. \*\*'
awk '/<final_step>/,/<\/final_step>/' agents/prompt-auditor.md | grep -cE '^[1-9]\. \*\*'

# Per-file audit-agent run — Critical Issues empty, score ≥ 8/10
# (invoke coding:audit-agent against each of the five agent files; inspect report)

# CHANGELOG
awk '/^## Unreleased/,/^## v/' CHANGELOG.md
```

Expected: every grep returns the documented count; every audit-agent report shows zero critical issues and a score of 8-10.

## Do-Nothing Option

Leaving the agents as-is is functional today — the auditor and creator agents do their jobs. The cost is ongoing rather than acute: every new agent added to dark-factory must pick between matching the existing inconsistent style and matching the guide; every audit-agent run on the existing files re-flags the same seven recommendations; cross-agent diff review stays noisier than necessary because every file has a slightly different shape. None of this blocks shipping, but the fix is small and bounded (single-day work; seven metadata changes across five files) and ages well — once aligned, the audit-agent tool can be trusted as a clean signal again, instead of being mostly-noise on the dark-factory fleet.
