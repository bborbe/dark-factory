---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-03-30T16:37:46Z"
prompted: "2026-03-30T16:42:18Z"
branch: dark-factory/full-slug-spec-references
---

## Summary

- Spec references in prompt frontmatter use full slug (`spec: ["001-workflow-direct"]`) instead of bare number (`spec: ["001"]`)
- Dark-factory auto-detects bare number spec references and migrates them to full slugs during normal operation
- Makes spec references human-readable and resilient to number conflicts after branch merges
- No behavior change for lookups — existing number extraction already handles both formats

## Problem

Prompt frontmatter stores spec references as bare numbers: `spec: ["001"]`. This is:
1. **Opaque** — reader must look up which spec 001 is
2. **Fragile** — after a branch merge with number conflicts, bare `001` could point to the wrong spec
3. **Inconsistent** — some prompts already use full slugs (e.g., `spec: "020-auto-prompt-generation"` in test fixtures)

The full slug format (`001-workflow-direct`) is already supported by the existing number extraction and spec-matching logic — it's just not consistently used.

## Goal

After this work, every spec reference in every prompt across all lifecycle directories uses the human-readable full slug format. New prompts generated from specs automatically get the full slug. Existing bare-number references are auto-migrated on daemon/run startup.

## Assumptions

- The existing number extraction logic is the single source of truth and already handles both bare numbers and full slugs
- No external consumers depend on bare-number format in prompt frontmatter
- All spec files follow the `NNN-slug.md` naming convention

## Non-goals

- Changing the spec file format itself
- Changing how number extraction works (already handles both formats)
- Changing spec filenames

## Desired Behavior

1. When the YOLO agent generates prompts from a spec, the `spec:` frontmatter field contains the full spec filename without extension (e.g., `spec: ["035-self-healing-number-conflicts"]` instead of `spec: ["035"]`).
2. During normal operation (daemon startup, run command), dark-factory scans all prompt directories and replaces bare spec numbers with full slugs by looking up the actual spec file. Changes are logged.
3. If a bare spec number matches multiple spec files (shouldn't happen, but defensive), log a warning and skip that reference.
4. If a bare spec number matches no spec file, leave it unchanged and log a warning.
5. The migration is idempotent — references that already have the full slug are left unchanged.
6. Spec-matching operations continue to accept both bare numbers and full slugs — no behavior change for lookups.

## Constraints

- Use the existing number extraction logic — do not duplicate it
- Existing prompts with full-slug references must not be modified
- The spec field format in YAML (string or array of strings) is unchanged
- Migration must work across all lifecycle dirs (inbox, in-progress, completed, log)

## Security / Abuse Cases

N/A — internal CLI tooling, no user-facing input. Migration operates only on configured prompt/spec directories.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Bare number matches no spec file | Leave reference unchanged, log warning | Manual fix |
| Bare number matches multiple specs | Leave reference unchanged, log warning | Manual fix after reindex |
| Spec file has no number prefix | Skip, not relevant | None needed |
| Prompt has no `spec:` field | Skip | None needed |

## Acceptance Criteria

- [ ] Generated prompts from specs have full slug in `spec:` field
- [ ] Daemon/run auto-migrates bare spec numbers to full slugs on startup
- [ ] Migration is idempotent (second run = no changes)
- [ ] Unresolvable bare numbers logged as warnings, not modified
- [ ] Spec-matching accepts both `"001"` and `"001-workflow-direct"` (backward compatible)
- [ ] All existing tests pass

## Verification

```
make precommit
```

## Do-Nothing Option

Bare number references remain. Users must manually cross-reference spec numbers. After branch merges with number conflicts, bare references may silently point to the wrong spec.
