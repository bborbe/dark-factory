---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-04-16T19:39:29Z"
generating: "2026-04-16T19:56:39Z"
prompted: "2026-04-16T19:58:46Z"
branch: dark-factory/rate-limit-event-formatter
---

## Summary

- Claude Code's stream-json emits `rate_limit_event` messages the Go formatter (spec 047) does not know about, so operators see `[unknown type: rate_limit_event]` noise in live output and the formatted log.
- Render these events as a single concise line showing rate-limit type, utilization percentage, reset time, and status.
- Preserve the existing fallback so a malformed or info-less event still renders gracefully rather than crashing.

## Problem

Spec 047 ported the stream formatter to Go but only covered the message types the Python v2 formatter handled. `rate_limit_event` was not among them. Claude Code emits this event whenever a rate-limit bucket crosses a threshold (currently observed at 85% of the seven-day window). Every such emission shows up as `[unknown type: rate_limit_event]`, which is noise that obscures real output and hides the actual signal — the operator loses visibility into how close the run is to a rate-limit block.

## Goal

After this work, every `rate_limit_event` in the container's stream-json is rendered as one human-readable line in the formatted log and terminal, matching the existing formatter's timestamp-prefixed style, with enough detail that the operator can tell at a glance which bucket is filling up, how full it is, when it resets, and whether usage is still allowed.

## Non-goals

- Acting on the rate-limit event (throttling, pausing, retrying, alerting) — this spec is presentation-only.
- Persisting rate-limit history or surfacing it in prompt frontmatter.
- Changing the raw JSONL log — it already captures these events verbatim via spec 047.
- Making the line format configurable.

## Desired Behavior

1. A `rate_limit_event` with a populated `rate_limit_info` renders as a single timestamp-prefixed line that includes: a warning glyph, the rate-limit type (e.g. `seven_day`), utilization as an integer percentage, the reset time formatted as human-readable local time, and the status (e.g. `allowed_warning`).
2. A `rate_limit_event` with no `rate_limit_info` field, or with an empty/null one, renders without crashing — falling back to a generic rendering in the same style as other unknown-shape messages (it MUST NOT emit `[unknown type: rate_limit_event]`).
3. Unknown `status` or `rateLimitType` string values pass through verbatim — the formatter does not enumerate them.
4. Reset time is formatted in the operator's local timezone, consistent with the rest of the formatter's timestamp handling.
5. The event no longer appears as `[unknown type: rate_limit_event]` in any output.

## Constraints

- Pure presentation change inside the Go formatter introduced by spec 047 — no impact on executor, container, CLI, or config schema.
- Stylistic consistency with the rest of the formatter: single line, timestamp prefix, unicode glyphs permitted (⚠ already used).
- No new runtime dependencies.
- The formatter must remain robust against missing or unexpected fields — spec 047's failure-mode rules apply unchanged.
- Raw JSONL log output is untouched — this spec only adds a rendering branch.

## Assumptions

- `rate_limit_event` messages arrive on the same stdout stream as other stream-json messages, one per line.
- Observed field names are stable: `rate_limit_info.status`, `resetsAt` (unix seconds), `rateLimitType`, `utilization` (0..1 float), `isUsingOverage`, `surpassedThreshold`. Additional fields may appear later; the formatter ignores unknown ones.
- `resetsAt` is a unix epoch in seconds.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `rate_limit_info` is nil or missing | Render a concise fallback line naming the event type, no crash | None |
| `resetsAt` is zero or absent | Omit the reset clause from the line; still render the rest | None |
| `utilization` is outside 0..1 | Render the percentage as-is (rounded), no clamping | None |
| Unknown `status` or `rateLimitType` string | Render the string verbatim | None |
| Malformed JSON at the envelope level | Handled by existing spec-047 parse-failure path (passthrough) | None |

## Security / Abuse Cases

Same boundary as spec 047: the stream is untrusted input. The new renderer does string concatenation and number formatting only — no subprocess, no filesystem, no template evaluation, no network. Field values are displayed, never interpreted.

## Acceptance Criteria

- [ ] A `rate_limit_event` with full `rate_limit_info` renders as a single timestamp-prefixed line that names the rate-limit type, utilization percentage, reset time, and status.
- [ ] `[unknown type: rate_limit_event]` no longer appears in formatted output for well-formed events.
- [ ] A `rate_limit_event` missing `rate_limit_info` renders a fallback line instead of crashing or emitting the generic unknown-type line unchanged.
- [ ] Existing formatter tests continue to pass.
- [ ] New unit tests cover: (a) a full render with `seven_day` / `allowed_warning`, (b) a full render with a different `rateLimitType` value (e.g. `five_hour`), (c) a `rate_limit_event` with nil `rate_limit_info` producing a fallback line.
- [ ] No new runtime dependencies introduced.

## Verification

```
make precommit
```

Operator smoke check: re-run a prompt known to trigger rate-limit warnings (or replay the captured raw JSONL from prompt `003-update-go-deps`) and confirm the formatted log contains a readable rate-limit line instead of the unknown-type marker.

## Do-Nothing Option

Leave `rate_limit_event` falling through to the generic unknown-type branch. Operators continue to see `[unknown type: rate_limit_event]` noise and lose visibility into actual rate-limit pressure. Acceptable only if we expect the event to be removed upstream — there is no indication of that, so doing nothing means the noise keeps accumulating in every long-running session.
