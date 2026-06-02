---
status: failed
spec: [092-daemon-blocked-queue-ux]
container: dark-factory-blocked-queue-exec-442-spec-092-per-spec-queue-advance-guard
dark-factory-version: v0.174.1-dirty
created: "2026-06-02T19:24:09Z"
queued: "2026-06-02T20:16:12Z"
started: "2026-06-02T20:43:36Z"
completed: "2026-06-02T21:24:51Z"
branch: dark-factory/daemon-blocked-queue-ux
lastFailReason: 'validate completion report: completion report status: failed'
---

<summary>

- The `previous prompt is completed` guard is re-scoped from "global lowest number" to "per-spec predecessor". Failed prompts on one spec no longer block unrelated specs.
- Adds `AllPreviousInSpecCompleted(ctx, candidateNumber int, specID string) bool` and `FindMissingInSpecCompleted(ctx, candidateNumber int, specID string) (int, error)` to `prompt.PromptScanner` and `prompt.Manager`. The new methods walk the in-progress directory for prompt files whose `spec` field includes the candidate's spec id and whose number is strictly less than the candidate's number; the highest such number must be in `completed/`.
- The queue-advance loop in `pkg/queuescanner/scanner.go` is refactored: it iterates the queued list, computes the per-spec predecessor for each candidate, and picks the first candidate (in alphabetic-by-filename order) whose per-spec predecessor is `completed`. The `prompt blocked` log line gets `spec=<id>` appended.
- Determinism is enforced by the existing `ListQueued` invariant (already sorted alphabetically by filename at `/workspace/pkg/prompt/prompt.go:1008`) — when multiple specs each have a ready candidate, the alphabetic order from `os.ReadDir` resolves the tie deterministically. No new sort is needed.
- New mock methods on `mocks/queue-scanner-prompt-manager.go`: `AllPreviousInSpecCompletedReturns`, `FindMissingInSpecCompletedReturns`, `FindMissingInSpecCompletedArgsForCall`. Counterfeiter generates these from the new interface methods.
- The widened `reject` command from prompt 1 is exercised by a Ginkgo + goroutine integration test that fires `prompt reject 226` and the scanner's advance loop concurrently on the same prompt id. The test asserts: exactly one final on-disk state, no merge-conflict markers, no double-write. The project lock test uses `lock.NewFileLock` from `/workspace/pkg/lock/filelock.go`; lock acquisition is test-internal because prompt 1 does not add the lock to the reject command.
- `AllPreviousCompleted` and `FindMissingCompleted` STAY unchanged (the pre-execution callers and the prompt-manager tests still use them; the new methods are additive).
- New unit tests in `pkg/prompt/prompt_test.go` for `AllPreviousInSpecCompleted` and `FindMissingInSpecCompleted` (happy path, missing predecessor, no predecessor, cross-spec non-interference, no-spec-field fallback, `specnum.Parse` boundary).
- New ginkgo tests in `pkg/queuescanner/scanner_test.go` for the per-spec ordering: cross-spec advance-allowed, cross-spec deterministic tiebreak, missing-predecessor in same spec still blocks, log line carries spec id.

</summary>

<objective>
Re-scope the queue-advance guard so that the "previous prompt completed" check looks up the highest-numbered prompt strictly less than the candidate within the SAME spec, not globally. Multi-spec repos stop seeing a failed prompt on spec 056 stall spec 058 for 40 minutes.
</objective>

<context>
Read `/workspace/CLAUDE.md` first for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` for the Interface → Constructor → Struct → Method convention.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` for `bborbe/errors` (no `fmt.Errorf`, always pass `ctx`).
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` for Ginkgo v2 / Gomega patterns and Counterfeiter mock reuse.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` for `Create*` factory wiring.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-concurrency-patterns.md` for the Ginkgo + goroutine integration test pattern (the "concurrent reject + advance" test).

Files to read end-to-end before editing:
- `/workspace/specs/in-progress/092-daemon-blocked-queue-ux.md` — full spec, especially `Desired Behavior #3` and AC `per-spec-allows-unrelated`, `per-spec-deterministic-cross-spec`, `concurrent-reject-advance`
- `/workspace/pkg/prompt/prompt.go` lines 707-720 — current `FindPromptStatusInProgress`, `AllPreviousCompleted`, `FindMissingCompleted` (the global-scoped versions that this prompt leaves in place)
- `/workspace/pkg/prompt/prompt.go` lines 1442-1473 — current `allPreviousCompleted` helper (the private function backing the global check; do NOT modify)
- `/workspace/pkg/prompt/prompt.go` lines 1475-1511 — current `findMissingCompleted` helper (do NOT modify)
- `/workspace/pkg/prompt/prompt.go` lines 916-929 — `*Manager.AllPreviousCompleted` / `FindMissingCompleted` / `FindPromptStatusInProgress` (the public surface the scanner uses; do NOT modify)
- `/workspace/pkg/prompt/prompt.go` lines 244-263 — `Frontmatter` struct (the `SpecList` field is what `Specs()` returns — used to look up the candidate's spec id)
- `/workspace/pkg/prompt/prompt.go` lines 569-575 — `(*PromptFile).Specs()` accessor
- `/workspace/pkg/prompt/prompt.go` lines 312-345 — `load` (the per-spec predecessor walk needs to load each candidate's frontmatter to read the `spec` field — mirror this read pattern)
- `/workspace/pkg/prompt/prompt.go` lines 1410-1421 — existing private `readFrontmatter(ctx, path, currentDateTimeGetter)` helper. REUSE this rather than writing a new helper (see §1b note).
- `/workspace/pkg/prompt/prompt.go` lines 961-1014 — `listQueued` (the function that produces the candidates this prompt filters; `ListQueued` returns `[]Prompt` with `Path` and `Status` — the candidates do NOT carry their `SpecList`, so the scanner must `Load` each candidate to read its spec id)
- `/workspace/pkg/prompt/prompt.go` lines 673-720 — `PromptScanner` struct + its public methods (the new methods are added here)
- `/workspace/pkg/specnum/specnum.go` — `Parse(s string) int` returns the leading numeric value (`"058"` → 58, `"092-foo-bar"` → 92, no-prefix → -1). Returns `int`, NOT a canonical string id.
- `/workspace/pkg/queuescanner/scanner.go` lines 1-220 — entire file; this is the core refactor target
- `/workspace/pkg/queuescanner/scanner.go` lines 36-44 — `PromptManager` interface in queuescanner (add new methods here)
- `/workspace/pkg/queuescanner/scanner.go` lines 89-164 — `ScanAndProcess` and `processSingleQueued` (the loop that picks the next candidate)
- `/workspace/pkg/queuescanner/scanner.go` lines 195-219 — `logBlockedOnce` (the `prompt blocked` log line — add `spec=<id>` per spec § Desired Behavior #1)
- `/workspace/pkg/queuescanner/scanner_test.go` lines 22-65 — fixture setup, mock setup (the per-spec tests follow the same pattern)
- `/workspace/pkg/queuescanner/scanner_test.go` lines 224-247 — existing "blocked on prior prompt" tests (mirror for the per-spec test)
- `/workspace/pkg/processor/prompt_manager.go` lines 15-29 — `processor.PromptManager` interface (does NOT need new methods — the per-spec work happens in the scanner's `PromptManager` interface, not the processor's; verify by reading what processor's interface is actually used for vs queuescanner's)
- `/workspace/pkg/lock/filelock.go` — `FileLock` interface used by the concurrent-reject+advance integration test
- `/workspace/pkg/cmd/reject.go` — the widened `reject` from prompt 1; the concurrent test in this prompt invokes it (see §4 precondition).
- `/workspace/pkg/factory/factory.go` line 940 — `queuescanner.NewScanner` call (the constructor signature does NOT change — the per-spec work happens inside the scanner's `processSingleQueued`)

</context>

<requirements>

## 1. Add per-spec predecessor methods to `pkg/prompt/prompt.go`

### 1a. New methods on `PromptScanner`

Add these to `/workspace/pkg/prompt/prompt.go` immediately AFTER the existing `FindMissingCompleted` method (line 720). The methods take the candidate's number and the candidate's spec id.

```go
// AllPreviousInSpecCompleted checks if the predecessor prompt within the same spec
// is in the completed directory. Specifically: walks in-progress/ for files whose
// spec field includes specID and whose number is strictly less than n; the highest
// such number M is the predecessor; returns true iff M is in completed/.
//
// If no predecessor is found (candidate is the first prompt of its spec),
// returns true (no predecessor to check). If specID is empty, returns true
// (caller should fall back to global guard).
func (p PromptScanner) AllPreviousInSpecCompleted(ctx context.Context, n int, specID string) bool {
    return allPreviousInSpecCompleted(ctx, p.completedDir, p.inProgressDir, n, specID, p.currentDateTimeGetter)
}

// FindMissingInSpecCompleted returns the number of the predecessor prompt
// within the same spec that is NOT in completed/, or -1 if no predecessor
// exists for the candidate. Walks in-progress/.
func (p PromptScanner) FindMissingInSpecCompleted(ctx context.Context, n int, specID string) (int, error) {
    return findMissingInSpecCompleted(ctx, p.completedDir, p.inProgressDir, n, specID, p.currentDateTimeGetter)
}
```

The `ctx` is threaded through to the private helpers; the new helpers use it for `errors.Wrap(ctx, err, ...)`. Do NOT add a `cancelledDir` / `rejectedDir` walk — the spec doesn't require it and including them risks scope creep. The `inProgressDir` walk is sufficient because the predecessor of a queued prompt is either still in-progress (for re-runs) or already in completed.

Confirm the `PromptScanner` struct already has a `currentDateTimeGetter libtime.CurrentDateTimeGetter` field (it does — same one passed to `readFrontmatter` elsewhere). If the field is named differently, use the actual name found in the struct definition near line 673.

### 1b. New private helpers

Add these private helpers AFTER `findMissingCompleted` (line 1511) in `/workspace/pkg/prompt/prompt.go`. They take `(ctx, completedDir, scanDir, n, specID, currentDateTimeGetter)` and use the standard `errors.Wrap(ctx, err, ...)` convention.

**Reuse the existing `readFrontmatter(ctx, path, currentDateTimeGetter)` helper** at `/workspace/pkg/prompt/prompt.go:1410-1421`. Do NOT introduce a new `readFrontmatterQuiet` — the existing helper already returns `(*Frontmatter, error)` and is used at lines 822, 978, 1106, 1397. Verify with `grep -n 'func readFrontmatter' /workspace/pkg/prompt/prompt.go` before writing helpers.

```go
// allPreviousInSpecCompleted checks if the predecessor prompt in the same spec
// is in the completed directory. If no predecessor exists, returns true.
// If specID is empty, returns true (caller should fall back to global guard).
func allPreviousInSpecCompleted(
    ctx context.Context,
    completedDir string,
    scanDir string,
    n int,
    specID string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) bool {
    if specID == "" {
        return true
    }
    pred, ok := findPredecessorInSpec(ctx, scanDir, n, specID, currentDateTimeGetter)
    if !ok {
        return true
    }
    return isNumberInCompletedDir(completedDir, pred)
}

// findMissingInSpecCompleted returns the predecessor number in the same spec
// that is NOT in completed/, or -1 if no predecessor exists.
func findMissingInSpecCompleted(
    ctx context.Context,
    completedDir string,
    scanDir string,
    n int,
    specID string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (int, error) {
    if specID == "" {
        return -1, nil
    }
    pred, ok := findPredecessorInSpec(ctx, scanDir, n, specID, currentDateTimeGetter)
    if !ok {
        return -1, nil
    }
    if isNumberInCompletedDir(completedDir, pred) {
        return -1, nil
    }
    return pred, nil
}

// findPredecessorInSpec walks scanDir for prompt files whose spec field includes specID
// and whose number is strictly less than n. Returns the highest such number and true,
// or (-1, false) if no such prompt exists.
func findPredecessorInSpec(
    ctx context.Context,
    scanDir string,
    n int,
    specID string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (int, bool) {
    entries, err := os.ReadDir(scanDir)
    if err != nil {
        return -1, false
    }
    highest := -1
    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
            continue
        }
        num := extractNumberFromFilename(entry.Name())
        if num < 0 || num >= n {
            continue
        }
        path := filepath.Join(scanDir, entry.Name())
        fm, err := readFrontmatter(ctx, path, currentDateTimeGetter)
        if err != nil {
            continue
        }
        if !specListContains(fm.SpecList, specID) {
            continue
        }
        if num > highest {
            highest = num
        }
    }
    return highest, highest >= 0
}

// isNumberInCompletedDir returns true if a file with the given number exists in completedDir.
func isNumberInCompletedDir(completedDir string, num int) bool {
    entries, err := os.ReadDir(completedDir)
    if err != nil {
        return false
    }
    prefix := fmt.Sprintf("%03d-", num)
    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
            continue
        }
        if strings.HasPrefix(entry.Name(), prefix) {
            return true
        }
    }
    return false
}

// specListContains returns true if specID matches any entry in the spec list.
// Comparison uses specnum.Parse to normalize numeric prefixes ("058" == "058-foo-bar" == 58).
// When specnum.Parse returns -1 for both sides (no numeric prefix), falls back to string equality.
func specListContains(specs SpecList, specID string) bool {
    target := specnum.Parse(specID)
    for _, s := range specs {
        if target >= 0 {
            if specnum.Parse(s) == target {
                return true
            }
        } else {
            if s == specID {
                return true
            }
        }
    }
    return false
}
```

Notes:
- The existing `allPreviousCompleted` and `findMissingCompleted` STAY (the prompt-manager tests at `prompt_test.go:244-403` use them; the per-spec additions are net-new).
- The actual frontmatter field name for the spec list is `SpecList` (confirm at `prompt.go:244-263`). Use that name verbatim.
- The `imports` block at lines 7-30 already provides `errors`, `specnum`, `strings`, `filepath`, `fmt`, `libtime` — no new imports needed.

### 1c. New methods on `*Manager`

Add to `/workspace/pkg/prompt/prompt.go` immediately AFTER the existing `FindPromptStatusInProgress` (line 929):

```go
// AllPreviousInSpecCompleted checks if the predecessor prompt in the same spec is completed.
func (pm *Manager) AllPreviousInSpecCompleted(ctx context.Context, n int, specID string) bool {
    return pm.promptScanner.AllPreviousInSpecCompleted(ctx, n, specID)
}

// FindMissingInSpecCompleted returns the predecessor number in the same spec that is NOT completed.
func (pm *Manager) FindMissingInSpecCompleted(ctx context.Context, n int, specID string) (int, error) {
    return pm.promptScanner.FindMissingInSpecCompleted(ctx, n, specID)
}
```

## 2. Add the new methods to the queuescanner's `PromptManager` interface

Edit `/workspace/pkg/queuescanner/scanner.go` lines 36-44. The interface becomes:

```go
type PromptManager interface {
    ListQueued(ctx context.Context) ([]prompt.Prompt, error)
    Load(ctx context.Context, path string) (*prompt.PromptFile, error)
    AllPreviousCompleted(ctx context.Context, n int) bool
    FindMissingCompleted(ctx context.Context, n int) []int
    FindPromptStatusInProgress(ctx context.Context, number int) string
    SetStatus(ctx context.Context, path string, status string) error
    // Per-spec predecessor lookup (spec 092)
    AllPreviousInSpecCompleted(ctx context.Context, n int, specID string) bool
    FindMissingInSpecCompleted(ctx context.Context, n int, specID string) (int, error)
}
```

`AllPreviousCompleted` and `FindMissingCompleted` STAY (the scanner still uses them as a fallback when the candidate has no `spec` field — see step 3b).

## 3. Refactor `processSingleQueued` in `pkg/queuescanner/scanner.go`

The current `processSingleQueued` (lines 120-164) picks `queued[0]` and runs the global predecessor check. The new logic:

### 3a. Per-candidate filter loop

Replace the `pr := queued[0]` (line 133) and the `AllPreviousCompleted` check (line 143) with a per-candidate filter. Pseudocode:

```go
for _, candidate := range queued {
    if s.shouldSkipPrompt(ctx, candidate) {
        continue
    }
    specID, err := s.readSpecID(ctx, candidate)
    if err != nil {
        // Malformed prompt frontmatter — treat as blocked, surface via logBlockedOnce
        s.logBlockedOnce(ctx, candidate, "", "malformed frontmatter")
        continue
    }
    if specID == "" {
        // No spec field — fall back to global guard
        if s.promptManager.AllPreviousCompleted(ctx, candidate.Number()) {
            pr = candidate
            break
        }
        continue
    }
    if s.promptManager.AllPreviousInSpecCompleted(ctx, candidate.Number(), specID) {
        pr = candidate
        break
    }
    // Blocked: log once with spec id, then return DONE
    s.logBlockedOnce(ctx, candidate, specID, "previous prompt not completed")
    return true, nil
}
if pr.Path == "" {
    return true, nil // no candidates
}
```

The `readSpecID` helper:
- Loads the prompt via `s.promptManager.Load(ctx, candidate.Path)`.
- If `len(pf.Frontmatter.SpecList) == 0` → returns `("", nil)` (legacy un-specced prompt).
- If `len(pf.Frontmatter.SpecList) == 1` → returns `(pf.Frontmatter.SpecList[0], nil)`.
- If `len(pf.Frontmatter.SpecList) > 1` → returns `("", errors.New(ctx, "multi-spec prompt: tie-breaking unspecified"))`. The spec does not specify how to choose among multiple spec ids; **fail closed and treat as malformed**. The scanner surfaces this via `logBlockedOnce` (same dedup mechanism as the missing-predecessor case). The spec author can refine the rule if multi-spec prompts become real.

### 3b. Spec field absent

If the candidate has no `spec` field (e.g. a legacy prompt written before spec-id tracking was added), fall back to the existing `AllPreviousCompleted` (global guard). This preserves the existing behavior for un-specced prompts. Add a comment explaining the fallback: "Prompts without a spec field use the legacy global predecessor guard."

### 3c. Determinism: alphabetic order is the tiebreak

The current `ListQueued` already returns entries sorted alphabetically by filename (`/workspace/pkg/prompt/prompt.go:1008`). The alphabetical order over a fixed-width numeric prefix (`NNN-slug.md`) corresponds to numeric order for prompts with the same prefix length, and the spec's example (spec A's prompt 221 vs spec B's prompt 223) sorts correctly. Document the invariant with a one-line comment near the filter loop.

### 3d. Replace `pr := queued[0]`

The new logic iterates all candidates and picks the first one whose per-spec guard passes. The variable `pr` (line 133) becomes the chosen candidate. The downstream calls (`autoSetQueuedStatus`, `shouldSkipPrompt`, `ProcessPrompt`, `failureHandler.Handle`) operate on `pr` as today.

### 3e. The "blocked" path

If no candidate passes the per-spec guard, return `(true, nil)` (same as today's behavior). Widen `logBlockedOnce` to accept `(ctx, candidate, specID, reason)`; the log line carries `spec=<id>` and the appropriate reason:

```go
slog.Info(
    "prompt blocked",
    "file", filepath.Base(candidate.Path),
    "reason", reason,
    "spec", specID,
    "missing", missingStr,
)
```

The `lastBlockedMsg` deduplication key is the full string including spec id and reason. The two reasons emitted are `"previous prompt not completed"` (per-spec predecessor missing) and `"malformed frontmatter"` (multi-spec or unreadable prompt). The `Blocked:` surfacing prompt 3 owns the user-facing surface; this prompt only widens the log line.

The `missingStr` is computed via `FindMissingInSpecCompleted` for the spec'd path and via `FindMissingCompleted` for the legacy global path; for malformed prompts, pass `""`.

## 4. Concurrent reject + advance lock semantics test (AC: `concurrent-reject-advance`)

**Precondition from prompt 1:** prompt 1 adds `cmd.NewRejectCommand(...)` (see `/workspace/pkg/cmd/reject.go`). Read its exported constructor signature before writing this test. If prompt 1's constructor signature differs from what this test assumes, mirror prompt 1's actual signature here verbatim — do not invent parameters. The test below assumes `cmd.NewRejectCommand` returns a `*cobra.Command` that runs reject for a numeric arg; adapt to the real shape.

Add a new ginkgo test in `pkg/queuescanner/scanner_test.go` that runs the widened reject command and the scanner's `ScanAndProcess` concurrently on the same prompt id. The test verifies:

- Exactly one final on-disk state (file is in `rejected/`, not in `in-progress/` or duplicated).
- No merge-conflict markers in the file.
- Scanner log shows a wait-for-lock then post-lock re-read.
- `os.ReadDir("prompts/rejected/")` returns exactly one entry matching the prompt id.

The test uses `lock.NewFileLock` from `/workspace/pkg/lock/filelock.go` to enforce the lock manually. Prompt 1 does NOT add the project lock to the reject command (per its constraints); the test wraps both operations in `lock.NewFileLock(path).Acquire(ctx)` / `Release(ctx)` itself. Skip the test if running in an environment where `flock` is unavailable (use `runtime.GOOS == "linux"` or `"darwin"`).

Concrete shape:

```go
var _ = Describe("ConcurrentRejectAndAdvance", func() {
    It("serializes reject and advance via project lock — no double-write", func() {
        // Set up: a failed prompt in in-progress/, scanner's AllPreviousInSpecCompleted returns true
        // Spawn two goroutines: one runs reject, one runs scanner.ScanAndProcess
        // Wait for both, assert the file is in rejected/ exactly once
        // Use lock.NewFileLock to serialize — acquire in reject first, then scanner waits
    })
})
```

Lock acquisition is test-internal only.

## 5. Counterfeiter mock regeneration

The `queuescanner.PromptManager` interface gained two new methods. Run `cd /workspace && make generate-mocks` (or the equivalent target per the Makefile — verify with `grep -n 'generate-mocks\|counterfeiter:' /workspace/Makefile`). The regenerated `mocks/queue-scanner-prompt-manager.go` will gain `AllPreviousInSpecCompletedReturns` and `FindMissingInSpecCompletedReturns` methods. The regenerated `mocks/processor-prompt-manager.go` is NOT touched (the processor's interface doesn't get the new methods).

Verify the diff before declaring done:

```
cd /workspace && git diff mocks/ | head -100
```

Expected: only `mocks/queue-scanner-prompt-manager.go` has new methods. If the processor mock is also touched, revert that change (the counterfeiter regen is wider than needed).

## 6. Add ginkgo tests in `pkg/queuescanner/scanner_test.go`

### 6a. Per-spec ordering allows unrelated spec to advance (AC: `per-spec-allows-unrelated`)

```go
It("selects candidate 227 of spec 058 even when prompt 226 of spec 056 is failed", func() {
    // Fixtures: 226 of spec 056 in in-progress/ with status failed; 227 of spec 058 in in-progress/ with status queued
    // mgr.AllPreviousInSpecCompleted for (227, "058") returns true (225 of spec 058 is in completed/)
    // mgr.AllPreviousInSpecCompleted for (226, "056") returns true (no predecessor in spec 056)
    // mgr.Load returns the right PromptFile for the spec id lookup
    // pp.ProcessPrompt receives the 227 candidate
    // Assert: pp.ProcessPromptCallCount() == 1, the call's pr.Path ends with "227"
})
```

### 6b. Per-spec deterministic cross-spec ordering (AC: `per-spec-deterministic-cross-spec`)

```go
It("picks the lower global prompt number when both specs have a ready candidate", func() {
    // Fixtures: spec A's 221 ready, spec B's 223 ready
    // ListQueued returns [221, 223] (alphabetically sorted)
    // Both pass the per-spec guard
    // pp.ProcessPrompt receives 221 (the lower global number)
    // Assert: pp.ProcessPromptCallCount() == 1, the call's pr.Path ends with "221"
})
```

### 6c. Missing predecessor in same spec still blocks

```go
It("blocks candidate whose same-spec predecessor is not completed", func() {
    // Fixtures: 220 of spec 056 in completed/; 222 of spec 056 in in-progress/ (no 221)
    // mgr.AllPreviousInSpecCompleted for (222, "056") returns false (221 is missing)
    // Assert: pp.ProcessPromptCallCount() == 0
    // Assert: scan returned 0 completed
})
```

### 6d. Log line carries spec id

```go
It("logs prompt blocked with spec id", func() {
    // Same fixture as 6c
    // Capture slog output via slog.SetDefault with a buffer-backed handler
    // Assert the "prompt blocked" line contains "spec=056"
})
```

Slog capture: the test can use `slog.SetDefault(slog.New(slog.NewTextHandler(buf, ...)))` and restore in `AfterEach`. Verify by searching for `slog.SetDefault` in `pkg/queuescanner/*_test.go` and `pkg/doctor/*_test.go` for an existing pattern.

### 6e. Multi-spec prompt is flagged as malformed

```go
It("treats a multi-spec prompt as malformed and surfaces via Blocked", func() {
    // Fixture: candidate whose PromptFile.Frontmatter.SpecList has 2 entries
    // mgr.Load returns a PromptFile with SpecList=["056","058"]
    // Assert: pp.ProcessPromptCallCount() == 0 (not advanced)
    // Assert: slog output contains reason="malformed frontmatter"
})
```

## 7. Add ginkgo tests in `pkg/prompt/prompt_test.go`

Add a new sibling `Describe("AllPreviousInSpecCompleted", ...)` block alongside the existing `Describe("AllPreviousCompleted", ...)` at line 244.

### 7a. Happy path — predecessor in completed/

```go
It("returns true when predecessor in same spec is in completed/", func() {
    // Fixtures:
    //   in-progress/225-spec-058-foo.md with spec: ["058"]
    //   completed/224-spec-058-bar.md with spec: ["058"]
    // Call AllPreviousInSpecCompleted(ctx, 225, "058") — expect true
})
```

### 7b. Missing predecessor in same spec

```go
It("returns false when predecessor in same spec is missing", func() {
    // Fixtures:
    //   in-progress/225-spec-058-foo.md with spec: ["058"]
    //   completed/223-spec-058-bar.md (224 is missing)
    // Call AllPreviousInSpecCompleted(ctx, 225, "058") — expect false
    // Call FindMissingInSpecCompleted(ctx, 225, "058") — expect (224, nil)
})
```

### 7c. No predecessor in same spec

```go
It("returns true when no predecessor exists in same spec", func() {
    // Fixtures: only in-progress/225-spec-058-foo.md; nothing before it
    // Call AllPreviousInSpecCompleted(ctx, 225, "058") — expect true (no predecessor to check)
    // Call FindMissingInSpecCompleted(ctx, 225, "058") — expect (-1, nil)
})
```

### 7d. Cross-spec non-interference

```go
It("ignores a failed prompt in a different spec", func() {
    // Fixtures:
    //   in-progress/225-spec-058-foo.md with spec: ["058"] — the candidate
    //   in-progress/224-spec-056-blocker.md with status: failed, spec: ["056"] — different spec
    //   completed/223-spec-058-predecessor.md with spec: ["058"] — same-spec predecessor, completed
    // Call AllPreviousInSpecCompleted(ctx, 225, "058") — expect true
    //   Reason: predecessor of 225 in spec 058 is 224; 224 of spec 058 doesn't exist
    //   223 of spec 058 IS in completed, and 223 < 225
    //   The function returns the HIGHEST number < 225 in spec 058; 223 is the highest (no 224 of spec 058)
    //   223 is in completed → true
})
```

The OLD `AllPreviousCompleted(ctx, 225)` (global) would return `false` because 224 (the failed one in spec 056) is missing from completed. The new per-spec guard does not have this problem.

### 7e. No spec field on candidate

```go
It("returns true for a candidate with no spec field (empty specID)", func() {
    // Fixture: in-progress/225-no-spec.md with NO spec field
    // Call AllPreviousInSpecCompleted(ctx, 225, "") — expect true
    //   Rationale: empty specID returns true. The scanner's fallback to the global guard
    //   happens at the scanner layer, not in the helper.
})
```

### 7f. Boundary test for `specnum.Parse` normalization

```go
It("treats bare and full spec ids as equivalent (specnum.Parse normalization)", func() {
    // Fixtures: in-progress/225-foo.md with spec: ["058"]
    //           completed/224-foo.md with spec: ["058-some-slug"]
    // Call AllPreviousInSpecCompleted(ctx, 225, "058") — expect true
    //   The completed predecessor declares spec "058-some-slug" but specnum.Parse("058-some-slug")
    //   == specnum.Parse("058") == 58, so specListContains matches.
    // Sub-assertion: also verify the table case Parse("058") == 58 and Parse("092-foo-bar") == 92
    //   (sanity-check the normalization the scanner relies on).
})
```

Document the choice in a comment near `AllPreviousInSpecCompleted`: "If specID is empty, no per-spec predecessor lookup is performed; the function returns true (caller should fall back to global guard at the scanner layer)."

## 8. Run `cd /workspace && make test`

All tests must pass. The new tests must compile (the new method signatures must match the mocks) and pass. The existing tests must still pass (the old `AllPreviousCompleted` / `FindMissingCompleted` are unchanged; the per-spec additions are net-new).

If the counterfeiter regen in step 5 changed more than `mocks/queue-scanner-prompt-manager.go`, the diff will surface in `make test`'s lint step. Run `git diff mocks/` and revert any unwanted changes.

</requirements>

<constraints>

- Do NOT modify the existing `AllPreviousCompleted` / `FindMissingCompleted` / `findMissingCompleted` / `allPreviousCompleted` private helpers in `pkg/prompt/prompt.go`. The old methods stay so the prompt-manager tests and the global fallback path both work.
- Do NOT change the public signature of `queuescanner.NewScanner`, `queuescanner.Scanner`, `queuescanner.PromptManager` (other than ADDING new methods — adding is fine; removing is not).
- Do NOT change the sort order of `ListQueued`. The current alphabetical-by-filename sort is the deterministic tiebreak the spec requires. Do NOT introduce a new sort.
- Do NOT modify `processor.PromptManager` at `/workspace/pkg/processor/prompt_manager.go`. The per-spec work is in the scanner's interface, not the processor's.
- Do NOT add a new `SpecList` field or any new prompt frontmatter. The `spec` field is read-only here. Per spec § Non-goals: "no new field is added to prompt frontmatter (other than the `originalStatus` and `rejectedReason` written by the widened reject path)."
- Do NOT add a per-spec opt-out flag. The per-spec guard is the invariant. Per spec § Non-goals: "Do NOT add an opt-out flag for any of the three behaviors."
- Do NOT renumber prompts per-spec. The spec's "Suggested Decomposition" and Non-goals both state this.
- Do NOT introduce a new `readFrontmatterQuiet` helper — reuse the existing `readFrontmatter(ctx, path, currentDateTimeGetter)` at `prompt.go:1410`.
- Do NOT pick a tie-breaking rule for multi-spec prompts. Fail closed (surface as malformed) and let the spec author refine.
- Do NOT commit — dark-factory handles git.
- File mode `0600` for new test-fixture frontmatter writes; `0750` for directories the test creates. Project convention.
- Branch is `dark-factory/daemon-blocked-queue-ux` (the spec's branch per frontmatter). Do not switch branches.
- Test files in `package queuescanner_test` and `package prompt_test` (external test packages) per `go-testing-guide.md`.
- Counterfeiter mocks must be regenerated via `make generate-mocks` after the interface change. Do NOT hand-write the mock methods.

</constraints>

<verification>

Run from the repo root:

```
cd /workspace && make test
```

All tests must pass. New test cases added in this prompt must compile and pass.

Spot checks (exact deltas from this prompt):

```
cd /workspace
grep -n 'AllPreviousInSpecCompleted' pkg/prompt/prompt.go      # exactly 3 lines: PromptScanner method, Manager method, private allPreviousInSpecCompleted
grep -n 'FindMissingInSpecCompleted' pkg/prompt/prompt.go      # exactly 3 lines: PromptScanner method, Manager method, private findMissingInSpecCompleted
grep -n 'findPredecessorInSpec\|isNumberInCompletedDir\|specListContains' pkg/prompt/prompt.go  # exactly 3 new helpers (1 line each at func definition)
grep -n 'specID' pkg/queuescanner/scanner.go                   # >= 5 lines: readSpecID, log line, dedup key, filter loop
grep -nE 'prompt blocked' pkg/queuescanner/scanner.go          # 1 line
grep -nE '"spec",' pkg/queuescanner/scanner.go                 # 1 line (log field key)
grep -n 'AllPreviousInSpecCompletedReturns\|FindMissingInSpecCompletedReturns' mocks/queue-scanner-prompt-manager.go   # >= 2 lines
# Test deltas added by THIS prompt:
#   pkg/queuescanner/scanner_test.go: +5 new It blocks (6a, 6b, 6c, 6d, 6e)
#   pkg/prompt/prompt_test.go:        +6 new It blocks (7a, 7b, 7c, 7d, 7e, 7f)
# (Section 4 adds the ConcurrentRejectAndAdvance Describe with 1 It; included in the +5 scanner count if you place it in scanner_test.go.)
```

The unit tests in steps 6a-7f are the primary verification of the per-spec behavior. There is no manual smoke test — running the daemon binary inside the YOLO container is out of scope. Verification is `make precommit` + `make test` + the Ginkgo spec exit codes.

If any spot check fails, fix the gap before declaring done.

</verification>
