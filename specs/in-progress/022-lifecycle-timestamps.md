---
status: prompted
---

# Lifecycle Timestamps for Specs and Prompts

## Problem

Specs only track `status` — there is no record of when each transition happened. Prompts in the inbox have no frontmatter at all until they reach the queue. This makes it impossible to see how long each phase took or when a file was first created.

## Goal

Both specs and inbox prompts get timestamps at each lifecycle transition. A spec's full history is visible in its frontmatter. A prompt's `created` timestamp is set the moment it lands in the inbox.

## Non-goals

- No change to existing timestamp fields on queued/completed prompts
- No analytics or reporting on the timestamps

## Desired Behavior

1. When a prompt file appears in the inbox (`prompts/`), dark-factory adds `created: <ISO timestamp>` to its frontmatter if not already present.
2. When a spec transitions to `approved`, dark-factory adds `approved: <ISO timestamp>`.
3. When a spec transitions to `prompted`, dark-factory adds `prompted: <ISO timestamp>`.
4. When a spec transitions to `verifying`, dark-factory adds `verifying: <ISO timestamp>`.
5. When a spec transitions to `completed`, dark-factory adds `completed: <ISO timestamp>`.
6. Timestamps are only written once — if a field already exists it is not overwritten.

## Constraints

- Existing `created`, `queued`, `started`, `completed` fields on prompts are unchanged
- Spec frontmatter struct must be extended — not a new file format
- `make precommit` must pass

## Failure Modes

| Trigger | Expected behavior |
|---------|------------------|
| Inbox file already has `created` | Skip — do not overwrite |
| Spec already has transition timestamp | Skip — do not overwrite |

## Acceptance Criteria

- [ ] New prompt in inbox gets `created` timestamp automatically
- [ ] `spec approve` writes `approved` timestamp to spec
- [ ] `spec generate` (auto-prompt) writes `prompted` timestamp
- [ ] All-prompts-merged transition writes `verifying` timestamp
- [ ] `spec verify` writes `completed` timestamp
- [ ] No existing timestamps are overwritten
- [ ] `make precommit` passes

## Verification

```
touch prompts/test.md && sleep 1 && head -5 prompts/test.md  # created field present
dark-factory spec approve specs/022-lifecycle-timestamps.md
head -5 specs/022-lifecycle-timestamps.md                     # approved field present
```

## Do-Nothing Option

Keep current state. Spec history is invisible; inbox prompts have no creation time. Acceptable for small projects, but makes auditing and debugging the pipeline harder as it grows.
