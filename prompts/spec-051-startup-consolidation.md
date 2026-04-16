---
status: created
spec: [051-runner-startup-consolidation]
created: "2026-04-16T19:45:00Z"
branch: dark-factory/runner-startup-consolidation
---

<summary>
- A new `StartupDeps` struct in `pkg/runner/lifecycle.go` carries every shared dependency the startup sequence needs — no more scattered parameter lists
- A new `startupSequence(ctx, StartupDeps) error` function in `lifecycle.go` encapsulates the six steps that are duplicated today across `runner.go` and `oneshot.go`
- `runner.go` calls `startupSequence` exactly once before entering its main loop; all six shared step calls are removed from `Run()`
- `oneshot.go` calls `startupSequence` exactly once before beginning its single-pass execution; all six shared step calls are removed from `Run()`
- Three daemon-only steps (`resumeOrResetGenerating`, `processor.ResumeExecuting`, and the `.git/index.lock` guard) remain in `runner.go` with a comment explaining why they are not in the shared sequence
- A unit test in `lifecycle_test.go` verifies that both runner types produce the same startup-call ordering by recording steps on a fake `StartupDeps`
- Adding a new shared startup step requires editing `lifecycle.go` and `StartupDeps` only; daemon-specific logic stays in `runner.go` with no changes to `lifecycle.go`
- All existing runner tests pass; `make precommit` exits 0
</summary>

<objective>
Extract the six startup steps duplicated in `runner.go` and `oneshot.go` into a single `startupSequence(ctx, StartupDeps) error` function in `pkg/runner/lifecycle.go`, so that any new shared startup concern is a one-file change. Three daemon-only steps are discovered to be interleaved in the shared sequence in `runner.go`; per the spec's failure-mode guidance, they remain in `runner.go` with a comment.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read before editing:
- `pkg/runner/runner.go` — `Runner` interface, `runner` struct, `NewRunner`, `Run()` method, and the helper method wrappers at the bottom
- `pkg/runner/oneshot.go` — `OneShotRunner` interface, `oneShotRunner` struct, `NewOneShotRunner`, `Run()` method, and helper method wrappers at the bottom
- `pkg/runner/lifecycle.go` — already contains `reindexAll`, `normalizeFilenames`, `migrateQueueDir`, `createDirectories`, `resumeOrResetExecuting`, `resumeOrResetGenerating`, and their sub-helpers
- `pkg/runner/export_test.go` — exported helpers for tests
- `pkg/runner/runner_test.go` — existing runner tests (understand the `newTestRunner` helper pattern)
- `pkg/runner/oneshot_test.go` — existing oneshot tests (understand the `newTestOneShotRunner` helper pattern)
- `pkg/runner/runner_suite_test.go` — Ginkgo suite setup
- `mocks/` directory — existing counterfeiter mocks available (Manager, Locker, ContainerChecker, Processor, FileMover, SpecSlugMigrator, etc.)
- `pkg/processor/processor.go` — `Processor` interface (has `ResumeExecuting(ctx) error`)
- `pkg/slugmigrator/` — `Migrator` interface

**Key observation about the two Run() methods:**

`runner.go` Run() startup steps (after lock + startupLogger):
1. `.git/index.lock` guard — DAEMON-ONLY
2. signal.NotifyContext setup — DAEMON-ONLY (not a step, just context setup)
3. `r.migrateQueueDir(ctx)` — SHARED
4. `r.createDirectories(ctx)` — SHARED
5. `r.resumeOrResetExecuting(ctx)` — SHARED
6. `r.resumeOrResetGenerating(ctx)` — DAEMON-ONLY (interleaved between shared steps 5 and 7)
7. `r.processor.ResumeExecuting(ctx)` — DAEMON-ONLY (interleaved between shared steps 5 and 7)
8. `r.reindexAll(ctx)` — SHARED
9. `r.normalizeFilenames(ctx)` — SHARED
10. `r.migrateSpecSlugs(ctx)` — SHARED

`oneshot.go` Run() startup steps (after lock + startupLogger):
1. `r.migrateQueueDir(ctx)` — SHARED
2. `r.createDirectories(ctx)` — SHARED
3. `r.resumeOrResetExecuting(ctx)` — SHARED
4. `r.reindexAll(ctx)` — SHARED
5. `r.normalizeFilenames(ctx)` — SHARED
6. `r.slugMigrator.MigrateDirs(...)` — SHARED (inline, not via wrapper method)

The six SHARED steps (same in both, same order) are: migrateQueueDir, createDirectories, resumeOrResetExecuting, reindexAll, normalizeFilenames, migrateSpecSlugs. The three daemon-only steps are interleaved in runner.go between `resumeOrResetExecuting` and `reindexAll` — forcing them into a single linear `startupSequence` would require splitting the sequence or adding daemon-specific conditionals. Per spec failure-mode guidance: keep them in `runner.go` with a comment.
</context>

<requirements>

## 1. Add `StartupDeps` struct to `pkg/runner/lifecycle.go`

At the top of `pkg/runner/lifecycle.go`, after the import block, add the `StartupDeps` struct. It carries every dependency the six shared startup steps need. Add it just before the `reindexAll` function:

```go
// StartupDeps carries every dependency the shared startup sequence needs.
// Both Runner and OneShotRunner populate this struct from their own fields
// before calling startupSequence.
type StartupDeps struct {
    InboxDir              string
    InProgressDir         string
    CompletedDir          string
    LogDir                string
    SpecsInboxDir         string
    SpecsInProgressDir    string
    SpecsCompletedDir     string
    SpecsLogDir           string
    PromptManager         prompt.Manager
    ContainerChecker      executor.ContainerChecker
    Notifier              notifier.Notifier  // may be nil (oneshot passes nil)
    ProjectName           string             // may be empty (oneshot passes "")
    SlugMigrator          slugmigrator.Migrator
    Mover                 prompt.FileMover
    CurrentDateTimeGetter libtime.CurrentDateTimeGetter
}
```

Add the required import for `slugmigrator`:
```go
"github.com/bborbe/dark-factory/pkg/slugmigrator"
```

`lifecycle.go` already imports `prompt`, `executor`, `notifier`, and `libtime`, so those don't need to be added.

## 2. Add `startupSequence` function to `pkg/runner/lifecycle.go`

Immediately after the `StartupDeps` struct, add the `startupSequence` function:

```go
// startupSequence runs the six startup steps shared by both Runner and OneShotRunner.
// Steps:
//  1. migrateQueueDir — migrate prompts/queue/ → prompts/in-progress/ if needed
//  2. createDirectories — ensure all eight lifecycle dirs exist
//  3. resumeOrResetExecuting — selectively resume or reset stuck executing prompts
//  4. reindexAll — resolve cross-directory number conflicts
//  5. normalizeFilenames — normalize in-progress filenames
//  6. migrateSpecSlugs — replace bare spec number refs with full slugs
//
// Daemon-only steps (resumeOrResetGenerating, processor.ResumeExecuting) are NOT
// included here because they are interleaved between steps 3 and 4 only in the
// daemon runner. Forcing them here would split this function or add mode-specific
// conditionals. They remain in runner.go with a comment.
func startupSequence(ctx context.Context, deps StartupDeps) error {
    if err := migrateQueueDir(ctx, deps.InProgressDir); err != nil {
        return errors.Wrap(ctx, err, "migrate queue dir")
    }

    dirs := []string{
        deps.InboxDir,
        deps.InProgressDir,
        deps.CompletedDir,
        deps.LogDir,
        deps.SpecsInboxDir,
        deps.SpecsInProgressDir,
        deps.SpecsCompletedDir,
        deps.SpecsLogDir,
    }
    if err := createDirectories(ctx, dirs); err != nil {
        return errors.Wrap(ctx, err, "create directories")
    }

    if err := resumeOrResetExecuting(ctx, deps.InProgressDir, deps.PromptManager, deps.ContainerChecker, deps.Notifier, deps.ProjectName); err != nil {
        return errors.Wrap(ctx, err, "resume or reset executing prompts")
    }

    specDirs := []string{deps.SpecsInboxDir, deps.SpecsInProgressDir, deps.SpecsCompletedDir, deps.SpecsLogDir}
    promptDirs := []string{deps.InboxDir, deps.InProgressDir, deps.CompletedDir, deps.LogDir}
    if err := reindexAll(ctx, specDirs, promptDirs, deps.Mover, deps.CurrentDateTimeGetter); err != nil {
        return errors.Wrap(ctx, err, "reindex files")
    }

    if err := normalizeFilenames(ctx, deps.PromptManager, deps.InProgressDir); err != nil {
        return errors.Wrap(ctx, err, "normalize filenames")
    }

    if err := deps.SlugMigrator.MigrateDirs(ctx, []string{
        deps.InboxDir, deps.InProgressDir, deps.CompletedDir, deps.LogDir,
    }); err != nil {
        return errors.Wrap(ctx, err, "migrate spec slugs")
    }

    return nil
}
```

## 3. Update `pkg/runner/runner.go`

### 3a. Add `startupDeps()` helper method on `runner`

Add this method near the other helper methods at the bottom of `runner.go`:

```go
// startupDeps builds a StartupDeps from this runner's fields.
func (r *runner) startupDeps() StartupDeps {
    return StartupDeps{
        InboxDir:              r.inboxDir,
        InProgressDir:         r.inProgressDir,
        CompletedDir:          r.completedDir,
        LogDir:                r.logDir,
        SpecsInboxDir:         r.specsInboxDir,
        SpecsInProgressDir:    r.specsInProgressDir,
        SpecsCompletedDir:     r.specsCompletedDir,
        SpecsLogDir:           r.specsLogDir,
        PromptManager:         r.promptManager,
        ContainerChecker:      r.containerChecker,
        Notifier:              r.notifier,
        ProjectName:           r.projectName,
        SlugMigrator:          r.slugMigrator,
        Mover:                 r.mover,
        CurrentDateTimeGetter: r.currentDateTimeGetter,
    }
}
```

### 3b. Replace the six shared step calls in `runner.Run()` with `startupSequence`

In `runner.Run()`, the current code after the signal setup is:

```go
// Migrate old prompts/queue/ → prompts/in-progress/ if needed
if err := r.migrateQueueDir(ctx); err != nil {
    return errors.Wrap(ctx, err, "migrate queue dir")
}

// Create directories if they don't exist
if err := r.createDirectories(ctx); err != nil {
    return errors.Wrap(ctx, err, "create directories")
}

slog.Info("watching for queued prompts", "dir", r.inProgressDir)

// Selectively resume or reset executing prompts based on container liveness
if err := r.resumeOrResetExecuting(ctx); err != nil {
    return errors.Wrap(ctx, err, "resume or reset executing prompts")
}

// Reset any specs left in generating state if their container is gone
if err := r.resumeOrResetGenerating(ctx); err != nil {
    return errors.Wrap(ctx, err, "resume or reset generating specs")
}

// Resume any prompts still in executing state (container was still running on restart)
if err := r.processor.ResumeExecuting(ctx); err != nil {
    return errors.Wrap(ctx, err, "resume executing prompts")
}

// Reindex all spec and prompt dirs to resolve cross-directory number conflicts
if err := r.reindexAll(ctx); err != nil {
    return errors.Wrap(ctx, err, "reindex files")
}

// Normalize filenames before processing
if err := r.normalizeFilenames(ctx); err != nil {
    return errors.Wrap(ctx, err, "normalize filenames")
}

// Migrate bare spec number refs to full slugs in all prompt lifecycle dirs
if err := r.migrateSpecSlugs(ctx); err != nil {
    return errors.Wrap(ctx, err, "migrate spec slugs")
}
```

Replace it with the following block. **Preserve the `slog.Info("watching for queued prompts", ...)` log line — move it to just before the call to `startupSequence`:**

```go
slog.Info("watching for queued prompts", "dir", r.inProgressDir)

// Run the six shared startup steps (migrateQueueDir, createDirectories,
// resumeOrResetExecuting, reindexAll, normalizeFilenames, migrateSpecSlugs).
if err := startupSequence(ctx, r.startupDeps()); err != nil {
    return errors.Wrap(ctx, err, "startup sequence")
}

// Daemon-only: reset specs left in generating state if their container is gone.
// Not in startupSequence because this step has no counterpart in the one-shot runner.
if err := r.resumeOrResetGenerating(ctx); err != nil {
    return errors.Wrap(ctx, err, "resume or reset generating specs")
}

// Daemon-only: reattach to any prompts still in executing state from a prior run.
// Not in startupSequence for the same reason.
if err := r.processor.ResumeExecuting(ctx); err != nil {
    return errors.Wrap(ctx, err, "resume executing prompts")
}
```

**Important:** The `slog.Info("watching for queued prompts", ...)` line was originally between `createDirectories` and `resumeOrResetExecuting`. After this change it moves to just before `startupSequence`. This is acceptable — the log line describes the intent of the loop, not a specific step.

### 3c. Remove the now-unused helper method wrappers from `runner`

After the refactor, the following receiver methods on `runner` are no longer called from `Run()` (they were only there to delegate to lifecycle package-level functions):
- `(*runner).migrateQueueDir`
- `(*runner).createDirectories`
- `(*runner).normalizeFilenames`
- `(*runner).migrateSpecSlugs`

Remove all four. **Keep** `(*runner).reindexAll`, `(*runner).resumeOrResetExecuting`, `(*runner).resumeOrResetGenerating`, and `(*runner).createDirectories` — wait, recheck: `reindexAll` and `resumeOrResetExecuting` and `resumeOrResetGenerating` are still called (two daemon-only ones remain as direct method calls). So remove only the four that are now dead code: `migrateQueueDir`, `createDirectories`, `normalizeFilenames`, `migrateSpecSlugs`.

**Verification step before removing:** run `grep -n "r\.migrateQueueDir\|r\.createDirectories\|r\.normalizeFilenames\|r\.migrateSpecSlugs" pkg/runner/runner.go` — each should return zero hits after removing the call sites in step 3b. Then remove the method definitions.

Also remove `(*runner).reindexAll` if it is now unused (it was called from `Run()` before but `startupSequence` now calls the package-level function directly). **Check** with `grep -n "r\.reindexAll" pkg/runner/runner.go` — if zero hits, remove the method. Similarly for `(*runner).resumeOrResetExecuting` — it is now called only from `resumeOrResetGenerating`? No — `resumeOrResetExecuting` is daemon-specific remaining. Wait: after step 3b, `resumeOrResetExecuting` is inside `startupSequence`, so the receiver method `(*runner).resumeOrResetExecuting` is dead. **Check** with grep and remove if unused.

Summary: after 3b, these receiver methods are dead and must be removed:
- `(*runner).migrateQueueDir`
- `(*runner).createDirectories`
- `(*runner).normalizeFilenames`
- `(*runner).migrateSpecSlugs`
- `(*runner).reindexAll`
- `(*runner).resumeOrResetExecuting`

These receiver methods are still needed (called from Run or healthCheckLoop):
- `(*runner).resumeOrResetGenerating` — called directly in Run() as daemon-only step
- `(*runner).healthCheckLoop` — called from Run() via `runners` slice
- `(*runner).createDirectories` — wait, no, this was moved into startupSequence

Run `grep -n "func (r \*runner)" pkg/runner/runner.go` before and after to confirm exactly which methods to remove.

## 4. Update `pkg/runner/oneshot.go`

### 4a. Add `startupDeps()` helper method on `oneShotRunner`

Add near the other helper methods at the bottom:

```go
// startupDeps builds a StartupDeps from this runner's fields.
func (r *oneShotRunner) startupDeps() StartupDeps {
    return StartupDeps{
        InboxDir:              r.inboxDir,
        InProgressDir:         r.inProgressDir,
        CompletedDir:          r.completedDir,
        LogDir:                r.logDir,
        SpecsInboxDir:         r.specsInboxDir,
        SpecsInProgressDir:    r.specsInProgressDir,
        SpecsCompletedDir:     r.specsCompletedDir,
        SpecsLogDir:           r.specsLogDir,
        PromptManager:         r.promptManager,
        ContainerChecker:      r.containerChecker,
        Notifier:              nil,   // oneshot has no notifier field
        ProjectName:           "",    // oneshot has no projectName field
        SlugMigrator:          r.slugMigrator,
        Mover:                 r.mover,
        CurrentDateTimeGetter: r.currentDateTimeGetter,
    }
}
```

### 4b. Replace the six shared step calls in `oneShotRunner.Run()` with `startupSequence`

In `oneShotRunner.Run()`, the current code after `startupLogger` is:

```go
// Migrate old prompts/queue/ → prompts/in-progress/ if needed
if err := r.migrateQueueDir(ctx); err != nil {
    return errors.Wrap(ctx, err, "migrate queue dir")
}

// Create directories if they don't exist
if err := r.createDirectories(ctx); err != nil {
    return errors.Wrap(ctx, err, "create directories")
}

// Selectively resume or reset executing prompts based on container liveness
if err := r.resumeOrResetExecuting(ctx); err != nil {
    return errors.Wrap(ctx, err, "resume or reset executing prompts")
}

// Reindex all spec and prompt dirs to resolve cross-directory number conflicts
if err := r.reindexAll(ctx); err != nil {
    return errors.Wrap(ctx, err, "reindex files")
}

// Normalize filenames before processing
if err := r.normalizeFilenames(ctx); err != nil {
    return errors.Wrap(ctx, err, "normalize filenames")
}

// Migrate bare spec number refs to full slugs in all prompt lifecycle dirs
if err := r.slugMigrator.MigrateDirs(ctx, []string{
    r.inboxDir, r.inProgressDir, r.completedDir, r.logDir,
}); err != nil {
    return errors.Wrap(ctx, err, "migrate spec slugs")
}

// Loop: generate from approved specs, then drain queue; repeat until idle.
return r.drainLoop(ctx)
```

Replace the six step calls with `startupSequence`:

```go
// Run the six shared startup steps (migrateQueueDir, createDirectories,
// resumeOrResetExecuting, reindexAll, normalizeFilenames, migrateSpecSlugs).
if err := startupSequence(ctx, r.startupDeps()); err != nil {
    return errors.Wrap(ctx, err, "startup sequence")
}

// Loop: generate from approved specs, then drain queue; repeat until idle.
return r.drainLoop(ctx)
```

### 4c. Remove now-unused receiver method wrappers from `oneShotRunner`

After 4b, the following are dead (verify with grep before removing):
- `(*oneShotRunner).migrateQueueDir`
- `(*oneShotRunner).createDirectories`
- `(*oneShotRunner).normalizeFilenames`
- `(*oneShotRunner).reindexAll`
- `(*oneShotRunner).resumeOrResetExecuting`

Run `grep -n "func (r \*oneShotRunner)" pkg/runner/oneshot.go` before and after to enumerate what to remove.

Note: `approveInboxPrompts`, `generateSpecPrompts`, `generateFromApprovedSpecs`, `logInboxPrompts`, and `drainLoop` are all still needed (called from drainLoop / Run). Keep those.

### 4d. Remove now-unused imports in `oneshot.go`

After removing the inline `r.slugMigrator.MigrateDirs` call and the wrapper methods, check if any imports become unused. Specifically, `"strings"` was used in the removed wrapper `generateSpecPrompts` → no wait, `generateSpecPrompts` is still there. Run `go build ./pkg/runner/...` to catch unused imports; fix as needed.

## 5. Add unit tests in `pkg/runner/lifecycle_test.go`

Create a new file `pkg/runner/lifecycle_test.go`. This is an external test (package `runner_test`).

The test verifies that `startupSequence` calls all six steps in order, using mocks to record each call. Since the steps call package-level functions (which in turn call mock methods), we record which mock methods are called and verify all six steps executed.

Use a `callLog []string` and stub each relevant mock to append a string when called.

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner_test

import (
    "context"
    "os"
    "path/filepath"

    libtime "github.com/bborbe/time"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/bborbe/dark-factory/mocks"
    "github.com/bborbe/dark-factory/pkg/runner"
)

var _ = Describe("startupSequence", func() {
    var (
        ctx      context.Context
        cancel   context.CancelFunc
        tempDir  string
        deps     runner.StartupDepsForTest // see requirement 6 for export_test.go helper
    )

    BeforeEach(func() {
        ctx, cancel = context.WithCancel(context.Background())

        var err error
        tempDir, err = os.MkdirTemp("", "lifecycle-test-*")
        Expect(err).NotTo(HaveOccurred())

        promptsBase := filepath.Join(tempDir, "prompts")
        specsBase := filepath.Join(tempDir, "specs")

        manager := &mocks.Manager{}
        manager.NormalizeFilenamesReturns(nil, nil)
        manager.ListExecutingReturns(nil, nil)

        containerChecker := &mocks.ContainerChecker{}

        slugMigrator := &mocks.SpecSlugMigrator{}
        slugMigrator.MigrateDirsReturns(nil)

        mover := &mocks.FileMover{}
        mover.ListReturns(nil, nil) // reindexAll uses mover.List

        deps = runner.StartupDepsForTest{
            InboxDir:              filepath.Join(promptsBase, "inbox"),
            InProgressDir:         filepath.Join(promptsBase, "in-progress"),
            CompletedDir:          filepath.Join(promptsBase, "completed"),
            LogDir:                filepath.Join(promptsBase, "logs"),
            SpecsInboxDir:         filepath.Join(specsBase, "inbox"),
            SpecsInProgressDir:    filepath.Join(specsBase, "in-progress"),
            SpecsCompletedDir:     filepath.Join(specsBase, "completed"),
            SpecsLogDir:           filepath.Join(specsBase, "logs"),
            PromptManager:         manager,
            ContainerChecker:      containerChecker,
            SlugMigrator:          slugMigrator,
            Mover:                 mover,
            CurrentDateTimeGetter: libtime.NewCurrentDateTime(),
        }
    })

    AfterEach(func() {
        cancel()
        _ = os.RemoveAll(tempDir)
    })

    It("creates all lifecycle directories", func() {
        Expect(runner.RunStartupSequenceForTest(ctx, deps)).To(Succeed())
        for _, dir := range []string{
            deps.InboxDir, deps.InProgressDir, deps.CompletedDir, deps.LogDir,
            deps.SpecsInboxDir, deps.SpecsInProgressDir, deps.SpecsCompletedDir, deps.SpecsLogDir,
        } {
            _, err := os.Stat(dir)
            Expect(err).NotTo(HaveOccurred(), "directory should exist: %s", dir)
        }
    })

    It("calls NormalizeFilenames on the in-progress dir", func() {
        Expect(runner.RunStartupSequenceForTest(ctx, deps)).To(Succeed())
        manager := deps.PromptManager.(*mocks.Manager)
        Expect(manager.NormalizeFilenamesCallCount()).To(Equal(1))
        _, dir := manager.NormalizeFilenamesArgsForCall(0)
        Expect(dir).To(Equal(deps.InProgressDir))
    })

    It("calls SlugMigrator.MigrateDirs with the four prompt dirs", func() {
        Expect(runner.RunStartupSequenceForTest(ctx, deps)).To(Succeed())
        migrator := deps.SlugMigrator.(*mocks.SpecSlugMigrator)
        Expect(migrator.MigrateDirsCallCount()).To(Equal(1))
        _, dirs := migrator.MigrateDirsArgsForCall(0)
        Expect(dirs).To(ConsistOf(deps.InboxDir, deps.InProgressDir, deps.CompletedDir, deps.LogDir))
    })

    It("returns an error if NormalizeFilenames fails", func() {
        manager := deps.PromptManager.(*mocks.Manager)
        manager.NormalizeFilenamesReturns(nil, errors.New("normalize error"))
        err := runner.RunStartupSequenceForTest(ctx, deps)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("normalize filenames"))
    })

    It("returns an error if MigrateDirs fails", func() {
        migrator := deps.SlugMigrator.(*mocks.SpecSlugMigrator)
        migrator.MigrateDirsReturns(errors.New("migrate error"))
        err := runner.RunStartupSequenceForTest(ctx, deps)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("migrate spec slugs"))
    })
})
```

**Note on mock methods:** Before writing tests, run `grep -n "func.*Manager\b" mocks/` and `grep -n "func.*SpecSlugMigrator" mocks/` to confirm the exact method names (e.g. `NormalizeFilenamesReturns`, `MigrateDirsReturns`, `MigrateDirsArgsForCall`). Adjust if names differ. Check `mocks/` directory to see what's available.

**Note on imports:** The test uses `errors.New` for creating test errors — import `"errors"` (standard library `stderrors`) or use `github.com/bborbe/errors`. Check the existing test files for the convention used.

## 6. Export `StartupDeps` and `startupSequence` for external tests via `export_test.go`

Add the following to `pkg/runner/export_test.go`:

```go
// StartupDepsForTest re-exports StartupDeps for external test packages.
type StartupDepsForTest = StartupDeps

// RunStartupSequenceForTest exposes startupSequence for external test packages.
func RunStartupSequenceForTest(ctx context.Context, deps StartupDeps) error {
    return startupSequence(ctx, deps)
}
```

`StartupDepsForTest` uses a type alias (`=`) so external tests can pass it directly without conversion.

## 7. Check that mocks exist for `slugmigrator.Migrator`

Run `ls mocks/ | grep -i slug` to verify a mock exists. If it doesn't exist (or the interface changed), regenerate it. The `slugmigrator.Migrator` interface should have `MigrateDirs(ctx context.Context, dirs []string) error`. If the mock is named differently (e.g. `SpecSlugMigrator` vs `SlugMigrator`), use whatever name matches the mocks directory.

## 8. Write `## Unreleased` CHANGELOG entry

Check if `CHANGELOG.md` has an `## Unreleased` section. If not, add one immediately after the first `# Changelog` heading. Append:

```
- refactor: extract shared startupSequence from runner and oneshot into pkg/runner/lifecycle.go
```

If `## Unreleased` already exists, append the bullet to it.

## 9. Run `make precommit`

Run `make precommit` in `/workspace`. It must exit 0. If it fails:
1. Fix the failing target
2. Run only that target (`make lint`, `make test`, etc.)
3. Repeat until all targets pass
4. Then run `make precommit` once more

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- No behavioral changes — all existing tests must pass; the startup steps execute in exactly the same order as before; only the code structure changes
- Error messages from startup failures must remain identical to today — the same `errors.Wrap(ctx, err, "migrate queue dir")` etc. messages are used inside `startupSequence`, so error text is preserved
- Error wrapping uses `github.com/bborbe/errors` — no bare `return err`, no `fmt.Errorf`
- `startupSequence` is NOT exported (lowercase) — it is package-private to `pkg/runner`; only the export_test.go helper exposes it for tests
- Three daemon-only steps (`resumeOrResetGenerating`, `processor.ResumeExecuting`, and the `.git/index.lock` guard) remain in `runner.go` with a clear comment explaining they are daemon-only and why they are not in `startupSequence`
- Context propagation preserved: both runners pass their own `ctx` to `startupSequence`; `startupSequence` never creates a new context
- Do not touch `go.mod`, `go.sum`, or `vendor/`
- Do not move any startup steps from `pkg/runner` into another package
- Do not change the `Runner` or `OneShotRunner` public interfaces
- Do not change what any startup step does — this is purely a structural refactoring
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "r\.migrateQueueDir\|r\.createDirectories\|r\.normalizeFilenames\|r\.migrateSpecSlugs\|r\.reindexAll\|r\.resumeOrResetExecuting" pkg/runner/runner.go` — must return zero matches (all six shared steps moved into startupSequence)
2. `grep -n "r\.migrateQueueDir\|r\.createDirectories\|r\.normalizeFilenames\|r\.slugMigrator\.MigrateDirs\|r\.reindexAll\|r\.resumeOrResetExecuting" pkg/runner/oneshot.go` — must return zero matches
3. `grep -n "startupSequence" pkg/runner/runner.go pkg/runner/oneshot.go` — each file must have exactly one match (the call site)
4. `grep -n "daemon-only" pkg/runner/runner.go` — must show the comments on resumeOrResetGenerating and processor.ResumeExecuting
5. `go test ./pkg/runner/...` — all tests pass, including the new lifecycle_test.go
6. `grep -c "func startupSequence\|StartupDeps" pkg/runner/lifecycle.go` — must return at least 2 (the struct + the function)
</verification>
