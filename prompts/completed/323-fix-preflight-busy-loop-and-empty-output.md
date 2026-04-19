---
status: completed
spec: [055-preflight-baseline-check]
summary: Fixed preflight-failure busy-loop by returning errPreflightSkip sentinel from checkPreflightConditions and handling it in processExistingQueued (via extracted processSingleQueued helper), so the scan loop exits and waits for the next 5s ticker instead of re-scanning immediately
container: dark-factory-323-fix-preflight-busy-loop-and-empty-output
dark-factory-version: v0.128.1-3-gf1cfca3-dirty
created: "2026-04-19T00:00:00Z"
queued: "2026-04-19T19:18:03Z"
started: "2026-04-19T19:42:37Z"
completed: "2026-04-19T19:51:57Z"
---

<summary>
- Fixes a CPU-pegging busy-loop in the daemon when the preflight baseline is broken
- Today, when preflight fails the scan loop treats "skip" as success and immediately rescans the same prompt, spinning at ~6ms/cycle (1.2M log lines observed in production)
- After this fix, a failing preflight exits the scan loop and the daemon waits for the next 5s ticker or watcher event before re-checking
- Surfaces the docker container's stdout/stderr in the ERROR log on preflight failure so operators can diagnose without re-running the command manually
- Adds regression tests: one ensures N scan cycles produce at most N preflight checks (not per-prompt per scan), another ensures the captured output is included in the error returned from preflight `Check`
- Does NOT change the public config (`preflightCommand`, `preflightInterval` remain unchanged)
- Does NOT change the spec-055 contract (per-SHA caching, notifier fires on failure — both preserved)
- Scoped fix: same patch is NOT applied to the other skip conditions (git-index-lock, dirty-files) unless verification shows they busy-loop too
</summary>

<objective>
Stop the preflight-failure busy-loop in `pkg/processor/processor.go` and ensure the docker container's combined stdout+stderr is visible in the `preflight: baseline check FAILED` ERROR log line so a failing baseline can be diagnosed from logs alone.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/prompt-writing.md` for prompt conventions.

Relevant documentation already in the repo:
- `~/.claude-yolo/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — sentinel-error + `errors.Is` pattern (the repo already uses this via `stderrors.Is(contentErr, prompt.ErrEmptyPrompt)` at `pkg/processor/processor.go:1207`)
- `~/.claude-yolo/plugins/marketplaces/coding/docs/go-concurrency-patterns.md` — ticker/for-loop patterns
- `~/.claude-yolo/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo test conventions

Key files and symbols:

- `pkg/processor/processor.go`:
  - `Process` (line 156) — main daemon loop, 5s ticker at line 176, dispatches to `processExistingQueued`
  - `processExistingQueued` (line 491) — inner `for { ... }` scan loop that is the site of the busy-loop
  - Scan-loop tail (lines 542-552): calls `processPrompt`, on nil error logs `"watching for queued prompts"` then loops again
  - `processPrompt` (line 949): first action is `checkPreflightConditions`; on skip it returns `nil` — this is what the caller confuses with "done successfully, look for more"
  - `checkPreflightConditions` (line 926): returns `(true, nil)` for three separate skip conditions — preflight baseline broken, git index lock exists, dirty files over threshold
  - `shouldSkipPrompt` (line 558) — DIFFERENT method, about prompt validation. Do not touch.

- `pkg/preflight/preflight.go`:
  - `Check` (line 93) — logs at line 128 `slog.Error("preflight: baseline check FAILED ...")` with `"output", output`
  - `runInContainer` (line 155) — already calls `cmd.CombinedOutput()` at line 173 and returns `(string(output), err)`. The value flows into `Check`, which on failure passes it to the slog ERROR line as `"output"`. Verify with a real failing command whether the buffer is actually populated — user reports `output=""` in production. The most likely cause is that `errors.Wrap` at line 175 produces an error whose `.Error()` string includes a duplicated prefix but the `output` return value itself is empty because docker was never actually invoked (e.g., docker command not found, or the image pull failed before the command ran). Confirm by reading the wrapped error closely in `runInContainer`. Regardless of root cause: the fix is to ensure whatever bytes docker wrote to stdout/stderr are returned as the `output` string — not just the wrapped error.

- `pkg/processor/processor_internal_test.go`:
  - `fakePreflightChecker` at line 26 — existing test stub; reuse it
  - `Describe("checkPreflightConditions — preflight checker", ...)` at line 1321 — existing preflight tests in the processor suite; add the new busy-loop regression next to it

- `pkg/preflight/preflight_test.go` — existing Ginkgo suite for the preflight package

- `pkg/notifier/notifier.go:5-10` — `Event` struct has four fields: `ProjectName`, `EventType`, `PromptName`, `PRURL`. None of them carry command output. Do NOT add a new payload field in this prompt — out of scope. The "surface output in notifier payload" point from the bug report is reduced to "surface output in the slog ERROR line".

Production evidence to reproduce the first bug:
- Agent project's `.dark-factory.log` grew to 1.2M lines at ~6ms between cycles
- Preflight is currently disabled in dark-factory's own `.dark-factory.yaml` (empty string) as a workaround
- Without the fix, enabling preflight on any project with a broken baseline pegs CPU
</context>

<requirements>

## BUG 1: Fix the preflight busy-loop

### 1.1 Define a sentinel error in `pkg/processor/processor.go`

Add a package-level sentinel error that signals "the current scan cycle cannot proceed — wait for the next ticker or watcher event". Place it near the top of `processor.go` (after imports, before `NewProcessor`):

```go
// errPreflightSkip is returned by processPrompt when the baseline preflight check
// failed and the prompt should NOT be retried within the same scan cycle.
// The caller in processExistingQueued recognizes this sentinel and returns,
// which gives control back to the 5s ticker in Process().
//
// Do NOT use this for the other skip conditions (git-index-lock, dirty-files) —
// those are transient and it is safe to advance to the next prompt in the queue.
var errPreflightSkip = stderrors.New("preflight baseline broken — skip cycle")
```

Use the already-imported `stderrors "errors"` alias at `processor.go:9`. Do NOT add a new import.

### 1.2 Split preflight out of the generic skip check

Refactor `checkPreflightConditions` (line 926) so the preflight-broken case is returned as a distinct sentinel error, not as `(skip=true, nil)`. The git-lock and dirty-files branches keep their existing `(true, nil)` behavior because they are cheap, transient, and not observed to busy-loop.

```go
// Before:
func (p *processor) checkPreflightConditions(ctx context.Context) (bool, error) {
    if p.preflightChecker != nil {
        ok, err := p.preflightChecker.Check(ctx)
        if err != nil {
            slog.Warn("preflight checker error, skipping prompt this cycle", "error", err)
            return true, nil
        }
        if !ok {
            slog.Info("preflight: baseline broken — prompt stays queued until baseline is fixed")
            return true, nil
        }
    }

    if p.checkGitIndexLock() {
        slog.Warn("git index lock exists, skipping prompt — will retry next cycle")
        return true, nil
    }
    return p.checkDirtyFileThreshold(ctx)
}

// After:
func (p *processor) checkPreflightConditions(ctx context.Context) (bool, error) {
    if p.preflightChecker != nil {
        ok, err := p.preflightChecker.Check(ctx)
        if err != nil {
            slog.Warn("preflight checker error, skipping cycle", "error", err)
            return false, errPreflightSkip
        }
        if !ok {
            slog.Info("preflight: baseline broken — prompt stays queued until baseline is fixed")
            return false, errPreflightSkip
        }
    }

    if p.checkGitIndexLock() {
        slog.Warn("git index lock exists, skipping prompt — will retry next cycle")
        return true, nil
    }
    return p.checkDirtyFileThreshold(ctx)
}
```

Rationale: preflight runs a docker container that can take seconds. Calling it every ~6ms is the bug. Returning `errPreflightSkip` propagates up and breaks the scan-loop.

### 1.3 Propagate the sentinel through `processPrompt`

In `processPrompt` at line 949-954, replace:

```go
// Before:
if skip, err := p.checkPreflightConditions(ctx); err != nil {
    return errors.Wrap(ctx, err, "check preflight conditions")
} else if skip {
    return nil // skip this cycle, re-check on next poll
}

// After:
if skip, err := p.checkPreflightConditions(ctx); err != nil {
    if stderrors.Is(err, errPreflightSkip) {
        return err // propagate sentinel unwrapped so caller can recognize it
    }
    return errors.Wrap(ctx, err, "check preflight conditions")
} else if skip {
    return nil // transient skip (git lock / dirty files) — advance to next prompt
}
```

IMPORTANT: do NOT wrap `errPreflightSkip` with `errors.Wrap`. Return the bare sentinel — it keeps the intent clear and the `stderrors.Is` check at the caller trivial. Verify via the new regression test (see §3.1).

### 1.4 Exit the scan loop in `processExistingQueued` on the sentinel

In `processExistingQueued` at lines 542-552, handle the sentinel specially:

```go
// Before:
if err := p.processPrompt(ctx, pr); err != nil {
    if stopErr := p.handleProcessError(ctx, pr.Path, err); stopErr != nil {
        return stopErr
    }
    continue // re-queued or permanently failed — process next prompt
}

slog.Info("watching for queued prompts", "dir", p.queueDir)

// Loop again to process next prompt

// After:
if err := p.processPrompt(ctx, pr); err != nil {
    if stderrors.Is(err, errPreflightSkip) {
        // Baseline is broken. Exit the scan loop and wait for the next
        // 5s tick or watcher event in Process() before re-checking.
        // Do NOT call handleProcessError — this is not a prompt failure.
        return nil
    }
    if stopErr := p.handleProcessError(ctx, pr.Path, err); stopErr != nil {
        return stopErr
    }
    continue // re-queued or permanently failed — process next prompt
}

slog.Info("watching for queued prompts", "dir", p.queueDir)

// Loop again to process next prompt
```

Return `nil` (not the sentinel) so the outer `Process()` loop at lines 190-202 continues running. The daemon MUST keep running after a preflight failure — only the inner scan loop exits.

### 1.5 Verify the two non-preflight skip conditions don't busy-loop

Read `checkGitIndexLock` (line 920) and `checkDirtyFileThreshold` (line 899) carefully. Both are cheap local checks (stat a file, count files) — they do NOT invoke docker or anything that takes > 1ms. Even if they busy-loop at ~6ms/cycle, they produce one `slog.Warn` line per scan (not per retry), and the queue advances naturally once the next prompt in alphabetical order is not blocked. No fix needed. Leave them returning `(true, nil)`.

If during implementation you find either of them DOES busy-loop with heavy logs, stop and report — do not expand scope without evidence.

## BUG 2: Already resolved by host-exec pivot

The original BUG 2 ("output=`""` on preflight failure") was rooted in the claude-yolo container's ENTRYPOINT exiting 4 with no stdout/stderr whenever CMD args it does not recognize are passed. We removed container execution from preflight entirely: `runInContainer` now runs `sh -c <command>` on the host with `cmd.Dir = c.projectRoot`. `CombinedOutput()` on a host shell reliably captures stdout and stderr, so the `output` value naturally populates.

Do NOT re-introduce docker execution in this prompt. Do NOT modify the notifier `Event` struct (out of scope).

## Regression tests

### 3.1 Processor test — sentinel contract

Add a new `It` block inside the existing `Describe("checkPreflightConditions — preflight checker", ...)` at `processor_internal_test.go:1321` (or a sibling `Describe` right after, which ends at line 1363). Use the existing `fakePreflightChecker` at line 26 and the already-exported `CheckPreflightConditions` seam from `pkg/processor/export_test.go`.

Test shape:

```go
It("returns errPreflightSkip when preflight checker returns false", func() {
    fakeChecker := &fakePreflightChecker{ok: false}
    // Build a processor with only what checkPreflightConditions reads.
    proc := &processor{}
    proc.SetPreflightChecker(fakeChecker)

    skip, err := proc.CheckPreflightConditions(context.Background())

    Expect(err).To(HaveOccurred())
    Expect(stderrors.Is(err, processor.PreflightSkipSentinel)).To(BeTrue())
    Expect(skip).To(BeFalse())
    Expect(fakeChecker.callCount).To(Equal(1))
})
```

Export the sentinel via `pkg/processor/export_test.go` (which already exists and exports `SetPreflightChecker` and `CheckPreflightConditions`) — add:

```go
var PreflightSkipSentinel = errPreflightSkip
```

If `fakePreflightChecker` at line 26 does not already track `callCount`, add it (one int field + `c.callCount++` in `Check`). Grep its current definition first.

This test proves the sentinel contract. It does NOT replay the full scan-loop — that is covered indirectly by requirement §1.4 plus the manual acceptance check in `<verification>`. Do not add an integration test that spins up a real queue dir unless §1.4 review leaves doubt.

### 3.2 Preflight test — output is surfaced via the cache entry

Use the already-exported `NewCheckerWithRunner` seam in `pkg/preflight/export_test.go` (which takes `(command, interval, notifier, projectName, headSHA, runner)`). Do NOT add a new `SetRunner` or `NewCheckerForTest` — the existing seam is sufficient.

Per the `Check` contract (preflight.go:93-139), when the runner returns an error `Check` absorbs it, caches the `output` on the failure entry, and returns `(false, nil)`. The test asserts the cache entry received the runner output:

```go
It("retains runner output in the cache entry when baseline check fails", func() {
    n := &mocks.Notifier{}
    runner := func(ctx context.Context) (string, error) {
        return "FAIL: assertion failed at foo_test.go:12", errors.New("exit status 1")
    }
    c := preflight.NewCheckerWithRunner("make test", 0, n, "proj", "abc123", runner)

    ok, err := c.Check(context.Background())
    Expect(err).NotTo(HaveOccurred())
    Expect(ok).To(BeFalse())
    // Calling notifier must have received the failure event.
    Expect(n.NotifyCallCount()).To(Equal(1))
})
```

No second test is needed — `runInContainer` now runs on host via `sh -c`, and `CombinedOutput()` reliably captures stdout/stderr. The cache-entry assertion above is sufficient.

## 4. Run precommit

```bash
make precommit
```

All tests must pass, including both new regressions. If any existing test relies on preflight-failure returning `(true, nil)` from `checkPreflightConditions`, update it to assert the new sentinel contract — but first confirm it exists by searching:

```bash
grep -rn 'checkPreflightConditions\|CheckPreflightConditions' pkg/
```

The existing `Describe("checkPreflightConditions — preflight checker", ...)` at `processor_internal_test.go:1321` has tests that will break:

- `"returns skip=true when preflight checker returns false"` (line 1335) — now must assert `(false, errPreflightSkip)` (or `errors.Is(err, errPreflightSkip) == true`).
- `"returns skip=true when preflight checker returns an error (non-fatal)"` (line 1349) — now must assert `errors.Is(err, errPreflightSkip) == true`. This test also asserts the error is "absorbed, not propagated" — that line of the assertion must change. Update the `It` description to match: `"returns errPreflightSkip when preflight checker returns false"`.

Keep the `"returns skip=false when preflight checker returns true"` test (line 1342) and `"returns skip=false when no preflight checker is set (nil)"` test (line 1357) — both still pass unchanged.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT change the public config: `preflightCommand` and `preflightInterval` stay as-is in `pkg/config/config.go`
- Do NOT change spec-055 contract: per-SHA caching in `preflight.checker.cache` must still work; the notifier must still fire on failure via `c.notifier.Notify(ctx, notifier.Event{EventType: "preflight_failed"})`
- Do NOT apply the sentinel-return pattern to git-index-lock or dirty-files checks — those are cheap transient checks; expanding scope without evidence of a busy-loop is prohibited
- Do NOT modify `pkg/notifier/notifier.go` — adding a payload field to `Event` is out of scope for this prompt
- Do NOT touch `go.mod` / `go.sum` / `vendor/`
- Do NOT add new third-party imports. Use `stderrors "errors"` (already imported in processor.go) and `github.com/bborbe/errors` (already imported in both files)
- Keep the diff minimal: two production files change (`pkg/processor/processor.go`, `pkg/preflight/preflight.go`) plus their test files and test seams (`export_test.go` in each package if not already present)
- Existing tests must pass. The two tests named in requirement §4 must be updated to match the new contract; no other test should need to change
- Use `make precommit` for verification — not `go build` or `go test` alone
</constraints>

<verification>

Run full precommit:

```bash
make precommit
```

Targeted spot checks:

```bash
# Processor package — run the preflight + busy-loop tests
go test -v -run 'TestProcessor' ./pkg/processor/... 2>&1 | grep -E 'preflight|busy-loop|PASS|FAIL'

# Preflight package — run the new output-surface test
go test -v -run 'TestPreflight' ./pkg/preflight/... 2>&1 | grep -E 'surfaces|output|PASS|FAIL'
```

Manual acceptance check (no automated test covers this, but it demonstrates the fix):

```bash
# In a scratch project with a broken baseline (e.g., `preflightCommand: "false"`),
# run the daemon for 30 seconds and count preflight invocations:
dark-factory --project /tmp/broken &
DF=$!
sleep 30
kill $DF
grep -c "preflight: running baseline check" /tmp/broken/.dark-factory.log
# Expected: ≤ 7 (once at startup, then once every 5s for 30s = 6)
# Before fix: hundreds or thousands
```

Acceptance — the following must all be true after this change:

1. `pkg/processor/processor.go` defines `errPreflightSkip` as a package-level sentinel
2. `checkPreflightConditions` returns `(false, errPreflightSkip)` when the preflight checker reports failure or an internal error
3. `processPrompt` returns the bare sentinel (not wrapped by `errors.Wrap`) so `stderrors.Is` works at the caller
4. `processExistingQueued` recognizes `errPreflightSkip` via `stderrors.Is` and returns `nil` to exit the scan loop
5. The git-index-lock and dirty-files branches of `checkPreflightConditions` still return `(true, nil)` — unchanged
6. `pkg/preflight/preflight.go` `runInContainer` includes the captured output (truncated to ≤ 4KiB) in the wrapped error returned on non-zero exit
7. `slog.Error("preflight: baseline check FAILED ...")` in `Check` still logs the full `output` as a structured attribute — unchanged
8. The `notifier.Event` struct is unchanged
9. Two new regression tests pass: the processor sentinel test and the preflight output-surface test
10. `make precommit` passes clean

</verification>
