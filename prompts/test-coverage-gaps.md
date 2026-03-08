---
status: created
---

<objective>
Add missing tests for `spec.Lister.List()`, `spec.Lister.Summary()`, and `report.ChangelogSuffix()`. These are untested business logic identified by coverage analysis.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/spec/lister.go` — `Lister` interface, `lister` struct, `List()` method, `Summary()` method with status tallying logic.
Read `pkg/spec/spec_test.go` — existing tests to follow pattern; add a new `Describe("Lister")` block.
Read `pkg/spec/spec.go` — `StatusDraft`, `StatusApproved`, `StatusPrompted`, `StatusVerifying`, `StatusCompleted` constants, `SpecFile` struct, `Frontmatter` struct.
Read `pkg/report/suffix.go` — `ChangelogSuffix()` function.
Read `pkg/report/report_test.go` — existing tests for `Suffix()` and `ValidationSuffix()` to follow pattern.
Read `/home/node/.claude/docs/go-testing.md`.
</context>

<requirements>
1. In `pkg/spec/spec_test.go`, add `Describe("Lister", func() { ... })` block with these test cases:

   a. `List` with specs in multiple directories (draft, completed):
      - Create temp dirs with `.md` spec files containing frontmatter
      - Call `List()` and verify all specs returned with correct status

   b. `List` with non-existent directory — should return empty, no error

   c. `List` with non-`.md` files — should skip them

   d. `List` with empty directory — should return empty list

   e. `Summary` — create specs with each status (draft, approved, prompted, verifying, completed):
      - Verify each counter in `Summary` struct is correct
      - Verify `LinkedPromptsCompleted` and `LinkedPromptsTotal` are populated

   f. `Summary` with empty directory — all counters zero

2. In `pkg/report/report_test.go`, add test for `ChangelogSuffix()`:
   - Call `ChangelogSuffix()` and verify it returns expected suffix string
   - Follow the existing pattern used for `Suffix()` and `ValidationSuffix()`
</requirements>

<constraints>
- Follow existing test patterns in each file (Ginkgo Describe/Context/It)
- Use temp directories for filesystem tests (cleaned up by GinkgoT().TempDir())
- Do NOT modify production code
- Do NOT commit — dark-factory handles git
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Check coverage:
```bash
go test -cover ./pkg/spec/... ./pkg/report/...
# Expected: coverage increase for both packages
```

Run new tests:
```bash
go test -v ./pkg/spec/... -run "Lister"
go test -v ./pkg/report/... -run "ChangelogSuffix"
```
</verification>

<success_criteria>
- `Lister.List()` tested with 4+ scenarios
- `Lister.Summary()` tested with status tallying verification
- `ChangelogSuffix()` tested
- All new tests pass
- `make precommit` passes
</success_criteria>
