---
status: completed
summary: Added pf.Frontmatter.LastFailReason = "" to MarkCompleted so stale failure reasons are cleared when a prompt transitions to completed status, with four regression tests covering success-after-failure, pristine success, second-failure-replaces, and in-memory clear.
container: dark-factory-301-clear-fail-reason-on-success
dark-factory-version: v0.111.2
created: "2026-04-16T00:00:00Z"
queued: "2026-04-16T20:31:04Z"
started: "2026-04-16T20:31:06Z"
completed: "2026-04-16T20:35:03Z"
---
<summary>
- Fixes a stale-data bug: prompts that fail once, get retried, and then succeed currently keep the old `lastFailReason` field in their frontmatter forever after being moved to `completed/`
- The field is intended to record WHY the most recent attempt failed; on a successful completion it should not be present
- Reproducer: `003-test-build-info-metrics` in `go-skeleton/prompts/completed/` shows `lastFailReason: 'execute prompt: docker run failed: wait command: exit status 128'` despite `status: completed`
- Fix is a one-liner in `MarkCompleted` on `PromptFile` — clear the field when transitioning to completed status
- Failure path is unchanged: a fresh `SetLastFailReason` on a re-failure still overwrites any previous value
- Pristine success (never failed) is a no-op because the field was already empty
- Adds regression tests for all three paths (success-after-failure clears, fail-retry-fail replaces, pristine success untouched)
</summary>

<objective>
When a prompt transitions to `status: completed`, ensure the `lastFailReason` frontmatter field is removed (the YAML tag is `lastFailReason,omitempty`, so clearing the Go string to `""` causes it to be omitted on save). Stale failure reasons from a prior failed attempt must not persist into the completed record.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions: `github.com/bborbe/errors` wrapping (no `fmt.Errorf`, no bare `return err`), Ginkgo/Gomega tests, `libtime.CurrentDateTimeGetter` for injected time.

Relevant coding guides: `go-error-wrapping-guide.md`, `go-testing-guide.md` (both at `~/.claude/plugins/marketplaces/coding/docs/`).

Read these files in full before editing:

- `pkg/prompt/prompt.go` — specifically:
  - `Frontmatter` struct (line ~180): `LastFailReason string ` with tag `yaml:"lastFailReason,omitempty"` (line 194). Because of `omitempty`, setting this to `""` causes the field to be omitted entirely when the file is serialized back to disk.
  - `MarkCompleted` (line 353–357): currently sets status + completed timestamp only. **This is where the fix goes.**
  - `MarkFailed` (line 359–363): unchanged — do not touch.
  - `SetLastFailReason` (line 365–368): unchanged — still the one writer of the field on the failure path.
  - `MoveToCompleted` / helper that calls `pf.MarkCompleted()` at line ~959 — no change needed here, it already calls `MarkCompleted` + `Save` in the right order.

- `pkg/processor/processor.go`:
  - `handlePromptFailure` at line 652–693 — calls `SetLastFailReason(err.Error())` then either re-queues via `MarkApproved` (retry) or `MarkFailed` (exhausted). **No change needed here.**
  - Other `SetLastFailReason` callers: `health_check.go:196` (maxPromptDuration exceeded), `processor.go:361` (reattach timeout). **No change needed.**

- `pkg/prompt/prompt_test.go` — existing Ginkgo suite. Note:
  - `Describe("PromptFile.SetLastFailReason", ...)` at line 2675.
  - `Describe("Frontmatter without lastFailReason", ...)` at line 2690.
  - Add the new `MarkCompleted`-clears-LastFailReason tests adjacent to these.
  - The file is `package prompt_test` and already imports `libtime "github.com/bborbe/time"`.

Bug reproduction:
- Project: `~/Documents/workspaces/go-skeleton`
- File: `prompts/completed/003-test-build-info-metrics.md`
- Frontmatter contains BOTH `status: completed` AND `lastFailReason: 'execute prompt: docker run failed: wait command: exit status 128'`
- The successful retry saved the completed status but never cleared the stale failure reason.
</context>

<requirements>

## 1. Clear `LastFailReason` in `MarkCompleted`

Edit `pkg/prompt/prompt.go`. The current implementation is:

```go
// MarkCompleted sets status to completed with timestamp.
func (pf *PromptFile) MarkCompleted() {
    pf.Frontmatter.Status = string(CompletedPromptStatus)
    pf.Frontmatter.Completed = pf.now().UTC().Format(time.RFC3339)
}
```

Change it to also clear `LastFailReason`:

```go
// MarkCompleted sets status to completed with timestamp and clears any
// previously recorded lastFailReason so a successful retry leaves no stale
// failure data in the frontmatter. The YAML tag is lastFailReason,omitempty
// so the field is dropped entirely from the serialised file when empty.
func (pf *PromptFile) MarkCompleted() {
    pf.Frontmatter.Status = string(CompletedPromptStatus)
    pf.Frontmatter.Completed = pf.now().UTC().Format(time.RFC3339)
    pf.Frontmatter.LastFailReason = ""
}
```

This is the entire production-code change. Do not touch `MarkFailed`, `SetLastFailReason`, `handlePromptFailure`, the retry command, or any other code path.

## 2. Do NOT change any other behaviour

- `MarkFailed` must continue to leave `LastFailReason` in place (the caller `handlePromptFailure` calls `SetLastFailReason` BEFORE `MarkFailed`, so the field is populated; the failed prompt's frontmatter must retain the reason).
- `MarkApproved` (called on retry re-queue) must not clear the field — we want "lazy clearing on success" semantics per the spec. The previous reason stays visible on a re-queued prompt until it either succeeds (cleared by this fix) or fails again (replaced by the new `SetLastFailReason` call on the next failure).
- Do NOT add a `ClearLastFailReason` public method. The clear is an implementation detail of `MarkCompleted`.
- Do NOT change the `yaml:"lastFailReason,omitempty"` tag — it is already correct.

## 3. Regression tests — `pkg/prompt/prompt_test.go`

Add a new `Describe("PromptFile.MarkCompleted clears LastFailReason", ...)` block adjacent to the existing `Describe("PromptFile.SetLastFailReason", ...)` (line ~2675). Use the existing `tempDir` / `ctx` / `libtime.NewCurrentDateTime()` setup pattern from neighbouring tests in the same file.

Required cases (one `It` each):

1. **success after failure clears the field**: write a prompt file whose frontmatter already contains `lastFailReason: "docker exit 128"` and `status: failed`. Call `prompt.Load`, call `pf.MarkCompleted()`, call `pf.Save(ctx)`, then re-load the file from disk and assert:
   - `pf2.Frontmatter.Status == "completed"`
   - `pf2.Frontmatter.LastFailReason == ""` (empty in memory)
   - The raw file contents on disk do NOT contain the substring `lastFailReason` (because `omitempty` drops the field). Read the file with `os.ReadFile` and assert with `Expect(string(raw)).NotTo(ContainSubstring("lastFailReason"))`.

2. **pristine success leaves frontmatter clean**: write a prompt file with `status: approved` and NO `lastFailReason` field. Call `MarkCompleted`, `Save`, re-load. Assert:
   - `pf2.Frontmatter.Status == "completed"`
   - `pf2.Frontmatter.LastFailReason == ""` (was empty, still empty).
   - Raw file contents do not contain `lastFailReason`.

3. **second failure replaces the old reason (verifies the failure path is untouched)**: write a prompt file with `status: approved` and `lastFailReason: "first reason"`. Call `pf.SetLastFailReason("second reason")`, call `pf.MarkFailed()`, `Save`, re-load. Assert:
   - `pf2.Frontmatter.Status == "failed"`
   - `pf2.Frontmatter.LastFailReason == "second reason"` (old reason replaced, not appended).
   - Raw file contents contain `second reason` and do NOT contain `first reason`.

4. **in-memory clear without Save** (sanity): construct a `PromptFile`, set `pf.Frontmatter.LastFailReason = "stale"`, call `pf.MarkCompleted()`, assert `pf.Frontmatter.LastFailReason == ""` immediately (no Save/Load round-trip). This is the unit-level assertion; cases 1–3 are the end-to-end round-trip assertions.

Use the existing test-file patterns: `os.WriteFile` to write the initial frontmatter, `prompt.Load(ctx, path, libtime.NewCurrentDateTime())` to load, `pf.Save(ctx)` to persist, `os.ReadFile` to inspect the raw serialised file. No new fakes, no new interfaces.

## 4. Verification

Run `make precommit` in `/workspace` — must exit 0. The four new `It` blocks added in requirement 3 are the authoritative regression gate.

Smoke-check the fix against the original reproducer: construct a temp file whose frontmatter matches `prompts/completed/003-test-build-info-metrics.md` (status + lastFailReason both present), call `Load` → `MarkCompleted` → `Save`, read the file, confirm `lastFailReason:` is absent from the output. This can be the body of test case 1 if you prefer — call it out in the `It` description so reviewers can see the link back to the original bug.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Wrap all non-nil errors with `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`. (No new error paths in production code for this fix; this applies to any test helpers you add.)
- Keep error messages lowercase, no file paths in the message.
- No new exported names. The fix is a single-line addition inside the existing `MarkCompleted`.
- Do NOT touch `MarkFailed`, `SetLastFailReason`, `handlePromptFailure`, `MarkApproved`, the retry/requeue CLI command, or any executor/factory code. The fix lives exclusively in `MarkCompleted`.
- Do NOT change the `yaml:"lastFailReason,omitempty"` tag.
- Do NOT add a `ClearLastFailReason` public method — the clear is an internal detail of `MarkCompleted`.
- Do NOT touch `go.mod` / `go.sum` / `vendor/`.
- Existing tests must still pass — specifically the existing `Describe("PromptFile.SetLastFailReason", ...)` and `Describe("Frontmatter without lastFailReason", ...)` suites must remain green unmodified.
</constraints>

<verification>
1. `cd /workspace && make precommit` must exit 0.
2. Diff check: `git diff pkg/prompt/prompt.go` shows exactly one added line inside `MarkCompleted` (`pf.Frontmatter.LastFailReason = ""`) plus the updated doc comment. No other production-code changes.
3. The new `Describe("PromptFile.MarkCompleted clears LastFailReason", ...)` suite in `pkg/prompt/prompt_test.go` contains at least 4 `It` blocks covering: success-after-failure clears, pristine success untouched, second failure replaces, in-memory clear.
4. Case 1 (success-after-failure clears) must fail on `master` before the fix and pass after; cases 2–4 must pass both before and after (they exercise paths that are already correct).
5. `grep -n "lastFailReason" pkg/prompt/prompt.go pkg/processor/processor.go pkg/runner/health_check.go` — confirms no new writer of the field was introduced; `SetLastFailReason` remains the only setter.
</verification>
