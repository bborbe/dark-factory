---
status: prompted
approved: "2026-05-08T08:57:51Z"
generating: "2026-05-08T08:59:20Z"
prompted: "2026-05-08T09:09:27Z"
branch: dark-factory/bug-completion-report-parser-tail-boundary
---

## Summary

- The completion report parser silently swallows a parse error and the daemon commits a "failed" prompt as if it succeeded
- Trigger is a log file whose 4096-byte tail window cuts between a start marker and its matching end marker, leaving an orphaned end marker before the next start marker
- `strings.Index` (first match) returns the orphan end before the surviving start; the `endIdx <= startIdx` guard fires; the wrapping validator downgrades the parse error to a debug log and treats the run as "no report found"
- The fallback critical-failure scanner only catches Claude-CLI-level crashes, not agent-reported failures, so the prompt commits, moves to `completed/`, and (with `autoRelease: true`) is pushed
- Observed in production for prompt `005-update-build-golang-1.26.3.md` against `dark-factory v0.154.0`

## Problem

Dark-factory is supposed to refuse to commit when the agent's `DARK-FACTORY-REPORT` says `status: failed` (`pkg/completionreport/validator.go:85`). When the parser cannot extract the report because of a tail-window boundary artifact, the daemon falls back to "no report found = success" and commits anyway. The agent's failure signal is lost. Operators see a green status and a pushed commit for work the agent declared broken.

## Goal

When the agent emits any `DARK-FACTORY-REPORT` block with `status != success`, the daemon marks the prompt failed and does not commit.

## Non-goals

- Raising `tailBytes` from 4 KiB. Defense-in-depth on tail-window size is a separate change.
- Changing the log level (`Debug` → `WARN`) of any path other than the specific `ParseFromLog` error downgrade this spec fixes.
- Touching the agent-side `DARK-FACTORY-REPORT` suffix or DoD prompt — fix is parser-side only.
- Changing the `MarkerStart` / `MarkerEnd` constants.

## Assumptions

- Tail window is 4096 bytes (`tailBytes` constant in the parser package).
- The agent may emit zero, one, or more than one `DARK-FACTORY-REPORT` block per run; duplicates are not currently deduplicated upstream.
- Production logs routinely exceed 4 KiB (the failing prompt's log was 9235 bytes), so the tail-window boundary is reachable in normal operation, not just under stress.
- `ParseFromLog` is called only from the validator and from tests — no third-party caller relies on the current orphan-end-returns-error behavior.

## Desired Behavior

1. The parser identifies the **last complete** start/end marker pair in the log, not the textually-first start and textually-first end.
2. When a start marker is in the file but its matching end marker is not (or vice versa) the parser surfaces an actionable error rather than returning `nil`.
3. When the parser returns an error, the validator does NOT downgrade it to "no report"; the prompt is treated as failed and the failure handler is notified.
4. Logs that contain duplicate `DARK-FACTORY-REPORT` blocks (agent emitted twice) are handled correctly: the last block wins.
5. A regression test reproduces the original 9235-byte log with two report blocks at byte offsets 5122/5693 and 7371/7942 and asserts the parser returns `status: failed` (not nil).

## Constraints

- Public function signature `report.ParseFromLog(ctx, logFile) (*CompletionReport, error)` MUST NOT change — callers in `pkg/completionreport/validator.go` and tests rely on it.
- Marker constants (`MarkerStart`, `MarkerEnd`) MUST NOT change — would break already-deployed agent prompt suffixes.
- Backwards compatibility: logs with no report at all must still return `(nil, nil)` (the documented "old prompt without report" path).
- Validator behavior for genuinely missing reports (no start marker anywhere in the tail) MUST stay the same: fall through to `ScanForCriticalFailures`.
- No change to the agent-side suffix or DoD prompt — fix is parser-side only.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Log >4096 bytes, last full report inside tail | Parse and return last report | n/a — happy path |
| Log >4096 bytes, only the final marker pair is fully inside tail; an earlier orphan end marker is also inside tail | Parse the last complete pair; ignore the orphan | n/a |
| Log has start marker in tail but no end marker anywhere | Return error; validator escalates to failure | Operator inspects log |
| Log has end marker in tail but no start marker anywhere | Return `(nil, nil)` (no visible report start = backwards-compat path) | Validator falls through to critical-failure scan |
| Log has zero markers | Return `(nil, nil)` (backwards compat) | Validator falls through to critical-failure scan |
| Agent emits two identical report blocks | Last block wins; status reflected accurately | n/a |
| Tail-window cuts a single marker pair so neither is fully present | Parser returns `(nil, nil)` (no markers visible) | Validator falls through to critical-failure scan; consider raising `tailBytes` |

## Acceptance Criteria

- [ ] `pkg/report/parse.go` selects the **last complete** marker pair, not the first start + first end.
- [ ] Unit test: log with two complete report blocks where the first is `status: success` and the second is `status: failed` → parser returns the failed block.
- [ ] Unit test: log mirroring the production failure (9235 bytes, marker byte offsets 5122/5693/7371/7942, both blocks `status: failed`) → parser returns `status: failed`, not `nil`.
- [ ] Unit test: log with an orphaned end marker followed by a complete start/end pair (tail-boundary case) → parser returns the complete pair, not an error and not `nil`.
- [ ] Unit test: log with only a start marker (no end) → parser returns an error.
- [ ] Unit test: log with only an end marker (no start) → parser returns `(nil, nil)` (no visible report start = backwards-compat path).
- [ ] Unit test: log with zero markers → parser returns `(nil, nil)`.
- [ ] The validator no longer downgrades a `ParseFromLog` error to a debug-level "no report" outcome; an actionable parse error propagates to the failure handler so the prompt is treated as failed.
- [ ] Existing tests in `pkg/report/parse_test.go` and `pkg/completionreport/validator_test.go` still pass.
- [ ] Reproduction (below) replayed: prompt with status:failed report does NOT commit and does NOT move to `completed/`.

## Verification

`make precommit` must pass cleanly with exit code 0.

Reproduction replay:

1. Use the captured production log `prompts/log/005-update-build-golang-1.26.3.log` (or its committed test-fixture equivalent at the same byte size and marker offsets).
2. Call the validator (`pkg/completionreport/validator.go`) against the log.
3. Assert: validator returns a non-nil error reflecting the parsed `status: failed`. The processor's commit/move path is NOT reached (verified at the validator boundary; full end-to-end execution is not required).

## Reproduction

**Environment:** dark-factory `v0.154.0`, container `docker.io/bborbe/claude-yolo:v0.6.3`, model `claude-sonnet-4-6`, project `~/Documents/workspaces/sm-octopus/build` (`workflow: direct`, `validationCommand: make precommit` default).

**Steps:**

1. Approve a prompt whose verification command (`make precommit`) is structurally guaranteed to fail (e.g. a Dockerfile-only directory inheriting `Makefile.precommit` that runs `go mod tidy` against a missing `go.mod`).
2. Daemon executes the prompt. Agent edits the file successfully but `make precommit` exits non-zero.
3. Agent emits `DARK-FACTORY-REPORT` block with `"status":"failed"`. Then emits a second identical block at the very end of its output (this happens when the agent appends an `## Improvements` section between the two reports).
4. Container exits cleanly (exit 0) — Claude CLI succeeds even when the agent self-reports failure.

**Observed evidence:**

Daemon log (`~/Documents/workspaces/sm-octopus/build/.dark-factory.log`):

```
time=2026-05-08T10:44:36.474+02:00 level=INFO msg="docker container exited" exitCode=0
time=2026-05-08T10:44:37.528+02:00 level=INFO msg="committed changes"
time=2026-05-08T10:44:37.557+02:00 level=INFO msg="moved to completed" file=005-update-build-golang-1.26.3.md
```

No `completion report` log entry — proves validator's `ParseFromLog` returned `(nil, error)`, the error was downgraded to debug, and `ScanForCriticalFailures` returned no critical failure.

Prompt log (`prompts/log/005-update-build-golang-1.26.3.log`, 9235 bytes) marker offsets:

| Marker | Byte offset | In last 4096 bytes (>5139)? |
|---|---|---|
| 1st `<!-- DARK-FACTORY-REPORT` | 5122 | NO (cut by 17 bytes) |
| 1st `DARK-FACTORY-REPORT -->` | 5693 | yes |
| 2nd `<!-- DARK-FACTORY-REPORT` | 7371 | yes |
| 2nd `DARK-FACTORY-REPORT -->` | 7942 | yes |

Both `DARK-FACTORY-REPORT` JSON blocks contain `"status":"failed"`.

Resulting prompt frontmatter (`prompts/completed/005-update-build-golang-1.26.3.md`):

```yaml
status: completed
container: build-005-update-build-golang-1-26-3
dark-factory-version: v0.154.0
started: "2026-05-08T08:43:13Z"
completed: "2026-05-08T08:44:37Z"
```

No `lastFailReason`. No `retryCount`. The push happened on top of this commit.

## Expected vs Actual

**Expected** (per `pkg/completionreport/validator.go:85-91`):

> When the parsed completion report has `status != success`, the validator returns an error like `completion report status: failed`. The processor (`pkg/processor/processor.go:359-363`) propagates this error, calls `failureHandler.NotifyFromReport`, and does NOT call `workflowExecutor.Complete`. The prompt stays in `prompts/in-progress/` with `status: failed`.

**Actual** (observed):

> Parser returned a non-nil error because of the orphaned end marker (`pkg/report/parse.go:65-68`: `endIdx <= startIdx`). Validator (`pkg/completionreport/validator.go:43-47`) downgraded the error to `slog.Debug` and set `completionReport = nil`. `ScanForCriticalFailures` found no claude-CLI-level fatal. Validator returned `(nil, nil)`. Processor proceeded to `Complete`. Commit was created and the prompt was moved to `completed/`.

## Why this is a bug

Two documented invariants are violated:

1. `pkg/completionreport/validator.go:30` — "Validate parses the completion report from the log and detects claude-CLI-level failures." A parse error that prevents detection of the agent's own failure report is a detection failure, not a benign "no report" condition.
2. `architecture-flow.md:96` — "Validate report (success/partial/failed)" is listed as Step 11 in the canonical execution flow, ahead of "Move prompt to completed/" (Step 12). Skipping validation when parsing fails inverts the flow.

The 4096-byte tail window is also too small for realistic agent logs — production logs cross this threshold routinely (this one was 9235 bytes), so the tail-boundary edge case is reachable in normal operation.

## Follow-ups (deferred to separate specs)

- Raise `tailBytes` (e.g. 16 KiB) as defense-in-depth alongside this parser fix.
- Promote the validator's `ParseFromLog` error log from Debug to WARN so similar regressions are visible without source-code reading.
