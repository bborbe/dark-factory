---
status: approved
spec: ["101"]
created: "2026-06-26T08:01:00Z"
queued: "2026-06-26T08:00:41Z"
---

<summary>

- Rewires the five places that today each re-derive a prompt's state from its raw inputs so they instead ask the single owning package for the answer.
- Each consumer's outward behaviour is unchanged — the same prompt still gets resumed, reset, cancelled, recovered, or skipped exactly as before; only the decision of "what state is this?" moves behind one function call.
- The resume and reset logic now expresses "container alive vs gone vs docker unreachable" through a shared state vocabulary instead of inline status-string comparisons.
- The half-state recovery rule (file already in the completed dir wins as completed) is now sourced from the shared package rather than re-implemented in the recoverer.
- Unrecognised on-disk status strings now resolve to a single "unknown" outcome with a clear error log, instead of silently falling through each consumer's ad-hoc branch.
- After this prompt the five named consumer files contain no inline frontmatter-status interpretation; the later precommit gate (prompt 4) can then lock that boundary.

</summary>

<objective>
Migrate the five existing consumers — `pkg/runner/lifecycle.go`, `pkg/promptresumer/resumer.go`, `pkg/committingrecoverer/recoverer.go`, `pkg/queuescanner/scanner.go`, `pkg/cancellationwatcher/watcher.go` — so each replaces its inline tuple reading with a call into `pkg/promptstate`. Every consumer's external behaviour stays identical. After this prompt, none of the five files contain a `prompt.PromptStatus` token used for tuple interpretation.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-composition.md` — call injected behaviour, not package functions (here `promptstate` exposes pure functions on a leaf package, allowed to call directly).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — error wrapping, GoDoc.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega, coverage, counterfeiter mocks.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors` wrapping idioms.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — changelog format.

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/101-extract-unified-prompt-state-machine.md` — Desired Behavior item 3; Constraints; Failure Modes rows 2, 4, 7; Acceptance Criteria 3, 8, 12.

Prompt 1's deliverable MUST already be on the tree. If `pkg/promptstate/interpret.go` is absent, STOP and report `Status: failed` with message "pkg/promptstate not yet deployed (prompt 1)". Read it before editing:
- `/workspace/pkg/promptstate/state.go`, `transitions.go`, `interpret.go` — the exported surface: `State`, the seven constants + `StateUnknown`, `Location` (`LocationInProgress`/`LocationCompleted`), `DockerState` (`DockerStateRunning`/`DockerStateStopped`/`DockerStateUnavailable`), `InterpretTuple(location, status, container, dockerState) State`, `IsValidTransition`.

Read these source files END-TO-END before editing (full reads):
- `/workspace/pkg/runner/lifecycle.go` — esp. `resumeOrResetExecutingEntry` (line 245). Current rule: load `pf`; if `PromptStatus(pf.Frontmatter.Status) != ExecutingPromptStatus` skip (line 259); read `containerName := pf.Frontmatter.Container` (262); `checker.IsRunning(ctx, containerName)` (263) — on ERROR it logs and RETURNS the error (refuse to reset); if running, resume (log + return nil); if not running, notify + `pf.MarkApproved()` + save.
- `/workspace/pkg/promptresumer/resumer.go` — `prepareResume` (line ~184). Same status check (line 192) and `containerName := pf.Frontmatter.Container` (196). Preserve its empty-container handling (lines 196-205).
- `/workspace/pkg/committingrecoverer/recoverer.go` — `Recover` (line 94). Half-state branch lines 131-145: `if filepath.Clean(filepath.Dir(promptPath)) == filepath.Clean(r.completedDir)` → `pf.MarkCompleted()` in place; else `MoveToCompleted`.
- `/workspace/pkg/queuescanner/scanner.go` — line 296 `status := prompt.PromptStatus(pf.Frontmatter.Status)` then `status.IsPreExecution()` gate (297), `pr.Status = status` (312); and line 481 `pf.Frontmatter.Status == string(prompt.PendingVerificationPromptStatus)`.
- `/workspace/pkg/cancellationwatcher/watcher.go` — line 99 `pf.Frontmatter.Status == string(prompt.CancelledPromptStatus)` → stop container.
- The existing test files for each package (`*_test.go` and `*_suite_test.go`) — match their Ginkgo style and update assertions.

VERIFIED FACTS (do not re-derive):
- The module path for the new package is `github.com/bborbe/dark-factory/pkg/promptstate`.
- `checker.IsRunning(ctx, name) (bool, error)` is the docker-liveness probe (lifecycle.go line 263, resumer). Map its result to `DockerState`: `(true, nil) -> DockerStateRunning`; `(false, nil) -> DockerStateStopped`. In `lifecycle.go` the CURRENT behaviour on `IsRunning` ERROR is to REFUSE to reset and PROPAGATE the error (lines 270-272) — preserve that guard exactly; only call `InterpretTuple` on the success path.
- `cancellationwatcher` and `queuescanner` do NOT consult docker — pass `promptstate.DockerStateUnavailable` (the "docker not consulted" value, which `InterpretTuple` treats as non-coercing).
- `filepath.Dir(promptPath)` vs `completedDir`/`inProgressDir` supplies `Location`. `committingrecoverer` already computes this comparison at line 131 — reuse it.
- The `queuescanner` `Prompt` struct field `pr.Status` is typed `prompt.PromptStatus` (verify in `pkg/prompt`). Assigning it from a raw string requires a `prompt.PromptStatus` conversion — this conversion is moved into a `promptstate` helper (requirement 0) so `scanner.go` carries no `prompt.PromptStatus` token.

</context>

<requirements>

The general approach: each consumer constructs the four inputs `(location, raw status string, container, docker state)` and calls a `promptstate` helper that returns a `promptstate.State`, then branches on that state. The action taken per state MUST be byte-for-byte identical to today. The three `promptstate` helpers added in requirement 0 keep every `prompt.PromptStatus(...)` conversion inside the allow-listed owner package, so the five consumer files end up with zero `prompt.PromptStatus` tokens (spec AC-3).

## 0. Add three string-input helpers to `pkg/promptstate` (do this FIRST)

In `/workspace/pkg/promptstate/interpret.go` add these three helpers. They keep the `prompt.PromptStatus(...)` conversions inside `pkg/promptstate` (the allow-listed owner), so consumers pass the raw `pf.Frontmatter.Status` string directly:

```go
// InterpretRawTuple is InterpretTuple with the raw on-disk status string as input.
// Consumers pass pf.Frontmatter.Status directly; the PromptStatus conversion lives
// here so consumer files contain no prompt.PromptStatus token (spec AC-3 gate).
func InterpretRawTuple(location Location, rawStatus string, container string, dockerState DockerState) State {
	return InterpretTuple(location, prompt.PromptStatus(rawStatus), container, dockerState)
}

// IsPreExecutionStatus reports whether the raw status is a pre-execution status
// (idea/draft/approved). It preserves the queuescanner pre-lock gate semantics,
// which are broader than the seven canonical InterpretTuple states.
func IsPreExecutionStatus(rawStatus string) bool {
	return prompt.PromptStatus(rawStatus).IsPreExecution()
}

// StatusFromRaw converts a raw on-disk status string to a prompt.PromptStatus,
// keeping the conversion inside the allow-listed owner package.
func StatusFromRaw(rawStatus string) prompt.PromptStatus {
	return prompt.PromptStatus(rawStatus)
}
```

Add focused `pkg/promptstate` tests: `InterpretRawTuple` matches `InterpretTuple` for a representative case (e.g. executing + running -> StateExecuting); `IsPreExecutionStatus("approved")` is true and `IsPreExecutionStatus("executing")` is false; `StatusFromRaw("approved") == prompt.ApprovedPromptStatus`.

## 1. `pkg/runner/lifecycle.go` — `resumeOrResetExecutingEntry`

1.1. Keep the early `pf == nil`/load-error guards as-is.

1.2. Replace the status skip (line 259) AND the running/not-running branch. Read `containerName := pf.Frontmatter.Container`; call `running, err := checker.IsRunning(ctx, containerName)`; KEEP the existing error guard verbatim (log `"container liveness check failed, refusing to reset prompt"` + `return errors.Wrapf(ctx, err, "check container liveness %s", containerName)`). On the success path:

```go
dockerState := promptstate.DockerStateStopped
if running {
	dockerState = promptstate.DockerStateRunning
}
state := promptstate.InterpretRawTuple(promptstate.LocationInProgress, pf.Frontmatter.Status, containerName, dockerState)
```

Then branch:
- `state == promptstate.StateExecuting` -> resume: keep the existing `slog.Info("resuming prompt, container still running", ...)` (do NOT touch the log call form) and `return nil`.
- `state == promptstate.StateAborted` -> reset: keep the existing `slog.Info("resetting prompt, container not found", ...)`, the notifier `n.Notify(...)` call, `pf.MarkApproved()`, and `return errors.Wrap(ctx, pf.Save(ctx), "reset executing prompt")`.
- any other `state` -> `return nil` (skip). A non-executing frontmatter status resolves to a non-`Executing`/non-`Aborted` state and falls into this skip branch — same outcome as the old `!= ExecutingPromptStatus` early return.

1.3. Verify `grep -nE 'prompt\.PromptStatus' pkg/runner/lifecycle.go` returns 0. The file MAY still import `pkg/prompt` for other symbols; only the `prompt.PromptStatus` token must be gone.

## 2. `pkg/promptresumer/resumer.go` — `prepareResume`

2.1. Replace the status skip (line 192). This consumer does NOT call `checker.IsRunning` here. Compute `state := promptstate.InterpretRawTuple(promptstate.LocationInProgress, pf.Frontmatter.Status, pf.Frontmatter.Container, promptstate.DockerStateUnavailable)` and skip (return the existing nil-pf tuple) when `state != promptstate.StateExecuting`. This preserves the exact "only proceed when executing" gate. Keep the subsequent empty-container handling (lines 196-205) verbatim.

2.2. Verify `grep -nE 'prompt\.PromptStatus' pkg/promptresumer/resumer.go` returns 0.

## 3. `pkg/committingrecoverer/recoverer.go` — `Recover`

3.1. Compute the location once: `location := promptstate.LocationInProgress; if filepath.Clean(filepath.Dir(promptPath)) == filepath.Clean(r.completedDir) { location = promptstate.LocationCompleted }`.

3.2. Drive the half-state branch (lines 131-145) off `state := promptstate.InterpretRawTuple(location, pf.Frontmatter.Status, pf.Frontmatter.Container, promptstate.DockerStateUnavailable)`:
- `state == promptstate.StateCompleted` (file already in completed dir) -> `pf.MarkCompleted()` in place + save + the existing `slog.Info("half-state recovery: status transitioned in place", ...)` log — keep verbatim.
- otherwise -> `r.promptManager.MoveToCompleted(ctx, promptPath)` as today.
Keep everything else in `Recover` (dirty-file commit, `autoCompleter` loop, final `CommitCompletedFile`/push) unchanged.

3.3. Confirm `grep -nE 'prompt\.PromptStatus' pkg/committingrecoverer/recoverer.go` returns 0.

## 4. `pkg/queuescanner/scanner.go`

4.1. Replace the pre-execution gate (lines 296-297). The scanner's gate is "candidate must still be pre-execution (idea/draft/approved) after the lock" — this is BROADER than any single canonical state, so route it through the dedicated helper: `if !promptstate.IsPreExecutionStatus(pf.Frontmatter.Status) { ...existing skip log + return... }`.

4.2. Replace the `pr.Status = status` assignment (line 312) so no `prompt.PromptStatus` cast remains in the file: `pr.Status = promptstate.StatusFromRaw(pf.Frontmatter.Status)`.

4.3. Replace line 481 `pf.Frontmatter.Status == string(prompt.PendingVerificationPromptStatus)` with `promptstate.InterpretRawTuple(promptstate.LocationInProgress, pf.Frontmatter.Status, pf.Frontmatter.Container, promptstate.DockerStateUnavailable) == promptstate.StatePendingVerification`.

4.4. Verify `grep -nE 'prompt\.PromptStatus' pkg/queuescanner/scanner.go` returns 0. The `prompt` package may still be imported for `prompt.Prompt`/`prompt.BaseName`; only the `PromptStatus` token must be gone.

## 5. `pkg/cancellationwatcher/watcher.go`

5.1. Replace line 99 `if pf.Frontmatter.Status == string(prompt.CancelledPromptStatus)` with `if promptstate.InterpretRawTuple(promptstate.LocationInProgress, pf.Frontmatter.Status, pf.Frontmatter.Container, promptstate.DockerStateUnavailable) == promptstate.StateCancelled`. Keep the body (cancel log, `close(ch)`, `StopAndRemoveContainer`, `return`) verbatim. This consumer does not consult docker — `DockerStateUnavailable` is correct and `InterpretTuple` does not block on it.

5.2. Verify `grep -nE 'prompt\.PromptStatus' pkg/cancellationwatcher/watcher.go` returns 0.

## 6. Update consumer tests

For each of the five packages, run `go test ./pkg/<pkg>/...` and FIX any test that asserted on the old inline path. Most behaviour is unchanged; where a test injected a specific status to drive resume/reset/cancel/recover, confirm it still produces the same action. Add or adjust at least one test per package that exercises the new `promptstate`-driven branch with the value production traffic takes:
- `lifecycle.go`: an executing prompt whose container `IsRunning` returns `(false, nil)` resets to approved; `(true, nil)` stays executing; `(_, err)` propagates the error.
- `committingrecoverer`: a `committing` prompt whose file is in the completed dir transitions in place; one whose file is in in-progress moves to completed.
- `cancellationwatcher`: a write event where status is `cancelled` stops the container.
Coverage for each modified package must not drop below its current level and must cover the new branches.

## 7. Counterfeiter mocks

Run `go generate ./...`. No consumer interface changed signature, so no mock should change. Verify `git status --porcelain mocks/` is clean.

## 8. CHANGELOG

Append to `## Unreleased` in `/workspace/CHANGELOG.md` ONE bullet:

```
- refactor: migrate the five prompt-state consumers (runner lifecycle, promptresumer, committingrecoverer, queuescanner, cancellationwatcher) to pkg/promptstate.InterpretTuple; remove inline frontmatter-status interpretation (spec 101 prompt 2)
```

</requirements>

<constraints>

- Each consumer's EXTERNAL behaviour stays identical to today (spec Desired Behavior item 3). This is a refactor, not a behaviour change. Do NOT alter the resume/reset/cancel/recover/skip actions.
- `lifecycle.go`'s docker-error policy is INVARIANT: on `checker.IsRunning` error, log and propagate (refuse to reset). Do NOT convert that error into `DockerStateUnavailable` and continue (spec Failure Mode row 7 applies to consumers that DON'T consult docker; lifecycle DOES and must keep failing fast).
- After this prompt, `grep -nE 'prompt\.PromptStatus' pkg/runner/lifecycle.go pkg/promptresumer/resumer.go pkg/committingrecoverer/recoverer.go pkg/queuescanner/scanner.go pkg/cancellationwatcher/watcher.go` MUST return 0 lines (spec AC-3). The `prompt.PromptStatus(...)` conversions live in `pkg/promptstate` helpers.
- Do NOT migrate `pkg/cmd/*`, `pkg/doctor/*`, or `pkg/runner/health_check.go` in this prompt — they are out of scope (CLI/repair tools that read status by design; see prompt 4's open question on the repo-wide grep).
- This prompt does NOT migrate any logging call. Leave all `slog.*` / `log.From(ctx)` calls exactly as they are.
- Frontmatter on disk MUST NOT change; existing prompts under `prompts/in-progress/` and `prompts/completed/` remain readable (spec AC-12). `dark-factory prompt list` must still exit 0.
- No new third-party dependencies (spec Constraint).
- Errors wrapped with `bborbe/errors` — never `fmt.Errorf`, never `context.Background()` in pkg/ non-test code.
- BSD-style license header on every modified file must survive the edit.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# AC 3 — none of the five consumers retain a prompt.PromptStatus token
grep -nE 'prompt\.PromptStatus' pkg/runner/lifecycle.go pkg/promptresumer/resumer.go pkg/committingrecoverer/recoverer.go pkg/queuescanner/scanner.go pkg/cancellationwatcher/watcher.go
# expected: 0 lines

# the conversions now live in the owner package
grep -nE 'func (InterpretRawTuple|IsPreExecutionStatus|StatusFromRaw)' pkg/promptstate/interpret.go
# expected: 3 lines

# build + generate clean (no mock churn)
go build ./... && go generate ./... && git status --porcelain mocks/
# expected: build exit 0; clean mocks/

# per-package tests pass
go test ./pkg/runner/... ./pkg/promptresumer/... ./pkg/committingrecoverer/... ./pkg/queuescanner/... ./pkg/cancellationwatcher/... ./pkg/promptstate/...
# expected: PASS

# AC 12 — existing prompts remain readable
dark-factory prompt list >/dev/null; echo "exit=$?"
# expected: exit=0

# CHANGELOG entry present
grep -n 'spec 101 prompt 2' CHANGELOG.md
# expected: >= 1 line

# full precommit
make precommit
# expected: exit 0
```

</verification>
