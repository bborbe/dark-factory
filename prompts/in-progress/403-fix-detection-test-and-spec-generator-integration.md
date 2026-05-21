---
status: committing
spec: [084-fail-fast-on-worktree-without-hidegit]
summary: Added stat-error detection test case and replaced source-text count assertion with behavioral table test for resolveSpecGeneratorHideGit
container: dark-factory-exec-403-fix-detection-test-and-spec-generator-integration
dark-factory-version: v0.164.0
created: "2026-05-22T00:30:00Z"
queued: "2026-05-21T22:31:28Z"
started: "2026-05-21T22:31:29Z"
branch: dark-factory/fail-fast-on-worktree-without-hidegit
---

<summary>
- Add a 6th case to the `DetectWorktreeOrSubmodule` unit test covering the stat-error shape (`.git` unreadable via parent-dir EACCES) — spec AC requires 5 shapes including stat-error; current test has worktree, submodule, regular, non-git, and symlink-to-dir (no stat-error)
- Extract the spec-generator's hideGit resolution into a small `resolveSpecGeneratorHideGit(cfg config.Config) bool` helper so it can be tested directly with three input shapes
- Replace the source-text count assertion in `pkg/factory/factory_test.go` with a behavioral table test that asserts the helper returns the right value for: `HideGit=true`, `Workflow=WorkflowWorktree`, and both default
- Both gaps were flagged by `spec-verifier` against spec 084 — concerns A (test mismatch) and B (source-text count is inspection-only)
</summary>

<objective>
Close the two AC gaps spec-verifier surfaced on spec 084: (1) add a stat-error detection-test case so the spec's 5-shape requirement is honestly met; (2) replace the source-text count with a behavioral test of the spec-generator's hideGit resolution logic — extract a `resolveSpecGeneratorHideGit` helper and table-test it across the three input shapes the spec cares about.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `specs/in-progress/084-fail-fast-on-worktree-without-hidegit.md` — the spec's acceptance criteria, specifically the "Unit test exercises the detection helper across five CWD shapes" AC and the "Integration test… drives the spec generator with hideGit=true" AC.
Read `pkg/runner/runner_test.go` lines 1175-1250 — existing 5-case Describe block for `DetectWorktreeOrSubmodule`; the symlink-to-dir case stays as case 5, the new stat-error case is added as case 6.
Read `pkg/runner/worktree.go` lines 25-40 — the helper currently wraps stat errors with the substring `"lstat .git failed"` (NOT `"stat .git failed"`).
Read `pkg/factory/factory.go` lines 633-665 — `CreateSpecGenerator`; line 654 has the inline `cfg.Workflow == config.WorkflowWorktree || cfg.HideGit` expression that will be replaced with a call to the new helper.
Read `pkg/factory/factory_test.go` lines 1-42 — the test file uses `config.Defaults()` in `BeforeEach` and imports `libtime "github.com/bborbe/time"`, `"github.com/bborbe/dark-factory/pkg/config"`. No `currentDateTimeGetterMock`-style locals exist; use the real getter `libtime.NewCurrentDateTime()` when one is needed.
Read `pkg/factory/factory_test.go` lines 386-410 — existing `hideGit fragment wiring` Describe block with the source-text count assertion (`strings.Count` against the file contents) that must be replaced.
Read `pkg/validationprompt/resolver_test.go` line 75 — existing precedent for `chmod 0000` in tests (no skip-root guard; project tests assume non-root).
</context>

<requirements>

1. **Add stat-error case to the detection test** in `pkg/runner/runner_test.go`. Inside the existing `Describe("DetectWorktreeOrSubmodule", ...)` block, append AFTER the existing symlink-to-directory case (which stays unchanged) as the 6th `It`:

   ```go
   It("stat-error: returns wrapped error when .git is unreadable (parent dir EACCES)", func() {
       parentDir, err := os.MkdirTemp("", "stat-error-test-*")
       Expect(err).NotTo(HaveOccurred())
       defer func() {
           // Restore parent permissions so RemoveAll can clean up.
           _ = os.Chmod(parentDir, 0755)
           _ = os.RemoveAll(parentDir)
       }()

       workDir := filepath.Join(parentDir, "work")
       Expect(os.MkdirAll(workDir, 0755)).To(Succeed())
       Expect(os.WriteFile(filepath.Join(workDir, ".git"), []byte("\n"), 0600)).To(Succeed())
       Expect(os.Chdir(workDir)).To(Succeed())

       // Drop search permission on parent — Lstat(".git") fails with EACCES.
       Expect(os.Chmod(parentDir, 0000)).To(Succeed())

       err = runner.DetectWorktreeOrSubmodule(context.Background())
       Expect(err).To(HaveOccurred())
       Expect(err).NotTo(MatchError(runner.ErrWorktreeOrSubmodule))
       Expect(err.Error()).To(ContainSubstring("lstat .git failed"))
   })
   ```

   IMPORTANT: the wrapped-error substring is `"lstat .git failed"` (from `pkg/runner/worktree.go`), NOT `"stat .git failed"`. Match the production wrapper verbatim.

   Do NOT delete the existing symlink-to-directory case — the Describe block now exercises 6 shapes total, which satisfies the spec AC's "5 shapes minimum".

2. **Extract `resolveSpecGeneratorHideGit` helper** in `pkg/factory/factory.go`. Add as an unexported package-level function (placement: just above `CreateSpecGenerator` at line ~633):

   ```go
   // resolveSpecGeneratorHideGit computes the effective hideGit value the
   // spec-generator's docker executor receives. Mirrors the expression at
   // line 891 (prompt-executor wiring) — both must agree so spec-generation
   // and prompt-execution containers see the same workspace shape.
   func resolveSpecGeneratorHideGit(cfg config.Config) bool {
       return cfg.Workflow == config.WorkflowWorktree || cfg.HideGit
   }
   ```

   Replace the inline expression at `factory.go:654` (`cfg.Workflow == config.WorkflowWorktree || cfg.HideGit`) with `resolveSpecGeneratorHideGit(cfg)`. The behavioral result at runtime is identical.

3. **Replace the source-text count test** in `pkg/factory/factory_test.go`. In the existing `Describe("hideGit fragment wiring", ...)` block:

   a) REMOVE the `It("passes the same hideGit expression to both executor and enricher", ...)` block that uses `strings.Count` on the file contents.

   b) REMOVE the now-unused `const resolvedHideGitExpr = ...` declaration at the top of the Describe block (it was only referenced by the deleted test).

   c) ADD a new behavioral table test:

   ```go
   DescribeTable("resolveSpecGeneratorHideGit",
       func(cfg config.Config, want bool) {
           Expect(factory.ResolveSpecGeneratorHideGitForTest(cfg)).To(Equal(want))
       },
       Entry("default config -> false", config.Config{}, false),
       Entry("HideGit=true -> true", config.Config{HideGit: true}, true),
       Entry("Workflow=worktree -> true", config.Config{Workflow: config.WorkflowWorktree}, true),
       Entry("both set -> true", config.Config{HideGit: true, Workflow: config.WorkflowWorktree}, true),
   )
   ```

   d) Since `resolveSpecGeneratorHideGit` is unexported, expose it for tests via an `export_test.go` file in the same package (`pkg/factory/`):

   ```go
   // pkg/factory/export_test.go
   package factory

   import "github.com/bborbe/dark-factory/pkg/config"

   // ResolveSpecGeneratorHideGitForTest exposes the package-private helper for
   // black-box tests in factory_test.go.
   var ResolveSpecGeneratorHideGitForTest = resolveSpecGeneratorHideGit

   // Suppress "unused" warnings if other Var-style test exports are added later.
   var _ = config.Config{}
   ```

   This is the idiomatic Go pattern for testing unexported symbols from `_test` packages (see Go stdlib `time/export_test.go` for the canonical shape).

4. **Keep the existing enricher behavioral test** untouched — it tests `promptenricher.NewEnricher` directly and is the right shape for spec 085. Do NOT modify the `It("enricher emits hideGit guidance fragment when hideGit=true", ...)` block.

5. **Verification:**
   - `go test ./pkg/runner/... -v` exits 0; the `DetectWorktreeOrSubmodule` describe block shows 6 `It` entries including the new `stat-error` line
   - `go test ./pkg/factory/... -v` exits 0; the new `DescribeTable("resolveSpecGeneratorHideGit", ...)` runs all 4 entries
   - `grep -n 'resolveSpecGeneratorHideGit' pkg/factory/factory.go` returns at least 2 lines (definition + call site)
   - `make precommit` exits 0

</requirements>

<constraints>
- Do NOT delete the existing symlink-to-directory detection-test case — it is useful additional coverage; the spec AC's "5 shapes" is a floor, not a ceiling
- Do NOT change `executor.NewDockerExecutor`'s signature
- Do NOT change `CreateSpecGenerator`'s signature — the helper is internal-only
- Do NOT change `pkg/factory/factory.go:891` (prompt-executor wiring) or any other call site
- Do NOT add new exported accessors on `config.Config` — the helper takes `config.Config` directly
- The new helper MUST live in `pkg/factory/`, not in `pkg/config/` (it's spec-generator wiring logic, not config)
- Use `chmod 0755`/`0000`/`0600` literally — do NOT introduce filesystem-mode constants
- All existing tests in `pkg/factory/...` and `pkg/runner/...` must continue to pass
- Do NOT commit — dark-factory handles git
- Use `errors.Wrapf(ctx, err, "message")` for error wrapping — never `fmt.Errorf`
</constraints>

<verification>
```bash
go test ./pkg/runner/... -v
go test ./pkg/factory/... -v
grep -n 'resolveSpecGeneratorHideGit' pkg/factory/factory.go
make precommit
```
</verification>
