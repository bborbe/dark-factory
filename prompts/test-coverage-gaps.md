---
status: created
created: "2026-03-08T21:06:35Z"
---

<summary>
- Add missing test for `ChangelogSuffix()` in report package
- Follow existing `ValidationSuffix` test pattern with `ContainSubstring` assertions
</summary>

<objective>
Cover the untested `ChangelogSuffix()` function in `pkg/report` to match the coverage of sibling functions `Suffix()` and `ValidationSuffix()`.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/report/suffix.go` — `ChangelogSuffix()` function at line ~48.
Read `pkg/report/report_test.go` — existing `Describe("ValidationSuffix")` block at line ~49 as the pattern to follow. No `ChangelogSuffix` test exists.
Read `/home/node/.claude/docs/go-testing.md`.
</context>

<requirements>
1. In `pkg/report/report_test.go`, add a new `Describe("ChangelogSuffix")` block after the existing `Describe("ValidationSuffix")` block. Follow the same pattern:
   ```go
   var _ = Describe("ChangelogSuffix", func() {
       It("should contain CHANGELOG.md reference", func() {
           suffix := report.ChangelogSuffix()
           Expect(suffix).To(ContainSubstring("CHANGELOG.md"))
       })

       It("should contain unreleased section instruction", func() {
           suffix := report.ChangelogSuffix()
           Expect(suffix).To(ContainSubstring("## Unreleased"))
       })

       It("should reference changelog guide", func() {
           suffix := report.ChangelogSuffix()
           Expect(suffix).To(ContainSubstring("changelog-guide.md"))
       })
   })
   ```
</requirements>

<constraints>
- Do NOT modify production code
- Follow existing test patterns in `report_test.go`
- Do NOT commit — dark-factory handles git
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Run targeted test:
```bash
go test -v ./pkg/report/... -run "ChangelogSuffix"
# Expected: 3 passing tests
```
</verification>

<success_criteria>
- `ChangelogSuffix` test exists in `report_test.go`
- Tests assert key substrings: `CHANGELOG.md`, `## Unreleased`, `changelog-guide.md`
- `make precommit` passes
</success_criteria>
