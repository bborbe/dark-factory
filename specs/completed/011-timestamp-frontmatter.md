---
status: completed
---

# Timestamp Frontmatter: Lifecycle Tracking

## Problem

No visibility into when prompts were created, queued, started, or completed. Can't measure execution duration, queue wait time, or total lifecycle. Status field only shows current state, not history.

## Goal

ISO 8601 timestamps in frontmatter tracking each lifecycle transition. Enables duration analysis, performance monitoring, and historical tracking.

## Non-goals

- No duration calculation in dark-factory itself (timestamps enable external analysis)
- No timezone support (all UTC)
- No sub-second precision

## Desired Behavior

1. `created`: set when file first seen during normalization. Never overwritten.
2. `queued`: set when status changes to `queued`. Preserved on retry (keeps original queue time).
3. `started`: set when status changes to `executing`. Always overwritten (retries get fresh start time).
4. `completed`: set when status changes to `completed` or `failed`. Always overwritten.
5. All timestamps: `time.Now().UTC().Format(time.RFC3339)` (e.g., `2026-03-01T22:30:00Z`)

## Constraints

- Timestamps are strings in frontmatter, not parsed time objects
- `created` is write-once (set if empty, never overwrite)
- `queued` is write-once per queue cycle (preserved on retry)
- `started` and `completed` are always overwritten (reflects latest attempt)

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| File already has timestamps | Respect write-once rules, don't overwrite created/queued | None needed |
| Clock skew | Timestamps may be slightly off; acceptable for logging | None needed |

## Acceptance Criteria

- [ ] `created` set during normalization, never overwritten
- [ ] `queued` set on status=queued, preserved on retry
- [ ] `started` set on status=executing, overwritten on retry
- [ ] `completed` set on status=completed or status=failed
- [ ] All timestamps in RFC3339 UTC format
- [ ] Status display includes timestamps for executing/completed prompts

## Verification

Run `make precommit` — must pass.

## Do-Nothing Option

No timing data. Can't measure if prompts take 5 minutes or 50 minutes. Can't identify slow prompts or estimate queue completion time.
