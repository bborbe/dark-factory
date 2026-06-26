---
status: approved
spec: ["101"]
created: "2026-06-26T08:02:00Z"
queued: "2026-06-26T08:00:41Z"
---

<summary>

- Confirms the previously-undocumented "pending verification" state is a real, first-class member of the shared state vocabulary (declared in prompt 1) and that the core orchestrator treats it that way.
- Routes the orchestrator's one remaining inline status comparison (the cancelled-during-execution fallback check) through the shared owning package, so the orchestrator no longer interprets a raw status string itself.
- Leaves the orchestrator's behaviour unchanged: a prompt cancelled mid-run is still detected and handled exactly as before; the verification-gate pause still works the same way.
- Keeps the on-disk format untouched — "pending_verification" remains the same string written to frontmatter.
- After this prompt the core orchestrator file contains no inline frontmatter-status interpretation, clearing it for the later precommit gate.

</summary>

<objective>
Promote `pending_verification` to a first-class state in the shared state machine (the constant was added in prompt 1) and migrate the one remaining inline frontmatter-status comparison in `pkg/processor/processor.go` (the cancelled-during-execution fallback at `runContainer`) to go through `pkg/promptstate`. The processor's external behaviour stays identical.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — error wrapping, GoDoc.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega, coverage.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — changelog format.

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/101-extract-unified-prompt-state-machine.md` — Desired Behavior item 4; Constraints; Acceptance Criteria 2, 4.

Prompt 1's deliverable MUST be on the tree. If `pkg/promptstate/state.go` lacks the `StatePendingVerification` constant, STOP and report `Status: failed` with message "StatePendingVerification not declared (prompt 1)". Read it:
- `/workspace/pkg/promptstate/state.go` — confirm `StatePendingVerification State = "pending_verification"` exists and is in `AvailableStates`.
- `/workspace/pkg/promptstate/interpret.go` — confirm `InterpretTuple` maps `prompt.PendingVerificationPromptStatus -> StatePendingVerification`, and confirm the `InterpretRawTuple` helper exists (added in prompt 2). If `InterpretRawTuple` is absent, STOP and report `Status: failed` with message "InterpretRawTuple not deployed (prompt 2)".

Read the source file END-TO-END before editing:
- `/workspace/pkg/processor/processor.go` — esp. `runContainer` (line ~407) and its cancelled-fallback at line 442 (`pf.Frontmatter.Status == string(prompt.CancelledPromptStatus)`), and `enterPendingVerification` (line ~465) which calls `pf.MarkPendingVerification()` (a WRITER — leave it untouched; it is not tuple interpretation).
- `/workspace/pkg/processor/processor_suite_test.go` and the processor tests — match Ginkgo style.

VERIFIED FACTS (do not re-derive):
- `pkg/processor/processor.go` already imports `prompt "github.com/bborbe/dark-factory/pkg/prompt"`. After this edit it may still import it for `prompt.ContainerName`, `prompt.PromptFile`, etc. — only the `prompt.CancelledPromptStatus`/`prompt.PromptStatus` TOKENS in tuple-reading positions must go.
- The ONLY inline tuple-interpretation in `pkg/processor` non-test files is the cancelled-fallback at line 442: `pf.Frontmatter.Status == string(prompt.CancelledPromptStatus)`. `enterPendingVerification` writes status via `pf.MarkPendingVerification()` and does NOT read/interpret a tuple — do NOT change it.
- `pending_verification` is ALREADY a real on-disk status (`pkg/prompt/prompt.go` line 69: `PendingVerificationPromptStatus="pending_verification"`) and already in `AvailablePromptStatuses` and the on-disk transition table (`InReviewPromptStatus: {PendingVerificationPromptStatus, ...}` and `PendingVerificationPromptStatus: {CompletedPromptStatus, FailedPromptStatus}`). The spec's "promote to first-class" work is: (a) the `StatePendingVerification` constant in `promptstate` (done in prompt 1), and (b) routing the processor's inline read through `promptstate` (this prompt). No on-disk change is needed.

</context>

<requirements>

## 1. Migrate the cancelled-during-execution fallback in `runContainer`

In `/workspace/pkg/processor/processor.go`, `runContainer`, line ~442, replace:

```go
if pf, loadErr := p.promptManager.Load(ctx, promptPath); loadErr == nil &&
	pf.Frontmatter.Status == string(prompt.CancelledPromptStatus) {
```

with a `promptstate`-driven check (import `promptstate "github.com/bborbe/dark-factory/pkg/promptstate"`):

```go
if pf, loadErr := p.promptManager.Load(ctx, promptPath); loadErr == nil &&
	promptstate.InterpretRawTuple(promptstate.LocationInProgress, pf.Frontmatter.Status, pf.Frontmatter.Container, promptstate.DockerStateUnavailable) == promptstate.StateCancelled {
```

Keep the body (the `log.From(ctx).Info("prompt cancelled", "workflow_step", "cancel")` and `return true, nil`) verbatim. The `prompts/in-progress/` location and `DockerStateUnavailable` are correct: the processor is not consulting docker here; it is reading the file's ground-truth cancel marker, which `InterpretTuple` maps to `StateCancelled` regardless of docker.

Decision already made — do NOT route `enterPendingVerification` through `promptstate`. It is a state-WRITE (`MarkPendingVerification`), not a tuple read. Leave it exactly as-is.

## 2. Confirm `StatePendingVerification` is first-class (AC-2, AC-4)

No new code is needed if prompt 1 declared the constant correctly. Add (if not already present from prompt 1) a focused test in `pkg/promptstate` asserting `InterpretTuple(LocationInProgress, prompt.PendingVerificationPromptStatus, "", DockerStateUnavailable) == StatePendingVerification` and that `AvailableStates.Contains(StatePendingVerification)` is true. If prompt 1 already covers this, SKIP — do not duplicate.

## 3. Verify the processor is now token-free

After the edit, `grep -nE 'prompt\.PromptStatus|prompt\.CancelledPromptStatus|prompt\.[A-Za-z]+PromptStatus' pkg/processor/*.go | grep -v _test` MUST return 0 lines for tuple-reading tokens. (If any `prompt.XxxPromptStatus` token remains in a WRITE position — there should be none, since writes go through `MarkXxx` methods — surface it in `## Improvements`.)

## 4. Tests

4.1. Update/confirm `pkg/processor` tests covering `runContainer`'s cancelled-fallback path: a test where the container `Execute` returns an error AND the re-loaded prompt has `frontmatter.status == cancelled` must still yield `cancelled=true, err=nil` (same as before). Add the test if missing — exercise the exact path production traffic takes (load returns a `PromptFile` with `Status: "cancelled"`).

4.2. Run `go test ./pkg/processor/... ./pkg/promptstate/...` — PASS. Coverage for `pkg/processor` must not drop below current; cover the migrated branch.

## 5. Counterfeiter mocks

Run `go generate ./...`. No interface signature changed — `git status --porcelain mocks/` must be clean.

## 6. CHANGELOG

Append to `## Unreleased` in `/workspace/CHANGELOG.md` ONE bullet:

```
- refactor: route the processor cancelled-during-execution fallback through pkg/promptstate; pending_verification is now a first-class promptstate.State (spec 101 prompt 3)
```

</requirements>

<constraints>

- The processor's external behaviour stays identical (spec Desired Behavior item 4). The verification-gate pause and the cancelled-fallback detection behave exactly as today.
- `pending_verification` keeps its on-disk string value `"pending_verification"` — no frontmatter change (spec Constraint, Non-goal).
- Do NOT change `enterPendingVerification` or `pf.MarkPendingVerification()` — those are state writes, not tuple interpretation.
- After this prompt, `pkg/processor` non-test files contain no inline `prompt.XxxPromptStatus` tuple-reading token (prerequisite for prompt 4's gate covering `pkg/processor`).
- This prompt does NOT migrate any logging call — leave `log.From(ctx)` / `slog.*` calls as-is.
- No new third-party dependencies (spec Constraint).
- Errors wrapped with `bborbe/errors` — never `fmt.Errorf`, never `context.Background()` in pkg/ non-test code.
- BSD-style license header on every modified file must survive the edit.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# AC 4 (partial) — processor non-test files have no inline status-token tuple read
grep -nE 'prompt\.[A-Za-z]*PromptStatus' pkg/processor/*.go | grep -v _test.go
# expected: 0 lines

# AC 2 — StatePendingVerification is first-class
grep -nE 'StatePendingVerification' pkg/promptstate/state.go
# expected: >= 1 line (constant declared + in AvailableStates)

# build + generate clean
go build ./... && go generate ./... && git status --porcelain mocks/
# expected: build exit 0; clean mocks/

# tests pass
go test ./pkg/processor/... ./pkg/promptstate/...
# expected: PASS

# CHANGELOG entry present
grep -n 'spec 101 prompt 3' CHANGELOG.md
# expected: >= 1 line

# full precommit
make precommit
# expected: exit 0
```

</verification>
