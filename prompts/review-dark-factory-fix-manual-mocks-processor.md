---
status: draft
created: "2026-04-06T00:00:00Z"
---

<summary>
- The processor package's internal test file defines seven manual stub implementations
- The most problematic is a 20-method hand-written stub for the prompt.Manager interface
- When Manager gains new methods the stub silently compiles without them, masking interface drift
- The project standard is to use counterfeiter-generated mocks exclusively
- Moving tests to the external package and using the already-generated mocks eliminates all manual stubs
</summary>

<objective>
Replace all manually-written stubs (`fakeDirtyFileChecker`, `fakeGitLockChecker`, `stubManager`, `stubExecutor`, `stubContainerCounter`, `stubReleaser`) in `pkg/processor/processor_internal_test.go` with counterfeiter-generated mocks from the `mocks/` package, migrating tests to `package processor_test` via an `export_test.go` bridge.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` for the counterfeiter pattern.

Files to read before making changes (read ALL first):
- `pkg/processor/processor_internal_test.go` — the full file (lines 1–943); note which unexported identifiers each test accesses
- `pkg/processor/processor.go` — `DirtyFileChecker` and `GitLockChecker` interfaces; confirm whether `counterfeiter:generate` directives already exist
- `mocks/` — check which processor-related mocks already exist (`prompt-manager.go`, `executor.go`, `releaser.go`, etc.)
- `pkg/runner/export_test.go` — the pattern for export_test.go bridge files
</context>

<requirements>
1. Read `pkg/processor/processor_internal_test.go` fully to catalog:
   a. All unexported types and functions accessed (these will need export_test.go bridges).
   b. All manually-stubbed interfaces and what their stub behavior returns.

2. Add `//counterfeiter:generate` directives to any interfaces in `pkg/processor/processor.go` that do not already have them:
   - `DirtyFileChecker` → `mocks/dirty-file-checker.go`
   - `GitLockChecker` → `mocks/git-lock-checker.go`

3. Run `make generate` to produce the new mocks.

4. Create `pkg/processor/export_test.go` (package `processor`) exposing any unexported functions or variables that tests need, following the pattern in `pkg/runner/export_test.go`.

5. Migrate all test cases from `processor_internal_test.go` to `pkg/processor/processor_test.go` (external `package processor_test`):
   - Replace `fakeDirtyFileChecker` with `mocks.NewDirtyFileChecker()`.
   - Replace `fakeGitLockChecker` with `mocks.NewGitLockChecker()`.
   - Replace `stubManager` (20+ methods) with `mocks.NewManager()` from the already-generated `mocks/prompt-manager.go`.
   - Replace `stubExecutor` with `mocks.NewExecutor()`.
   - Replace `stubContainerCounter` with `mocks.NewContainerCounter()`.
   - Replace `stubReleaser` with `mocks.NewReleaser()`.

6. Delete `pkg/processor/processor_internal_test.go` once all its tests have been migrated.

7. Run `make test` after each migration step to confirm tests still pass.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- All existing test behavior must be preserved — do not remove test cases, only change stub implementation
- Use counterfeiter mocks from `mocks/` package — never hand-write stubs
- External test package naming (`package processor_test`) required for all migrated tests
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
