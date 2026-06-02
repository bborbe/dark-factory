---
status: completed
spec: [091-doctor-command]
container: dark-factory-doctor-exec-439-spec-091-review-fix-correctness
dark-factory-version: v0.173.0
created: "2026-06-02T00:00:00Z"
queued: "2026-06-02T06:58:24Z"
started: "2026-06-02T06:58:26Z"
completed: "2026-06-02T07:12:18Z"
branch: feature/doctor-command
---

<summary>
- Fixes function-scoped `defer` inside `for`-loops in four fixer files so per-iteration file locks release at the end of each iteration (not at function return)
- Eliminates a spurious "prompted-but-not-swept" finding when a prompted spec has zero linked prompts
- Threads `ctx` through `pkg/lock/filelock.go`'s private `tryAcquire` so error wrapping no longer uses `context.Background()`
- Replaces a direct `time.Now()` call in the renumber audit-line renderer with the already-injected time source
- Reorders `pkg/cmd/doctor.go` so the struct definition follows the constructor (interface → constructor → struct → methods)
- Drops the dead `verifyingStaleHours` field/param: it was passed into the doctor command but never read at the cmd layer
- Removes a stranded `_ = specPath` in the duplicate-spec-numbers detector
- Removes the wired-but-never-invoked `GitRunner` dependency (field, default, factory pass-through, test plumbing)
- Builds `doctor.Deps` once in the factory and reuses it for both `NewChecker` and `NewFixer` (was duplicated verbatim)
- Replaces the concrete `*prompt.Manager` field on `doctor.Deps` with a minimal `PromptManager` interface defined inside `pkg/doctor`
- No new behavior, no new CLI flags, no CHANGELOG edits — fix-iteration on already-shipped prompts 435–438 only

</summary>

<objective>
Apply ten mechanical correctness and code-quality fixes from review feedback against the doctor-command implementation. No new behavior: each fix is a small, local rewrite that either eliminates a real bug (defer-in-loop, zero-prompts spurious finding) or removes dead/inconsistent code (unused field, unused param, concrete-type-where-interface-expected, factory deduplication). All existing tests must still pass after the changes; one new test is added for the zero-prompts fix.

</objective>

<context>
Read `/workspace/CLAUDE.md` first for project conventions.

Reference docs (these guide the patterns, do not re-derive them):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `github.com/bborbe/errors` usage with `ctx`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-time-injection.md` — never call `time.Now()` directly when a `CurrentDateTimeGetter` is available
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-architecture-patterns.md` — interface → constructor → struct → methods ordering
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — factory dedup style for shared-deps
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md` — counterfeiter regeneration when an interface changes
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-context-cancellation-in-loops.md` — defer-in-loop is the same family of bug

Files to read end-to-end before editing:
- `/workspace/pkg/doctor/fix_orphan_in_progress.go` — `defer fl.Release(ctx)` inside `for _, path := range finding.TargetPaths` at line 31
- `/workspace/pkg/doctor/fix_renumber.go` — `defer fl.Release(ctx)` inside the `relevantRenames` loop at line 93; `renderAuditLine` at line 159 uses `time.Now()` directly at line 162 and has an unused `logPath string` parameter
- `/workspace/pkg/doctor/fix_status_dir_mismatch.go` — `defer fl.Release(ctx)` inside the `for _, path := range finding.TargetPaths` loop at line 74
- `/workspace/pkg/doctor/fix_unlink.go` — `defer fl.Release(ctx)` inside the `for _, path := range finding.TargetPaths` loop at line 42
- `/workspace/pkg/doctor/prompted_not_swept.go` — `linkedPromptsAllTerminal` at lines 68-96; the `total == 0` case is implicit (the loop never runs ⇒ `(true, 0, nil)` is returned)
- `/workspace/pkg/doctor/prompted_not_swept_test.go` — Ginkgo style; the new spec follows the existing `var _ = Describe("PromptedNotSwept", func() { ... })` block
- `/workspace/pkg/lock/filelock.go` — `tryAcquire()` at line 69; `context.Background()` appears at line 73 and line 81; `Acquire(ctx context.Context, ...)` at line 45 is the only caller
- `/workspace/pkg/doctor/duplicate_spec_numbers.go` — `specPath` assignment and `_ = specPath` discard inside the `for _, d := range specDirs` loop at lines 60-79 (the var is set at line 64 and discarded at line 79)
- `/workspace/pkg/doctor/fixer.go` — `FixerDeps.GitRunner` at line 78; default at lines 94-96; `GitRunner` interface + `noopGitRunner` at lines 206-225 (all wired but no call site)
- `/workspace/pkg/doctor/fixer_test.go` — references `mocks.SubprocRunner` and `GitRunner` field at lines 65, 66, 98 (these must be removed)
- `/workspace/pkg/doctor/doctor.go` — `Deps.PromptManager *prompt.Manager` at line 75 (the only concrete-type field in `Deps`)
- `/workspace/pkg/cmd/doctor.go` — struct `doctorCommand` at line 28 is BEFORE constructor `NewDoctorCommand` at line 35; field `verifyingStaleHours` at line 31, constructor param at line 38, both never read
- `/workspace/pkg/factory/factory.go` — `CreateDoctorCommand` at lines 1118-1191; the `doctor.Deps` literal at lines 1153-1166 is repeated verbatim inside the `FixerDeps.Deps:` literal at lines 1169-1182; `GitRunner: subproc.NewRunner()` at line 1186; final call `cmd.NewDoctorCommand(checker, fixer, verifyingStaleHours)` at line 1190 passes `verifyingStaleHours` as the third arg
- `/workspace/main.go` — caller of `factory.CreateDoctorCommand(ctx, cfg, hours, currentDateTimeGetter)` at line 199; `hours` continues to flow into `Deps.VerifyingStaleHours` via the factory, so the public `CreateDoctorCommand` signature stays unchanged — only the `cmd.NewDoctorCommand` call inside loses one argument
- `/workspace/pkg/reindex/prompt_manager.go` — `reindex.PromptManager` interface declares `Load(ctx, path) (*prompt.PromptFile, error)` (no `MoveToCancelled`); the doctor's new interface MUST be a superset so it can still be passed to `reindex.UpdateSpecRefs` (see `fix_renumber.go` line 151)
- `/workspace/pkg/prompt/prompt.go` — `Manager.Load` at line 851 returns `(*PromptFile, error)`; `Manager.MoveToCancelled` at line 906 returns `error`. These are the exact two methods doctor uses.

</context>

<requirements>

## 1. Defer-in-loop in four fixer files (correctness — accumulates locks)

In each of these four files, the per-path body inside `for _, path := range finding.TargetPaths` ends with `defer fl.Release(ctx)`. A function-scoped defer inside a loop accumulates: locks for early-iteration paths are not released until the entire function returns. Fix by extracting the per-path body into a private method whose `defer fl.Release(ctx)` is genuinely function-scoped.

Apply the same shape uniformly to all four files. For each file, do the following:

### 1a. `/workspace/pkg/doctor/fix_orphan_in_progress.go`

Extract the body inside `for _, path := range finding.TargetPaths { ... }` (lines ~21-101 in the existing file) into a new method on `*fixer`:

```go
func (f *fixer) applyOrphanInProgressPath(
    ctx context.Context,
    path string,
    finding Finding,
    opts ApplyOptions,
) (applied *AppliedFix, skipped *SkippedFix, failed *FailedFix) {
    fl := f.deps.FileLockFactory(path)
    if err := fl.Acquire(ctx, opts.FileLockTimeout); err != nil {
        return nil, nil, &FailedFix{
            Category:    finding.Category,
            TargetPaths: []string{path},
            Detail:      "lock acquire failed: " + err.Error(),
        }
    }
    defer fl.Release(ctx)

    // ... rest of the existing per-path body, returning pointers in place of
    // `continue` / append branches. Use early `return nil, nil, &FailedFix{...}`
    // for failures, `return nil, &SkippedFix{...}, nil` for skips, and
    // `return &AppliedFix{...}, nil, nil` for the happy path.
}
```

Then rewrite `fixOrphanInProgressPrompt` so the outer loop calls the helper and accumulates results:

```go
func (f *fixer) fixOrphanInProgressPrompt(
    ctx context.Context,
    finding Finding,
    opts ApplyOptions,
) (applied []AppliedFix, skipped []SkippedFix, failed []FailedFix) {
    for _, path := range finding.TargetPaths {
        af, sf, ff := f.applyOrphanInProgressPath(ctx, path, finding, opts)
        if af != nil {
            applied = append(applied, *af)
        }
        if sf != nil {
            skipped = append(skipped, *sf)
        }
        if ff != nil {
            failed = append(failed, *ff)
        }
    }
    return
}
```

### 1b. `/workspace/pkg/doctor/fix_unlink.go`

Same shape. Helper method: `applyOrphanPromptLinkPath(ctx, path, finding, opts) (applied *AppliedFix, failed *FailedFix)`. (No skipped path in this file — keep the two-pointer return signature.) Outer `fixOrphanPromptLink` becomes the accumulator. Move the `orphanSpecID` empty-string check OUT of the loop — that one already runs once before the loop (it does in the current file) so it stays where it is, returning a single FailedFix in `failed` before entering the per-path loop.

### 1c. `/workspace/pkg/doctor/fix_status_dir_mismatch.go`

Same shape. The pre-loop logic that derives `expectedDir` and `filename` (lines ~25-62) STAYS in the outer function — `expectedDir` and `filename` are loop-invariant and depend only on `finding.FixCommand`/`finding.Detail`. Extract only the inside-loop body into:

```go
func (f *fixer) applyStatusDirMismatchPath(
    ctx context.Context,
    path string,
    expectedDir string,
    filename string,
    finding Finding,
    opts ApplyOptions,
) (applied *AppliedFix, failed *FailedFix) { ... }
```

Outer `fixStatusDirMismatch` keeps the pre-loop validation, then iterates and calls the helper.

### 1d. `/workspace/pkg/doctor/fix_renumber.go`

Same shape. Helper: `applyDuplicateSpecNumbersRename(ctx, rn reindex.Rename, finding Finding, opts ApplyOptions) (applied *AppliedFix, failed *FailedFix)`. The iteration variable is `rn` (a `reindex.Rename`), not `path` — keep that name. The pre-loop reindex/filter logic (lines ~32-80) stays in the outer function. The post-loop `reindex.UpdateSpecRefs` call (line 151) ALSO stays in the outer function.

### Constraints common to 1a-1d

- Do NOT change exported behavior — the outer functions keep their existing signatures and return the same `[]AppliedFix` / `[]SkippedFix` / `[]FailedFix` slices in the same order as today.
- Helper methods are unexported (lowercase first letter on the method name).
- Helper methods take `path` (or `rn` for fix_renumber) as a parameter, not as a captured loop variable.
- Inside each helper, the `defer fl.Release(ctx)` is the only `defer` — and it is now function-scoped on the helper, not on the outer function. That is the entire point of the refactor.

## 2. `prompted_not_swept.go` zero-prompts spurious finding (correctness)

In `/workspace/pkg/doctor/prompted_not_swept.go`, function `linkedPromptsAllTerminal` (lines 68-96) returns `(true, 0, nil)` when zero prompts reference the spec — because the loop body never executes and falls through to the final `return true, total, nil`. The caller `detectPromptedNotSwept` then treats `allTerminal=true` as "ready to sweep" and emits a `CategoryPromptedNotSwept` finding for a spec that has no prompts at all. That is wrong — a prompted spec with zero linked prompts is in an indeterminate state (probably a stale prompted status from before any prompt was created), not a sweep candidate.

Fix: after the loop, before `return true, total, nil`, insert:

```go
if total == 0 {
    return false, 0, nil
}
```

The final return remains `return true, total, nil` for the `total > 0 && allTerminal` case.

Add a Ginkgo `It("does not fire on a prompted spec with zero linked prompts", ...)` to `/workspace/pkg/doctor/prompted_not_swept_test.go`. The test:

- Creates a spec file in `specs/inbox/` with status `prompted` (use the existing `createSpecFile` helper visible in the test file).
- Creates ZERO prompt files referencing that spec.
- Builds the same `doctor.Deps` literal the existing `It` blocks build.
- Calls `checker.Check(ctx)`.
- Asserts `findings` does NOT contain any finding with `Category == CategoryPromptedNotSwept` for that spec. Use `Expect(findings).To(BeEmpty())` if no other detectors fire on the empty fixture, or filter by category if other findings are expected.

This is a one-test addition, not a coverage push — broader coverage is sibling prompt 6's responsibility.

## 3. `pkg/lock/filelock.go` thread ctx through `tryAcquire`

In `/workspace/pkg/lock/filelock.go`:

- Change the signature of the private method `tryAcquire` from `func (f *fileLock) tryAcquire() error` to `func (f *fileLock) tryAcquire(ctx context.Context) error`.
- Inside `tryAcquire`, replace both `context.Background()` references:
  - Line 73 `errors.Wrap(context.Background(), err, "open lock file")` → `errors.Wrap(ctx, err, "open lock file")`
  - Line 81 `errors.Errorf(context.Background(), "flock failed: %v", err)` → `errors.Errorf(ctx, "flock failed: %v", err)`
- Update the one call site in `Acquire` (line 62) from `if f.tryAcquire() == nil {` to `if f.tryAcquire(ctx) == nil {`.

No public API change — `tryAcquire` is unexported. The `Acquire` signature already takes `ctx`; this just plumbs it one level deeper. The `FileLock` interface (line 23) is unchanged.

## 4. `fix_renumber.go` audit-line renderer: inject time + drop unused param

In `/workspace/pkg/doctor/fix_renumber.go`:

- The `renderAuditLine` function at line 159 currently has signature `func renderAuditLine(logPath string, finding Finding, before, after string) string` and calls `time.Now().Format(time.RFC3339)` at line 162.
- The `logPath` parameter is accepted but never used in the body — drop it from the signature.
- The `time.Now()` call must use the injected time source. Since `renderAuditLine` is a package-level function (not a method on `*fixer`), thread the time source as a parameter:
  - New signature: `func renderAuditLine(now time.Time, finding Finding, before, after string) string`
  - Body uses `now.Format(time.RFC3339)` instead of `time.Now().Format(time.RFC3339)`.
- Update the only call site (after extracting the per-rename body per requirement 1d, this call moves into the helper `applyDuplicateSpecNumbersRename`). The call site currently reads:
  ```go
  auditLine := renderAuditLine(opts.AuditLogPath, finding, rn.OldPath, rn.NewPath)
  ```
  Change to:
  ```go
  auditLine := renderAuditLine(time.Time(f.deps.CurrentDateTimeGetter.Now()), finding, rn.OldPath, rn.NewPath)
  ```
  (Mirrors the existing `time.Time(f.deps.CurrentDateTimeGetter.Now())` cast already used a few lines below at the `WriteAuditEntry` call — same shape.)

If `time` is no longer imported after the change (it still is, for `time.RFC3339` and the WriteAuditEntry call), keep it. Do NOT remove the import — `time.RFC3339` is still referenced.

## 5. `pkg/cmd/doctor.go` struct-after-constructor ordering

In `/workspace/pkg/cmd/doctor.go`, the current order is:

1. `type DoctorCommand interface` (line 23) — interface
2. `type doctorCommand struct` (line 28) — struct
3. `func NewDoctorCommand(...) DoctorCommand` (line 35) — constructor
4. `func (d *doctorCommand) Run(...)` (line 48) — method

The codebase's interface → constructor → struct → methods order means the struct should sit AFTER the constructor. Move the entire `doctorCommand struct` block (lines 27-32 including its leading comment) to immediately AFTER the `NewDoctorCommand` function's closing brace and BEFORE the `Run` method. New order:

1. `type DoctorCommand interface`
2. `func NewDoctorCommand(...) DoctorCommand`
3. `type doctorCommand struct`
4. `func (d *doctorCommand) Run(...)`

(After requirement 6 below also runs, the struct will have one fewer field and the constructor will have one fewer parameter — apply both changes together.)

## 6. `pkg/cmd/doctor.go` drop dead `verifyingStaleHours`

The field `verifyingStaleHours int` on `doctorCommand` (line 31), the constructor parameter at line 38, the assignment at line 43, and the third argument to `cmd.NewDoctorCommand(checker, fixer, verifyingStaleHours)` at `pkg/factory/factory.go` line 1190 are all dead — the integer value is already baked into the Checker (`Deps.VerifyingStaleHours` at `doctor.go:77`) and into the Fixer's embedded `Deps` at factory time, so the `cmd` layer never reads its own field.

Apply all three deletions in one atomic change:

- `/workspace/pkg/cmd/doctor.go`:
  - Drop the `verifyingStaleHours int` field from `doctorCommand`.
  - Drop the `verifyingStaleHours int,` parameter from `NewDoctorCommand`.
  - Drop the `verifyingStaleHours: verifyingStaleHours,` line from the `doctorCommand` literal in the constructor body.
- `/workspace/pkg/factory/factory.go`:
  - At line 1190, change `return cmd.NewDoctorCommand(checker, fixer, verifyingStaleHours)` to `return cmd.NewDoctorCommand(checker, fixer)`.
  - The local `verifyingStaleHours` parameter at line 1122 of `CreateDoctorCommand` STAYS — it is still passed into `doctor.Deps.VerifyingStaleHours` (lines 1165 and 1181).

The public `factory.CreateDoctorCommand(ctx, cfg, hours, currentDateTimeGetter)` signature is UNCHANGED — `main.go:199` keeps compiling without edits.

Counterfeiter regeneration: the `DoctorCommand` interface itself is unchanged (it has only `Run`), so the existing `mocks/doctor-command.go` fake should still satisfy the interface. Run `make generate` (or whatever the repo's counterfeiter target is — `grep -n 'counterfeiter\|generate:' Makefile`) anyway to be safe; if no diff is produced, nothing to commit. If a diff IS produced, include it.

## 7. `duplicate_spec_numbers.go` remove dead `specPath`

In `/workspace/pkg/doctor/duplicate_spec_numbers.go`, inside the `for _, name := range names` loop (lines 54-80):

- Line 60 declares `var specPath string`.
- Line 64 assigns `specPath = p` inside the inner `for _, d := range specDirs` loop.
- Line 79 reads `_ = specPath` — a blank-discard with no further use.

Remove the `var specPath string` declaration, the `specPath = p` assignment, and the `_ = specPath` line. Verify with `grep -n 'specPath' /workspace/pkg/doctor/duplicate_spec_numbers.go` returns 0 lines after the fix.

## 8. Remove unused `GitRunner` dependency

`FixerDeps.GitRunner` (line 78 of `pkg/doctor/fixer.go`) is wired through the factory but never invoked anywhere in `pkg/doctor/`. Verify with `grep -rn 'GitRunner\|gitRunner\|deps\.GitRunner' /workspace/pkg/doctor/ /workspace/pkg/factory/factory.go` before the fix — confirm zero call sites (only declarations, defaults, and test fixtures).

Apply all of these in one atomic change:

- `/workspace/pkg/doctor/fixer.go`:
  - Remove the `GitRunner GitRunner` field from `FixerDeps` (line 78).
  - Remove the default-wiring block inside `NewFixer` (lines 94-96: `if deps.GitRunner == nil { deps.GitRunner = &noopGitRunner{} }`).
  - Remove the entire `GitRunner` interface declaration (lines 206-214).
  - Remove the `noopGitRunner` struct and its method (lines 216-225).
- `/workspace/pkg/factory/factory.go`:
  - Remove the `GitRunner: subproc.NewRunner(),` line at line 1186 from the `FixerDeps` literal.
  - If `subproc` is no longer referenced in `factory.go` after this removal, drop the import; otherwise leave it (it's used at lines 733 and 767 for other factories, so the import stays).
- `/workspace/pkg/doctor/fixer_test.go`:
  - Remove the `fakeGitRunner := &mocks.SubprocRunner{}` and `fakeGitRunner.RunWithWarnAndTimeoutReturns(nil, nil)` lines (lines 65-66).
  - Remove the `GitRunner: fakeGitRunner,` field from the `FixerDeps` literal (line 98).
  - If `mocks.SubprocRunner` is no longer referenced in this test file after this removal, the import line for `mocks` stays anyway (other mock types from the same import are still used).

Verify after the fix: `grep -rn 'GitRunner\|gitRunner\|noopGitRunner' /workspace/pkg/doctor/ /workspace/pkg/factory/factory.go` returns 0 lines.

## 9. Factory: deduplicate `doctor.Deps` literal

In `/workspace/pkg/factory/factory.go`, `CreateDoctorCommand` (lines 1118-1191) currently constructs `doctor.Deps` twice — once as the argument to `doctor.NewChecker` (lines 1153-1166) and once embedded inside the `FixerDeps.Deps:` field (lines 1169-1182). The two literals are identical field-for-field.

Refactor to build it once:

```go
deps := doctor.Deps{
    SpecsInboxDir:         cfg.Specs.InboxDir,
    SpecsInProgressDir:    cfg.Specs.InProgressDir,
    SpecsCompletedDir:     cfg.Specs.CompletedDir,
    SpecsRejectedDir:      cfg.Specs.RejectedDir,
    PromptsInboxDir:       cfg.Prompts.InboxDir,
    PromptsInProgressDir:  cfg.Prompts.InProgressDir,
    PromptsCompletedDir:   cfg.Prompts.CompletedDir,
    PromptsCancelledDir:   cfg.Prompts.CancelledDir,
    SpecLister:            specLister,
    PromptManager:         promptManager,
    CurrentDateTimeGetter: currentDateTimeGetter,
    VerifyingStaleHours:   verifyingStaleHours,
}

checker := doctor.NewChecker(deps)

fixer := doctor.NewFixer(doctor.FixerDeps{
    Deps:                  deps,
    AutoCompleter:         autoCompleter,
    Mover:                 releaser,
    FileLockFactory:       lock.NewFileLock,
    CurrentDateTimeGetter: currentDateTimeGetter,
})
```

(Note the `FixerDeps` literal no longer has `GitRunner:` per requirement 8.)

Verify: `grep -c 'doctor.Deps{' /workspace/pkg/factory/factory.go` returns exactly `1` after the fix.

## 10. `doctor.Deps.PromptManager` concrete → interface

In `/workspace/pkg/doctor/doctor.go`, line 75 declares `PromptManager *prompt.Manager`. Every other field on `Deps` is either an interface, a primitive, or a path string. The concrete-type dep makes test mocking harder than needed.

Define a minimal interface inside `pkg/doctor/doctor.go` (same file, near the top of the file, after `Category` constants but before `Finding`):

```go
//counterfeiter:generate -o ../../mocks/doctor-prompt-manager.go --fake-name DoctorPromptManager . PromptManager

// PromptManager is the subset of prompt.Manager that the doctor package uses.
type PromptManager interface {
    Load(ctx context.Context, path string) (*prompt.PromptFile, error)
    MoveToCancelled(ctx context.Context, path string) error
}
```

(Method signatures must match `*prompt.Manager.Load` and `*prompt.Manager.MoveToCancelled` verbatim per `pkg/prompt/prompt.go:851` and `pkg/prompt/prompt.go:906`.)

Change `Deps.PromptManager` field type from `*prompt.Manager` to `PromptManager`. The struct field name stays `PromptManager`.

No other source code change should be required — `*prompt.Manager` satisfies the new interface structurally (Go duck-typing), so:

- The factory at `pkg/factory/factory.go:1163` passing the concrete `promptManager` continues to work — `*prompt.Manager` implements both methods.
- The call site `reindex.UpdateSpecRefs(ctx, renames, promptDirs, f.deps.Mover, f.deps.PromptManager)` in `fix_renumber.go:151` continues to work — `doctor.PromptManager` is a superset of `reindex.PromptManager` (both declare `Load` with the same signature; Go satisfies `reindex.PromptManager` automatically).
- Existing tests that pass a concrete `*prompt.Manager` via `pm` in the test fixture continue to work.

Run `make generate` after editing — this MUST produce `/workspace/mocks/doctor-prompt-manager.go`. Include the generated file in the result. If your environment cannot run counterfeiter, document the failure in the verification report; do NOT hand-write the mock.

Verify:
- `grep -nE 'PromptManager.*\*prompt\.Manager' /workspace/pkg/doctor/doctor.go` returns 0 lines.
- `grep -n 'type PromptManager interface' /workspace/pkg/doctor/doctor.go` returns exactly 1 line.

## 11. Run all tests

```
cd /workspace && make test
```

If any test fails, fix it in this prompt before declaring done. The expected delta to test files is bounded:

- `pkg/doctor/prompted_not_swept_test.go` — one new `It` block (requirement 2).
- `pkg/doctor/fixer_test.go` — three lines removed (requirement 8).
- No other test files should need edits. If a test fails because of the `Deps.PromptManager` interface change (requirement 10), that's a true type-mismatch and likely requires updating the test's `Deps` literal — fix the test, do not revert the interface change.

</requirements>

<constraints>

- Use `github.com/bborbe/errors` only; never `fmt.Errorf`; always pass `ctx` to `errors.Wrap` / `errors.Errorf`.
- Do NOT change any PUBLIC API signature except:
  - `NewDoctorCommand` (drops one parameter — requirement 6).
  - `Deps.PromptManager` field type (concrete → interface — requirement 10; structurally compatible, no caller edits needed).
  Both are confirmed safe by grep — no callers outside this repo.
- Do NOT modify the parent spec at `/workspace/specs/completed/091-doctor-command.md` (lifecycle: `completed` — already shipped). Content is frozen regardless of where it sits in the spec lifecycle.
- Do NOT modify `/workspace/CHANGELOG.md` — sibling prompt 6 owns any follow-up entry.
- Do NOT add new CLI flags, new config keys, new metrics, or new behavior. Every change in this prompt either deletes code, renames a parameter, or extracts a helper to fix a specific bug.
- Do NOT add new exported symbols beyond the `doctor.PromptManager` interface declared in requirement 10.
- Do NOT commit — dark-factory handles git. Branch is already `feature/doctor-command`; commits land there.
- Do NOT skip `make test`. All existing tests must still pass after the changes.
- Counterfeiter regeneration is mandatory after the `Deps.PromptManager` interface change (requirement 10). If the repo's generate target is `make generate`, use it; otherwise inspect the Makefile for the right target and run that.
- The four `defer fl.Release(ctx)` extractions (requirements 1a-1d) MUST follow the same shape — uniform helper-method naming, uniform pointer-return convention, uniform outer-loop accumulator. Reviewer should be able to skim all four diffs and see the same pattern.
- Helper methods extracted in requirements 1a-1d are unexported and live in the same file as the outer function they support — do NOT spread them across new files.

</constraints>

<verification>

Run all tests:

```
cd /workspace && make test
```

Then run these grep spot-checks. Each command and its expected outcome are listed; any mismatch is a failure of this prompt.

1. **defer-in-loop is gone in all four files.** Run:
   ```
   grep -nE 'defer .*Release\(ctx\)' /workspace/pkg/doctor/fix_orphan_in_progress.go /workspace/pkg/doctor/fix_renumber.go /workspace/pkg/doctor/fix_status_dir_mismatch.go /workspace/pkg/doctor/fix_unlink.go
   ```
   Every match must be inside the body of a helper method (`applyOrphanInProgressPath`, `applyDuplicateSpecNumbersRename`, `applyStatusDirMismatchPath`, `applyOrphanPromptLinkPath`), NOT inside a `for _, path := range` or `for _, rn := range` loop in the outer `fix*` function. To confirm, run the same grep with `-B 5` and visually inspect that each match is inside a helper method body, not below a `range` loop in the outer function. If any match still appears below a `range` line within the same function, the fix is incomplete for that file.

2. **GitRunner is fully removed.**
   ```
   grep -rn 'GitRunner\|gitRunner\|noopGitRunner' /workspace/pkg/doctor/ /workspace/pkg/factory/factory.go
   ```
   Returns 0 lines.

3. **specPath dead-store gone.**
   ```
   grep -n 'specPath' /workspace/pkg/doctor/duplicate_spec_numbers.go
   ```
   Returns 0 lines.

4. **time.Now() gone from fix_renumber.go.**
   ```
   grep -nE 'time\.Now\(\)' /workspace/pkg/doctor/fix_renumber.go
   ```
   Returns 0 lines.

5. **verifyingStaleHours field is gone from pkg/cmd/doctor.go.**
   ```
   grep -n 'verifyingStaleHours' /workspace/pkg/cmd/doctor.go
   ```
   Returns 0 lines.

   In `/workspace/pkg/factory/factory.go`, the local parameter `verifyingStaleHours int` on `CreateDoctorCommand` stays (it still flows into `Deps.VerifyingStaleHours`), so:
   ```
   grep -cn 'verifyingStaleHours' /workspace/pkg/factory/factory.go
   ```
   Returns ≥ 1 (the parameter, plus the two `Deps.VerifyingStaleHours: verifyingStaleHours` field assignments — after requirement 9 dedupes the literal, that drops to one). At most 3, at least 1.

6. **Struct sits after constructor in pkg/cmd/doctor.go.**
   ```
   grep -nE '^func NewDoctorCommand|^type doctorCommand struct' /workspace/pkg/cmd/doctor.go
   ```
   Two lines. The `func NewDoctorCommand` line number MUST be LOWER (earlier in the file) than the `type doctorCommand struct` line number.

7. **context.Background() gone from pkg/lock/filelock.go.**
   ```
   grep -nE 'context\.Background\(\)' /workspace/pkg/lock/filelock.go
   ```
   Returns 0 lines.

8. **Deps.PromptManager is now an interface, not *prompt.Manager.**
   ```
   grep -nE 'PromptManager.*\*prompt\.Manager' /workspace/pkg/doctor/doctor.go
   ```
   Returns 0 lines.
   ```
   grep -n 'type PromptManager interface' /workspace/pkg/doctor/doctor.go
   ```
   Returns exactly 1 line.

9. **doctor.Deps literal deduped in factory.**
   ```
   grep -c 'doctor.Deps{' /workspace/pkg/factory/factory.go
   ```
   Returns exactly `1`.

10. **New mock generated for doctor.PromptManager.**
    ```
    ls /workspace/mocks/doctor-prompt-manager.go
    ```
    File exists.

11. **Zero-prompts test added.**
    ```
    grep -n 'zero linked prompts' /workspace/pkg/doctor/prompted_not_swept_test.go
    ```
    Returns ≥ 1 line.

12. **Final build + test gate.**
    ```
    cd /workspace && make test
    ```
    Exits 0. No new test failures introduced.

</verification>
