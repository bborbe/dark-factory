---
status: completed
spec: [091-doctor-command]
container: dark-factory-doctor-exec-440-spec-091-review-fix-coverage
dark-factory-version: v0.173.0
created: "2026-06-02T00:00:00Z"
queued: "2026-06-02T06:58:24Z"
started: "2026-06-02T07:19:59Z"
completed: "2026-06-02T08:09:07Z"
branch: feature/doctor-command
---

<summary>
- Closes branch-coverage gaps surfaced in the spec-091 review on `pkg/doctor/fix_renumber.go`, `pkg/doctor/verifying_stale.go`, `pkg/doctor/fix_status_dir_mismatch.go`, and the doctor flag-parsing helpers in `main.go`
- Tests-only: no production code is touched (sibling prompt 5 owns the production refactor)
- Exercises every error branch of the duplicate-spec-numbers fixer (reindex failure, lock-timeout, load failure, save failure, move failure, audit-write failure, the empty-rename early return, and the deliberately-swallowed `UpdateSpecRefs` error)
- Adds a missing finding-emission test for the unparseable verifying timestamp path
- Adds explicit success-path coverage for all four directory-move branches of the status/dir-mismatch fixer (specs in-progress -> completed, in-progress -> rejected, prompts in-progress -> completed, in-progress -> cancelled)
- Adds internal-package tests for `extractVerifyingStaleHours` and `validateDoctorArgs` in `main_internal_test.go`
- Matches the existing per-detector test-file convention and uses the existing Counterfeiter mocks under `mocks/` rather than introducing new fakes where one already exists
- Verifies via `make test` once prompt 5 has also been applied (this prompt assumes prompt 5's defer-loop refactor and the dropped `verifyingStaleHours` constructor parameter)
- All new test code is in external test packages (`package doctor_test`) except `main_internal_test.go` which is internal-by-necessity (`package main`)

</summary>

<objective>
Lift branch coverage on the four review hotspots — duplicate-spec-numbers fixer, verifying-stale detector, status/dir-mismatch fixer, and the doctor flag-parsing helpers in `main.go` — without modifying any production code. Sibling prompt 5 already owns production fixes for this spec; this prompt only writes tests against the post-prompt-5 surface.

</objective>

<context>
Read `/workspace/CLAUDE.md` first for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` for Ginkgo v2 + Gomega + `DescribeTable`/`Entry` conventions used across this repo.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md` for Counterfeiter mock conventions (the `mocks/` directory holds all `//go:generate` outputs; reuse before generating).
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` for the `github.com/bborbe/errors` idioms the tests will assert on.

Files to READ end-to-end before writing tests (do NOT modify them):
- `/workspace/specs/completed/091-doctor-command.md` — the spec; do NOT touch it (already shipped, status: completed)
- `/workspace/prompts/in-progress/5-*.md` (if present) OR the queued prompt 5 in `/workspace/prompts/` — owns the production refactor; gives you the post-refactor constructor signature for the doctor `Deps`/`FixerDeps` structs and the new doctor `PromptManager` interface
- `/workspace/pkg/doctor/fix_renumber.go` — entire file; every error branch in lines 28-156 is the target
- `/workspace/pkg/doctor/verifying_stale.go` — lines 56-65 (the `time.Parse` failure branch is the target)
- `/workspace/pkg/doctor/fix_status_dir_mismatch.go` — entire file; the four success paths through `expectedSpecDir`/`expectedPromptDir` are the target
- `/workspace/pkg/doctor/fixer.go` — to see the `FixerDeps` struct fields (`Mover`, `FileLockFactory`, `CurrentDateTimeGetter`, `PromptManager`, `SpecsInProgressDir`, etc.) and the `fixer.Apply` entry point that dispatches by `Category`
- `/workspace/pkg/doctor/doctor.go` — `Deps` struct field names (`VerifyingStaleHours`, `PromptManager` — the latter may have switched from `*prompt.Manager` to a doctor-local `PromptManager` interface in prompt 5; mirror whatever prompt 5 actually produces)
- `/workspace/pkg/doctor/fixer_test.go` — the existing test file for the fixer; extend this file (do NOT introduce a new sibling test file unless the existing file is already >800 lines AND the new tests form a coherent independent describe-block)
- `/workspace/pkg/doctor/verifying_stale_test.go` — entire file; mirror its existing fixture-helper pattern (`createSpecFileWithVerifying`)
- `/workspace/pkg/doctor/status_dir_mismatch_test.go` — to find existing fixture helpers for status/dir-mismatch findings
- `/workspace/pkg/doctor/duplicate_spec_numbers_test.go` — for the duplicate-spec-numbers detector test patterns (uses real `os.WriteFile` to set up fixtures)
- `/workspace/main_internal_test.go` — entire file; the new `extractVerifyingStaleHours` and `validateDoctorArgs` describes go in here, matching the existing `extractMaxContainers`/`validateNoArgs` style
- `/workspace/main.go` lines 697-744 — the actual signatures:
  - `func extractVerifyingStaleHours(ctx context.Context, args []string) (int, []string, error)` (THREE return values: value, remaining args, error)
  - `func validateDoctorArgs(ctx context.Context, args []string) error` (TWO args: ctx + args; NO `helpFn` parameter despite what review notes may say)
- `/workspace/mocks/` — list with `ls /workspace/mocks/ | sort` and reuse: `file-mover.go` (`mocks.FileMover`), `container-lock.go` (`mocks.LockFileLock` — name is historical; this is the `lock.FileLock` mock), `spec-auto-completer.go` (`mocks.AutoCompleter`), `reindexer.go` (`mocks.Reindexer`), the various `*-prompt-manager.go` (use the one matching the post-prompt-5 doctor interface — likely `mocks/cmd-prompt-manager.go` or a newly-generated `mocks/doctor-prompt-manager.go`)

</context>

<requirements>

1. **Discover prompt 5's final shape FIRST.** Before writing any test, run `ls /workspace/prompts/ /workspace/prompts/in-progress/` and read the prompt 5 file (whichever directory it sits in). Note exactly:
   - Whether `Deps` still has a `VerifyingStaleHours int` field (the task description for prompt 5 says it's dropped from the constructor — if so, the tests in this prompt MUST NOT pass `VerifyingStaleHours: 24` and instead rely on the post-refactor default).
   - The exact name and import path of the doctor `PromptManager` interface (if introduced by prompt 5) and the matching mock under `mocks/`. If prompt 5 introduces `pkg/doctor.PromptManager`, regenerate the mock if absent by running `make generate-mocks` once at the start of this prompt's work (do NOT hand-write a fake).
   - Whether `fix_renumber.go` keeps `defer fl.Release(ctx)` inside the loop or moves to an explicit-release helper. Tests that observe lock-release order must match the post-refactor behavior.
   If prompt 5 has NOT yet landed when this prompt runs, STOP and emit an error noting that prompt 6 depends on prompt 5; do not attempt to make tests pass against the unrefactored surface.

2. **`pkg/doctor/fix_renumber.go` branch coverage.** Extend `/workspace/pkg/doctor/fixer_test.go` with a new top-level `Describe("fixDuplicateSpecNumbers", func() { ... })` block. (Match the existing file's style: a single `Describe` with nested `Context`/`It` blocks; do NOT introduce a new sibling test file unless `fixer_test.go` exceeds 800 lines after step 2 is complete — re-check at the end with `wc -l`.) Each `It` or `DescribeTable`/`Entry` row asserts one branch:

   **Reindexer construction note (load-bearing for 2a, 2b, 2j):** the production code at `pkg/doctor/fix_renumber.go:46` constructs `r := reindex.NewReindexer(specDirs, f.deps.Mover)` INLINE — there is no injected `Reindexer` to mock. Drive reindexer behavior via on-disk fixtures + the injected `mocks.FileMover`. Do NOT attempt `mocks.Reindexer`; the doctor fixer doesn't have a `Reindexer` dependency to swap.

   2a. **Happy path** — `Context("when reindex returns one rename for a colliding spec", ...)`:
   - Set up: write two spec files to `specsDir/in-progress` whose names collide on the numeric prefix (e.g. `056-foo.md` and `056-bar.md`); the lex-last is `056-foo.md` (alphabetic) so `056-bar.md` is the duplicate to be renumbered. Use real `os.WriteFile` for the fixtures (matches `verifying_stale_test.go` and `duplicate_spec_numbers_test.go` patterns).
   - Use the real `reindex.NewReindexer`-driven flow (not a mock): the test relies on the production code's inline construction and the `mocks.FileMover` to observe the renames. Inject `mocks.FileMover` whose `MoveFile` returns `nil` and records the call — that's the only `Reindexer` collaborator the test controls.
   - Inject `mocks.LockFileLock` whose `Acquire` returns `nil`.
   - Call `fixer.Apply(ctx, []doctor.Finding{<dup-finding>}, opts)` where the finding has `Category: doctor.CategoryDuplicateSpecNumbers`, `TargetPaths: []string{"056-bar.md"}` (basenames, per the comment at fix_renumber.go:27), `SpecID: "056-foo"`, `FixCommand: "dark-factory spec renumber 056-bar"`.
   - Assert: result contains exactly one `AppliedFix` with `Category == CategoryDuplicateSpecNumbers`, `TargetPaths == [<oldPath>, <newPath>]`, `AuditLine != ""`. Assert `mocks.FileMover.MoveFileCallCount() == 1`. Assert the audit log file at `opts.AuditLogPath` exists and contains a line starting with the RFC3339 timestamp and `duplicate-spec-numbers\tapplied\t`.
   - Read back the new spec file from disk and assert its frontmatter contains `previous_id: 056` UNQUOTED on its own line. The struct tag is `yaml:"previous_id,omitempty"` (verified at `/workspace/pkg/spec/spec.go` lines 147–152 — there is NO `pkg/spec/frontmatter.go` file). Use `os.ReadFile` + `string(...)` + Gomega `ContainSubstring("\nprevious_id: 056\n")` (snake_case, unquoted). The audit explicitly called out the load-bearing distinction between quoted `previous_id: "056"` (wrong, would fail the spec's AC) and unquoted `previous_id: 056` (right).

   2b. **Reindex failure** (around fix_renumber.go:48): force the inline `reindex.Reindex(ctx)` to error. The cheapest reliable trigger is passing a non-existent `SpecsInProgressDir` in `Deps` (the directory-walk inside `reindex.NewReindexer(...).Reindex(ctx)` then fails with a wrapped `os.IsNotExist` error). Alternative: write a malformed file that breaks the prefix scanner — verify against `pkg/reindex/reindexer.go:Reindex` which path actually surfaces an error and use that.
   - Assert: result `failed` contains one `FailedFix` with `Category == CategoryDuplicateSpecNumbers`, `Detail` contains `"reindex"`, and `applied` is empty.

   2c. **Lock acquire failed** (around fix_renumber.go:85): inject a `mocks.LockFileLock` whose `Acquire` returns a timeout-style error (`errors.New(ctx, "lock acquire timeout after 5s")`).
   - Assert: `failed` contains a `FailedFix` whose `Detail` matches `ContainSubstring("lock acquire")` (the production code wraps with `"lock acquire failed: "` — assert against that exact prefix).

   2d. **Load failed** (around fix_renumber.go:98): set the fixture file to a non-readable mode (`os.Chmod(path, 0o000)` then defer-restore) OR make the file's frontmatter unparseable so `spec.Load` returns an error. Run a one-shot probe in the test setup to confirm the failure surfaces (some CI environments run as root and `0o000` is bypassed — prefer the unparseable-frontmatter approach: write `"---\nstatus: !!invalid\n---"` or similar).
   - Assert: `failed[0].Detail` matches `ContainSubstring("load failed")`.

   2e. **Save failed** (around fix_renumber.go:107): the spec file is opened and resaved; trigger a save failure by making the spec file's parent directory read-only AFTER load but BEFORE save. Concrete approach: use a `BeforeEach`-installed file-mode toggle that flips the directory mode between the load and save phases. If that's brittle, alternatively wrap the test in a `Context` that runs after step 2d's `0o000` fix has been applied — but track the ordering carefully. If neither approach is reliable, comment in the test source that `save failed` is not deterministically triggerable from outside and mark the test `Pending()` with an explanatory note (review flagged this branch as untested; if it remains untestable without injection, that finding gets escalated back to prompt 5 to add a `spec.Saver` interface in a follow-up).

   2f. **Move failed** (around fix_renumber.go:116): inject `mocks.FileMover` whose `MoveFile` returns `errors.New(ctx, "rename failed: device or resource busy")`.
   - Assert: `failed[0].Detail` matches `ContainSubstring("move failed")`.

   2g. **Audit log write failed** (around fix_renumber.go:132): set `opts.AuditLogPath` to a path that exists AS A REGULAR FILE inside a path component that is itself a regular file (e.g. write a normal file to `tempDir/audit-blocker`, then set `opts.AuditLogPath = tempDir + "/audit-blocker/log.tsv"` — `os.OpenFile` on the nested path returns `ENOTDIR`).
   - Assert: `failed[0].Detail` matches `ContainSubstring("audit log write failed")`. Assert that despite the audit-write failure, the file was still renamed (the production code appends to `failed` and `continue`s, never rolling back the rename).

   2h. **Empty relevantRenames early-return** (around fix_renumber.go:78): the finding's `TargetPaths` contains a filename that does NOT appear in any of the spec dirs (e.g. `TargetPaths: []string{"999-does-not-exist.md"}`), but the spec directory does contain other collisions so `reindex.Reindex` returns renames whose `OldPath` is none of `oldPathSet`'s entries.
   - Assert: result `applied` is empty AND `failed` is empty (early `return` with no findings emitted).

   2i. **Empty TargetPaths early-return** (around fix_renumber.go:28): finding with `TargetPaths: []string{}`.
   - Assert: result `applied` is empty AND `failed` is empty.

   2j. **`UpdateSpecRefs` error is intentionally swallowed** (around fix_renumber.go:151): drive the reindexer through the real `reindex.NewReindexer` (because the doctor fixer constructs it inline), but force `UpdateSpecRefs` to fail by writing a prompt file under `promptsDir/in-progress/999-bad.md` whose frontmatter is unparseable (so `UpdateSpecRefs`'s prompt-load step errors). Confirm with a probe call to `reindex.UpdateSpecRefs(ctx, ...)` in the test that it does indeed return an error against this fixture. THEN run `fixer.Apply` against a happy-path duplicate-spec fixture (as in 2a) and:
   - Assert: result contains an `AppliedFix` (NOT `FailedFix`) — the rename succeeded.
   - Assert: the audit log line was written.
   - Include a comment in the test source: `// fix_renumber.go:151-154 intentionally swallows UpdateSpecRefs errors — the renumber already succeeded on disk, and spec-ref updates in prompts are best-effort. This test pins that behavior; if you change the production code to surface the error, update this expectation accordingly.`

3. **`pkg/doctor/verifying_stale.go` unparseable-timestamp branch.** Edit `/workspace/pkg/doctor/verifying_stale_test.go`. Add ONE new `It` block inside the existing `Describe("VerifyingStale", ...)` (do NOT introduce a `DescribeTable` — the existing file uses bare `It` blocks; match its style):

   ```go
   It("returns a finding when verifying timestamp is unparseable", func() {
       createSpecFileWithVerifying(
           filepath.Join(specsDir, "inbox"),
           "001-feature.md",
           "verifying",
           "not-a-date",
       )

       deps := doctor.Deps{ /* ...same fields as the empty-timestamp test above... */ }
       checker := doctor.NewChecker(deps)
       findings, err := checker.Check(ctx)
       Expect(err).NotTo(HaveOccurred())

       var staleFindings []doctor.Finding
       for _, f := range findings {
           if f.Category == doctor.CategoryVerifyingStale {
               staleFindings = append(staleFindings, f)
           }
       }
       Expect(staleFindings).To(HaveLen(1))
       Expect(staleFindings[0].Detail).To(ContainSubstring("unparseable"))
       Expect(staleFindings[0].Detail).To(ContainSubstring("not-a-date"))
       Expect(staleFindings[0].FixCommand).To(ContainSubstring("dark-factory spec verify"))
   })
   ```

   Copy the `deps := doctor.Deps{...}` block verbatim from the existing "empty verifying timestamp" test above it. If prompt 5 has removed `VerifyingStaleHours` from `Deps`, drop that line from the copy.

4. **`pkg/doctor/fix_status_dir_mismatch.go` success-path coverage.** Edit `/workspace/pkg/doctor/fixer_test.go` (or extend the existing `Describe` block dedicated to status/dir-mismatch — locate it with `grep -n 'fixStatusDirMismatch\|status.*mismatch' fixer_test.go`). Add a `DescribeTable("status/dir-mismatch success paths", ...)` with **THREE** `Entry` rows — one per real success branch in `expectedSpecDir`/`expectedPromptDir` (verified at `pkg/doctor/fix_status_dir_mismatch.go:125-160` — there is NO branch from `prompts/in-progress/ + status cancelled` to `PromptsCancelledDir`; that path falls through to `return f.deps.PromptsInProgressDir` which is a no-op rename). The three real success branches:

   Each `Entry` is parameterized by:
   - `sourceDir` (e.g. `specsInProgressDir`)
   - `destDir` (the doctor `Deps` field name to read for the expected destination, e.g. `f.deps.SpecsCompletedDir`)
   - `filename` (e.g. `"056-foo.md"`)
   - `fixCommand` (e.g. `"dark-factory spec move 056-foo"`)
   - `detail` (the string the fixer parses to determine `expectedSpecDir`/`expectedPromptDir` — e.g. `"spec in specs/in-progress/ has status completed but only statuses {in-progress, verifying} are allowed in that directory"`)
   - `expectedAfterBasename` (e.g. `"056-foo.md"`)

   Inside the table body:
   - Create the source file with `os.WriteFile(filepath.Join(sourceDir, filename), []byte("---\nstatus: completed\n---\n"), 0o600)`.
   - Call `fixer.Apply(ctx, []doctor.Finding{{Category: doctor.CategoryStatusDirMismatch, TargetPaths: []string{filepath.Join(sourceDir, filename)}, FixCommand: fixCommand, Detail: detail}}, opts)`.
   - Assert: result `applied` has length 1 with `Category == CategoryStatusDirMismatch`, `TargetPaths` ending in `[<oldPath>, <newPath>]`, and `FixCommand == fixCommand`.
   - Assert: `os.Stat(filepath.Join(sourceDir, filename))` returns `os.IsNotExist(err) == true` (source removed).
   - Assert: `os.Stat(filepath.Join(destDir, expectedAfterBasename))` returns nil (dest present).
   - Assert: the audit log file at `opts.AuditLogPath` contains a line for `status-dir-mismatch\tapplied\t`.

   The three Entries (all real success branches):
   - `Entry("spec in-progress + status completed -> SpecsCompletedDir", specsInProgressDir, specsCompletedDir, "056-foo.md", "dark-factory spec move 056-foo", "spec in specs/in-progress/ has status completed but only statuses {in-progress, verifying} are allowed in that directory", "056-foo.md")`
   - `Entry("spec in-progress + status rejected -> SpecsRejectedDir", specsInProgressDir, specsRejectedDir, "057-foo.md", "dark-factory spec move 057-foo", "spec in specs/in-progress/ has status rejected but only statuses {in-progress, verifying} are allowed in that directory", "057-foo.md")`
   - `Entry("prompt in-progress + status completed -> PromptsCompletedDir", promptsInProgressDir, promptsCompletedDir, "1-foo.md", "dark-factory prompt move 1-foo", "prompt in prompts/in-progress/ has status completed", "1-foo.md")`

   Verify the `Detail` strings against the actual producer in `pkg/doctor/status_dir_mismatch.go` BEFORE finalizing the Entry rows — the strings must contain the substrings `expectedSpecDir`/`expectedPromptDir` parse on (`"specs/in-progress/"`, `"status completed"`, etc.).

   **Out of scope for this prompt:** the "no-op" path where `expectedPromptDir` returns the SAME directory the file is already in (e.g. `prompts/in-progress/ + status cancelled` falls through to `PromptsInProgressDir`). That path is a defect in the production detector (status-cancelled in `in-progress/` should either be a legitimate finding mapped to `PromptsCancelledDir` or be filtered out at detection time), not a coverage gap. File as a follow-up if it matters; do NOT write a test that asserts the current buggy fall-through.

5. **`main_internal_test.go` for `extractVerifyingStaleHours` and `validateDoctorArgs`.** Extend the existing `/workspace/main_internal_test.go` (do NOT create a new file). Append two new `Describe` blocks at the end of the file, matching the existing `extractMaxContainers` and `validateNoArgs` patterns exactly (bare `It` blocks, `ctx := context.Background()` at the top of each `Describe`).

   5a. **`extractVerifyingStaleHours`:**
   ```go
   var _ = Describe("extractVerifyingStaleHours", func() {
       ctx := context.Background()

       It("returns default 24 and original args when flag absent", func() {
           n, remaining, err := extractVerifyingStaleHours(ctx, []string{})
           Expect(err).NotTo(HaveOccurred())
           Expect(n).To(Equal(24))
           Expect(remaining).To(BeEmpty())
       })

       It("returns parsed value with --verifying-stale-hours=48", func() {
           n, remaining, err := extractVerifyingStaleHours(ctx, []string{"--verifying-stale-hours=48"})
           Expect(err).NotTo(HaveOccurred())
           Expect(n).To(Equal(48))
           Expect(remaining).To(BeEmpty())
       })

       It("returns error on empty value (--verifying-stale-hours=)", func() {
           _, _, err := extractVerifyingStaleHours(ctx, []string{"--verifying-stale-hours="})
           Expect(err).To(HaveOccurred())
           Expect(err.Error()).To(ContainSubstring("requires a value"))
       })

       It("returns error on non-integer value", func() {
           _, _, err := extractVerifyingStaleHours(ctx, []string{"--verifying-stale-hours=abc"})
           Expect(err).To(HaveOccurred())
           Expect(err.Error()).To(ContainSubstring("positive integer"))
       })

       It("returns error on value less than 1", func() {
           _, _, err := extractVerifyingStaleHours(ctx, []string{"--verifying-stale-hours=0"})
           Expect(err).To(HaveOccurred())
           Expect(err.Error()).To(ContainSubstring("positive integer"))
       })

       It("strips flag from remaining args, preserving other args", func() {
           n, remaining, err := extractVerifyingStaleHours(ctx, []string{"--fix", "--verifying-stale-hours=48", "other"})
           Expect(err).NotTo(HaveOccurred())
           Expect(n).To(Equal(48))
           Expect(remaining).To(Equal([]string{"--fix", "other"}))
       })
   })
   ```

   Verify the error-message substrings against `main.go:705`, `main.go:711-713`, and `main.go:716-720` BEFORE writing. The current text uses `"--verifying-stale-hours requires a value"` and `"--verifying-stale-hours value must be a positive integer, got %q"` — `ContainSubstring("positive integer")` works for both the parse-error and the `n < 1` branches.

   5b. **`validateDoctorArgs`:** the real signature is `func validateDoctorArgs(ctx context.Context, args []string) error` — TWO arguments, NO `helpFn`. The review notes that mention a `helpFn` are wrong; do NOT invent one.
   ```go
   var _ = Describe("validateDoctorArgs", func() {
       ctx := context.Background()

       It("returns nil for empty args", func() {
           Expect(validateDoctorArgs(ctx, []string{})).To(Succeed())
       })

       It("accepts --fix", func() {
           Expect(validateDoctorArgs(ctx, []string{"--fix"})).To(Succeed())
       })

       It("accepts --yes", func() {
           Expect(validateDoctorArgs(ctx, []string{"--yes"})).To(Succeed())
       })

       It("accepts --fix --yes combined", func() {
           Expect(validateDoctorArgs(ctx, []string{"--fix", "--yes"})).To(Succeed())
       })

       It("accepts --verifying-stale-hours=24", func() {
           Expect(validateDoctorArgs(ctx, []string{"--verifying-stale-hours=24"})).To(Succeed())
       })

       It("accepts --help and -h", func() {
           Expect(validateDoctorArgs(ctx, []string{"--help"})).To(Succeed())
           Expect(validateDoctorArgs(ctx, []string{"-h"})).To(Succeed())
       })

       It("returns error on unknown flag", func() {
           err := validateDoctorArgs(ctx, []string{"--unknown"})
           Expect(err).To(HaveOccurred())
           Expect(err.Error()).To(ContainSubstring("unknown flag"))
       })

       It("returns error on positional argument", func() {
           err := validateDoctorArgs(ctx, []string{"some-positional"})
           Expect(err).To(HaveOccurred())
           Expect(err.Error()).To(ContainSubstring("unknown flag"))
       })
   })
   ```

6. **Do NOT touch:**
   - `/workspace/specs/completed/091-doctor-command.md` (spec is already `completed`).
   - Any file under `/workspace/pkg/doctor/` other than `fixer_test.go` and `verifying_stale_test.go`.
   - Any production `.go` file outside of test files. Production refactors live in sibling prompt 5.

7. **After tests are written, run `cd /workspace && make test`.** All tests must pass. If a test fails because prompt 5's refactor changed a struct field name (e.g. `Deps.PromptManager` is now a doctor-local interface and `*prompt.Manager` no longer satisfies it directly), STOP and report — do NOT silently downgrade the test to make it green. The intent of prompt 6 is to lift coverage on real branches, not to paper over a missing wiring step.

8. **Counterfeiter mocks — reuse before generating.** Before writing any fake, run `ls /workspace/mocks/` and check for the existing mock by file name. Concrete reuses:
   - `lock.FileLock` -> `mocks.LockFileLock` (file: `mocks/container-lock.go`)
   - `prompt.FileMover` -> `mocks.FileMover` (file: `mocks/file-mover.go`)
   - `spec.AutoCompleter` -> `mocks.AutoCompleter` (file: `mocks/spec-auto-completer.go`)
   - `reindex.Reindexer` -> `mocks.Reindexer` (file: `mocks/reindexer.go`) — only useful if prompt 5 introduces dependency injection of the reindexer; per current `fix_renumber.go:46` it does not, so the on-disk-fixture approach is mandatory unless prompt 5 changes this.
   - doctor `PromptManager` interface (if introduced by prompt 5) -> regenerate via `make generate-mocks` if the mock file is missing.
   If a needed mock is genuinely absent AND no existing fake matches the interface, regenerate with the project's `make generate-mocks` target. Do NOT hand-write a fake.

</requirements>

<constraints>
- Use Ginkgo v2 + Gomega for all tests. Use `DescribeTable`/`Entry` for matrix cases (the four status/dir-mismatch success paths); use bare `It` blocks where the existing file's style is bare `It` blocks (verifying_stale_test.go, main_internal_test.go).
- All new tests in `pkg/doctor` go in external test packages (`package doctor_test`). `main_internal_test.go` stays `package main` (it's internal-by-necessity; the existing file already is).
- Use `github.com/bborbe/errors` only — never stdlib `errors.New` for test-side error injection. Concrete idiom: `errors.New(ctx, "lock acquire timeout after 5s")`.
- Do NOT modify any production code. Sibling prompt 5 owns the production refactor.
- Do NOT introduce a new top-level `Describe` block in a file that already has one for the same subject — extend the existing block.
- Do NOT add new dependencies to `go.mod`.
- Do NOT commit — dark-factory handles git.
- Branch is already `feature/doctor-command`. Do not switch branches.
- Match the existing per-detector test-file convention: tests for `fix_renumber.go` go in `fixer_test.go` (the project's existing convention groups all fixer tests in one file); tests for `verifying_stale.go` go in `verifying_stale_test.go`; tests for `fix_status_dir_mismatch.go` go in `fixer_test.go` (status-dir-mismatch fixer tests are already there per the existing `Describe` blocks). Re-check `fixer_test.go` line count after step 2 — if it exceeds 1000 lines, split out a new `fix_renumber_test.go` and report the split in the final message.

</constraints>

<verification>
Run from the repo root:

```
cd /workspace && make test
```

All tests must pass.

Spot checks (run after `make test` is green):

```
cd /workspace
grep -c 'fix_renumber\|fixDuplicateSpecNumbers' pkg/doctor/fixer_test.go            # expect >= 8
grep -nE '"not-a-date"|unparseable' pkg/doctor/verifying_stale_test.go              # expect >= 1 line
grep -cE 'spec move|prompt move' pkg/doctor/fixer_test.go                            # expect >= 4
test -f main_internal_test.go && echo present || echo MISSING                        # expect present
grep -c 'extractVerifyingStaleHours\|validateDoctorArgs' main_internal_test.go      # expect >= 8
```

If any spot check fails, fix the gap before declaring done.

</verification>
