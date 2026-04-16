---
tags:
  - dark-factory
  - spec
status: draft
---

## Summary

- When a prompt that previously failed is retried and succeeds, the stale `lastFailReason` from the earlier failure currently persists in the completed frontmatter.
- This spec requires `lastFailReason` to be cleared on successful completion so completed prompts do not carry misleading failure messages.
- A subsequent failure on retry still sets `lastFailReason` to the new reason — the field always reflects the most recent failure (or is absent).
- `retryCount` is preserved across the success transition so history of "how many retries it took" is retained.

## Problem

After a failed prompt is retried via `dark-factory prompt retry` and succeeds, the prompt moves to `completed/` but its frontmatter still contains `lastFailReason` from the original failed run. Anyone scanning completed prompts sees a failure reason on a prompt that actually succeeded, which is confusing and breaks any tooling that uses `lastFailReason` presence as a signal. The field is set in the failure path but never cleared in the success path.

## Goal

A completed prompt's frontmatter accurately reflects its final state: if the final attempt succeeded, no `lastFailReason` is present; if the final attempt failed, `lastFailReason` contains that failure's message. The field is "latest failure or nothing," never stale.

## Assumptions

- Clearing happens at **completion time** (lazy), not at requeue time (proactive). Completion-time clearing is simpler, covers manually-edited-then-succeeded prompts, and puts the clear logic next to the success transition it is conceptually tied to.
- `retryCount` is **preserved** on success — it is historically useful ("this prompt succeeded on the 2nd attempt") and is separately reset by `prompt retry`/`requeue` per the existing behavior in spec 044.
- Frontmatter YAML field removal is supported by existing `pkg/frontmatter` helpers (or trivially added) — setting a field to its zero value should omit it on write.

## Non-goals

- Do not change when or how `lastFailReason` is **set** on failure — only when it is cleared.
- Do not change `retryCount` semantics.
- Do not introduce a separate "previous failure history" field. `lastFailReason` remains "latest only."
- Do not retroactively rewrite existing completed prompts that already have stale `lastFailReason`. Fix applies to future completions only.

## Desired Behavior

1. When a prompt transitions to `status: completed` via the normal success path, any existing `lastFailReason` field is removed from the frontmatter before the file is written to `completed/`.

2. When a previously-failed prompt is retried and fails again, `lastFailReason` is replaced with the new failure's message (current behavior — must continue to work).

3. When a prompt succeeds on its first attempt (never failed), the success transition is a no-op with respect to `lastFailReason` — the field was never set and is not present after completion.

4. `retryCount` is not modified by the success transition. A prompt that failed once and succeeded on retry shows `retryCount: 1` (or whatever value existed at success time) in `completed/`.

5. The `permanently_failed` status is not a success path and is out of scope — `lastFailReason` remains set for permanently failed prompts (current behavior).

## Constraints

- Existing failed-path behavior (setting `lastFailReason` with the failure message) must not change.
- Existing completed prompts in `completed/` are not rewritten — no migration.
- Related spec: `044-prompt-timeout-auto-retry.md` introduced `lastFailReason` and the retry/requeue semantics. The `retryCount` reset on explicit retry/requeue (spec 044) is unaffected by this spec.
- Frontmatter field removal must be robust: the written YAML must not contain an empty `lastFailReason: ''` line — the field must be absent entirely.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Frontmatter write during success transition fails | Log error, leave prompt in current directory; do not mark completed | Human inspects file, re-runs completion |
| `lastFailReason` field is malformed in source file | Removal still succeeds (removing a malformed field is safe) | None needed |
| Concurrent write (daemon + manual edit) | Normal last-write-wins — not introduced by this change | Out of scope |

## Security / Abuse Cases

Not applicable. No new trust boundaries, no user input, no HTTP surface. Change is confined to frontmatter field management in the local prompt file.

## Acceptance Criteria

- [ ] fail → retry → success path: completed prompt in `completed/` has no `lastFailReason` field.
- [ ] fail → retry → fail path: failed prompt has `lastFailReason` set to the new failure's message, not the previous one.
- [ ] success-only path (never failed): completed prompt has no `lastFailReason` field and no side effects on other frontmatter fields.
- [ ] `retryCount` value at success time is preserved through the success transition.
- [ ] `permanently_failed` prompts still carry their `lastFailReason` (regression check for spec 044 behavior).
- [ ] Unit tests cover all four paths above.
- [ ] `make precommit` passes.

## Verification

```
make precommit
```

## Do-Nothing Option

Without this fix, completed prompts carry stale failure messages. Humans must manually edit frontmatter after every successful retry (as was done for `003-test-build-info-metrics`). Any tooling that treats `lastFailReason` presence as "this prompt had issues" gives false positives on successfully retried prompts. Acceptable only if retries are rare; with auto-retry (spec 044) enabled, this will compound quickly.
