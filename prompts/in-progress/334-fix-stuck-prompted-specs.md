---
status: committing
summary: Fixed spec auto-complete phase ordering in workflow_executor_direct.go (MoveToCompleted now runs before CheckAndComplete), added a 60-second self-healing sweep ticker in processor.go, added regression tests for both fixes, and updated CHANGELOG.md.
container: dark-factory-334-fix-stuck-prompted-specs
dark-factory-version: v0.132.0
created: "2026-04-25T13:42:00Z"
queued: "2026-04-25T11:49:27Z"
started: "2026-04-25T11:53:01Z"
---

<summary>
- The direct-workflow executor (`pkg/processor/workflow_executor_direct.go`) calls `CheckAndComplete` BEFORE `MoveToCompleted`, so when the last prompt of a spec finishes, `allLinkedPromptsCompleted` still sees that prompt as not-yet-completed and the spec stays in `prompted` forever
- Primary fix: swap the two phases so the prompt is moved to `prompts/completed/` first, then auto-complete is checked — same order already used by `moveToCompletedAndCommit` in `workflow_helpers.go`
- Belt-and-suspenders: add a slow (60-second) auto-complete sweep ticker so any missed transition (daemon crash mid-completion, future regression, race) is auto-corrected within ~1 minute without requiring a daemon restart — separate from the 5-second queue ticker so the more expensive cross-dir scan doesn't run on every queue poll
- Add regression tests in `workflow_executor_direct_test.go` that spawn a fake spec + single linked prompt, run the direct executor, and assert the spec transitions to `verifying` without a daemon restart
- No public API changes; no config changes; no impact on PR/branch/clone workflows
</summary>

<objective>
Fix the spec auto-completion ordering bug in the direct workflow so a spec transitions from `prompted` → `verifying` immediately after its last linked prompt completes, without requiring a daemon restart.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega patterns.

Read these files before editing:
- `pkg/processor/workflow_executor_direct.go` — the buggy file. Phases 2 (CheckAndComplete, lines 73–78) and 3 (MoveToCompleted, lines 80–84) are in the wrong order.
- `pkg/processor/workflow_helpers.go::moveToCompletedAndCommit` (lines 41–63) — the correct reference order: MoveToCompleted FIRST, then CheckAndComplete.
- `pkg/spec/spec.go::CheckAndComplete` (lines 386–429) and `allLinkedPromptsCompleted` (lines 442+) — confirms the cause: it scans `prompts/in-progress/` and `prompts/completed/` and only counts a prompt as "done" when it's physically in the completed dir.

### Bug repro

Today (after current direct executor):

```
Phase 2: CheckAndComplete(spec_058) → spec scans prompts; current prompt
         still in prompts/in-progress/ with status=committing → returns
         allCompleted=false → spec stays at "prompted"
Phase 3: MoveToCompleted          → file now in prompts/completed/, but
         no further CheckAndComplete call fires
```

After fix (swap):

```
Phase 2: MoveToCompleted          → file now in prompts/completed/
Phase 3: CheckAndComplete(spec_058) → all linked prompts found in
         completed dir → spec transitions to "verifying"
```

This is exactly the order `workflow_helpers.go::moveToCompletedAndCommit` uses (lines 50–58). The direct executor regressed.

We hit this bug twice in one day (specs 057 + 058) — both required `kill $(cat .dark-factory.lock) && dark-factory daemon` to push the spec forward. A real fix saves manual restarts.
</context>

<requirements>

## 1. Fix `pkg/processor/workflow_executor_direct.go`

In the `completeCommit` method (line 61 — `func (e *directWorkflowExecutor) completeCommit(...)`), swap the order of Phase 2 (auto-complete specs) and Phase 3 (move prompt to completed). The four phases must execute in this order:

1. **Commit work files** (Phase 1, unchanged)
2. **MoveToCompleted** — move prompt to `prompts/completed/` and set its status (was Phase 3, now Phase 2)
3. **CheckAndComplete** — best-effort, non-blocking auto-complete of every linked spec (was Phase 2, now Phase 3)
4. **CommitCompletedFile** — commit the prompt-file move with retry (Phase 4, unchanged)

The relevant block in the current file (around lines 73–84):

```go
// Phase 2: auto-complete specs (best-effort, non-blocking).
for _, specID := range pf.Specs() {
    if err := e.deps.AutoCompleter.CheckAndComplete(ctx, specID); err != nil {
        slog.Warn("spec auto-complete failed", "spec", specID, "error", err)
    }
}

// Phase 3: move prompt to completed/ (sets status: completed, physically moves the file).
if err := e.deps.PromptManager.MoveToCompleted(ctx, promptPath); err != nil {
    return errors.Wrap(ctx, err, "move to completed")
}
slog.Info("moved to completed", "file", filepath.Base(promptPath))
```

Becomes:

```go
// Phase 2: move prompt to completed/ (sets status: completed, physically moves the file).
if err := e.deps.PromptManager.MoveToCompleted(ctx, promptPath); err != nil {
    return errors.Wrap(ctx, err, "move to completed")
}
slog.Info("moved to completed", "file", filepath.Base(promptPath))

// Phase 3: auto-complete specs (best-effort, non-blocking).
// Must run AFTER MoveToCompleted so allLinkedPromptsCompleted can see this
// prompt in the completed dir.
for _, specID := range pf.Specs() {
    if err := e.deps.AutoCompleter.CheckAndComplete(ctx, specID); err != nil {
        slog.Warn("spec auto-complete failed", "spec", specID, "error", err)
    }
}
```

The Phase 4 commit block following these stays unchanged.

## 2. Update Phase 1 commit message comment if it references phase numbers

Read the lines just before the section above. If a comment says "Phase 1: commit work files" and the next phase is now "Phase 2: move", that's still accurate (we only swapped 2 and 3). No additional comment updates needed unless an existing comment misnumbers the new order.

## 3. Belt-and-suspenders: separate slow ticker for auto-complete sweep

`checkPromptedSpecs` scans `specs/in-progress/` and, for each prompted spec, scans both prompt directories. That cost should not run on every 5-second queue tick. Use a **separate, slower ticker** for the sweep.

First, declare a package-level `sweepInterval` variable near the top of `pkg/processor/processor.go` (so tests can override — see step 4b):

```go
// sweepInterval controls the auto-complete sweep cadence. Variable (not const)
// so tests can override via SetSweepInterval (export_test.go).
var sweepInterval = 60 * time.Second
```

Then in `Process()`, alongside the existing `ticker := time.NewTicker(5 * time.Second)` (around line 185), add a second ticker:

```go
ticker := time.NewTicker(5 * time.Second)
defer ticker.Stop()

// Slow self-healing sweep: catches specs stuck in `prompted` if the per-prompt
// CheckAndComplete missed (daemon crash mid-completion, race, future regression).
// Cadence kept slower than the queue ticker because the sweep is more expensive.
sweepTicker := time.NewTicker(sweepInterval)
defer sweepTicker.Stop()
```

Add a third case to the `select` block:

```go
case <-sweepTicker.C:
    if err := p.checkPromptedSpecs(ctx); err != nil {
        slog.Warn("periodic checkPromptedSpecs failed", "error", err)
        // do NOT return — daemon continues running
    }
```

Place this case near the other ticker cases for symmetry. Do **not** modify the 5-second queue ticker case — it stays focused on queue responsiveness.

The function `checkPromptedSpecs` already exists at line 774+ and is already used for the startup scan at line 169. Reuse it as-is — no signature changes.

Errors are logged and swallowed (do not return from the daemon). Same defensive shape as the existing 5-second ticker's `processExistingQueued` error handling (lines 207–209).

## 4. Add regression tests

### 4a. Direct-executor order test (`workflow_executor_direct_test.go`)

Add a new Ginkgo `It` block (do NOT modify existing tests):

```go
It("transitions linked spec to verifying after the last prompt completes (regression: order-of-operations bug)", func() {
    // Setup: create a spec + one linked prompt in tempdir
    // The prompt must be in prompts/in-progress/ with status: committing
    // The spec must be in specs/in-progress/ with status: prompted
    //
    // Action: call the direct executor's completeCommit() against the prompt
    //
    // Expected:
    //   - prompt file is now in prompts/completed/
    //   - spec file frontmatter status is "verifying" (NOT still "prompted")
    //
    // This guards against the order-of-operations bug where CheckAndComplete
    // ran before MoveToCompleted and never saw the prompt as completed.
})
```

Use `GinkgoT().TempDir()` for the directories. Construct the executor with the real `spec.NewAutoCompleter` (not a mock) so the test exercises the same path production uses. Mock only the git/release deps.

The test should fail (BeforeFix) and pass (AfterFix) — verify by temporarily reverting the swap in step 1 and observing the failure.

### 4b. Periodic sweep test (`processor_test.go`)

Add a new Ginkgo `It` block (do NOT modify existing tests):

```go
It("self-heals a stuck prompted spec via the periodic sweep", func() {
    // Setup: create a spec in `prompted` state in specs/in-progress/ AND
    // place all linked prompts directly in prompts/completed/ (skip running
    // them — simulating a state where the per-prompt CheckAndComplete failed
    // or a daemon crash left the spec stuck).
    //
    // Action: start the processor and let one sweep tick fire (use a short
    // sweep interval for the test, or directly invoke checkPromptedSpecs).
    //
    // Expected: spec frontmatter transitions to `verifying` after the sweep.
})
```

**Sweep interval parameterization (frozen pattern):** declare an unexported package-level variable in `processor.go`:

```go
var sweepInterval = 60 * time.Second
```

Use `sweepInterval` in the `time.NewTicker(...)` call from step 3. Add an exported test helper in `pkg/processor/export_test.go` that lets tests override it:

```go
// SetSweepInterval is for tests only — overrides the auto-complete sweep ticker interval.
func SetSweepInterval(d time.Duration) (restore func()) {
    prev := sweepInterval
    sweepInterval = d
    return func() { sweepInterval = prev }
}
```

The test calls `restore := processor.SetSweepInterval(20 * time.Millisecond); defer restore()` before starting the daemon.

## 5. CHANGELOG entry

Append to `## Unreleased` in `CHANGELOG.md`:

```
- fix: spec auto-complete now fires AFTER the last prompt is moved to prompts/completed/, not before — specs transition to `verifying` immediately on prompt completion without requiring a daemon restart (regression: workflow_executor_direct.go phase ordering); the daemon also runs a separate 60-second auto-complete sweep, self-healing any future stuck specs within ~1 minute
```

## 6. Run verification

```bash
cd /workspace && make precommit
```

Must exit 0.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Production changes: `pkg/processor/workflow_executor_direct.go` (phase swap) and `pkg/processor/processor.go` (new sweep ticker). Test additions in `workflow_executor_direct_test.go` and `processor_test.go`. Doc update in `CHANGELOG.md`
- No public API changes — `Run` signature, error returns, log messages all stay byte-identical
- The new order is structurally identical to `workflow_helpers.go::moveToCompletedAndCommit` — that function already does it right and is the reference
- `slog.Warn("spec auto-complete failed", ...)` semantics preserved: still best-effort, still non-blocking, still inside the `for _, specID := range pf.Specs()` loop
- Use `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` for any new error construction
- External test packages (`package processor_test`) where applicable
- Coverage ≥80% for the changed package
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:

```bash
cd /workspace

# Confirm MoveToCompleted now precedes the auto-complete loop in the direct executor
awk '/MoveToCompleted/{m=NR} /CheckAndComplete/{c=NR} END{print "MoveToCompleted line:",m,"CheckAndComplete line:",c; exit (m<c?0:1)}' pkg/processor/workflow_executor_direct.go

# Regression test exists
grep -n "transitions linked spec to verifying" pkg/processor/workflow_executor_direct_test.go

# Periodic sweep ticker exists in processor.go
grep -n "sweepTicker\|self-heals a stuck prompted spec" pkg/processor/processor.go pkg/processor/processor_test.go

# CHANGELOG mentions the fix
grep -n "spec auto-complete now fires AFTER" CHANGELOG.md
```
</verification>
