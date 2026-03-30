---
tags:
  - dark-factory
  - spec
status: draft
---

## Summary

- Spec references in prompt frontmatter use full slug (`spec: ["001-workflow-direct"]`) instead of bare number (`spec: ["001"]`)
- Migration command updates all existing prompts from bare numbers to full slugs
- Makes spec references human-readable and resilient to number conflicts after branch merges
- No behavior change — `specnum.Parse()` already handles both formats

## Problem

Prompt frontmatter stores spec references as bare numbers: `spec: ["001"]`. This is:
1. **Opaque** — reader must look up which spec 001 is
2. **Fragile** — after a branch merge with number conflicts, bare `001` could point to the wrong spec
3. **Inconsistent** — some prompts already use full slugs (e.g., `spec: "020-auto-prompt-generation"` in test fixtures)

The full slug format (`001-workflow-direct`) is already supported by `specnum.Parse()` and `HasSpec()` — it's just not consistently used.

## Goal

All spec references in prompt frontmatter use the full slug format. New prompts generated from specs automatically get the full slug. Existing prompts are migrated.

## Non-goals

- Changing the spec file format itself
- Changing how `specnum.Parse()` works (already handles both formats)
- Changing spec filenames

## Desired Behavior

1. When the YOLO agent generates prompts from a spec, the `spec:` frontmatter field contains the full spec filename without extension (e.g., `spec: ["035-self-healing-number-conflicts"]` instead of `spec: ["035"]`).
2. A new CLI command `dark-factory prompt migrate-spec-refs` scans all prompt directories and replaces bare spec numbers with full slugs by looking up the actual spec file. Prints changes, applies them.
3. If a bare spec number matches multiple spec files (shouldn't happen, but defensive), log a warning and skip that reference.
4. If a bare spec number matches no spec file, leave it unchanged and log a warning.
5. The migration is idempotent — references that already have the full slug are left unchanged.
6. `HasSpec()`, `CountBySpec()`, and other spec-matching functions continue to work with both formats (they already do via `specnum.Parse()`).

## Constraints

- `specnum.Parse()` is the canonical number extractor — do not duplicate its logic
- Existing prompts with full-slug references must not be modified
- The `SpecList` type (`[]string`) and YAML format are unchanged
- Migration must work across all lifecycle dirs (inbox, in-progress, completed, log)

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Bare number matches no spec file | Leave reference unchanged, log warning | Manual fix |
| Bare number matches multiple specs | Leave reference unchanged, log warning | Manual fix after reindex |
| Spec file has no number prefix | Skip, not relevant | None needed |
| Prompt has no `spec:` field | Skip | None needed |

## Acceptance Criteria

- [ ] Generated prompts from specs have full slug in `spec:` field
- [ ] `dark-factory prompt migrate-spec-refs` updates bare numbers to full slugs
- [ ] Migration is idempotent (second run = no changes)
- [ ] Unresolvable bare numbers logged as warnings, not modified
- [ ] `HasSpec()` works with both `"001"` and `"001-workflow-direct"`
- [ ] All existing tests pass

## Verification

```
make precommit
```

## Do-Nothing Option

Bare number references remain. Users must manually cross-reference spec numbers. After branch merges with number conflicts, bare references may silently point to the wrong spec.
