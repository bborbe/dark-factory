---
status: approved
created: "2026-03-08T21:06:35Z"
queued: "2026-03-08T23:18:05Z"
---

<summary>
- Standardise test suite setup so timezone and diff output behave consistently across all packages
- Extract missing suite file for report package
- Rename mock-prefixed test variables to match project convention (no prefix)
</summary>

<objective>
Ensure all test suites have consistent setup (`time.Local`, `format.TruncatedDiff`) and test variable naming follows project conventions (no `mock` prefix).
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/report/report_test.go` — contains `TestReport()` entry point that should be in a dedicated suite file.
Read `pkg/review/review_suite_test.go` — missing `time.Local = time.UTC`.
Read `pkg/project/project_suite_test.go` — missing both `time.Local = time.UTC` and `format.TruncatedDiff = false`.
Read `pkg/spec/spec_suite_test.go` — missing both `time.Local = time.UTC` and `format.TruncatedDiff = false`.
Read `pkg/review/poller_test.go` — variables `mockFetcher`, `mockPRMerger`, `mockManager`, `mockGenerator` use `mock` prefix.
Read `pkg/status/status_test.go` — variable `mockPromptMgr` uses `mock` prefix.
Read any existing `*_suite_test.go` file (e.g., `pkg/prompt/prompt_suite_test.go`) as the reference pattern.
Read `/home/node/.claude/docs/go-testing.md`.
</context>

<requirements>
1. Create `pkg/report/report_suite_test.go`:
   ```go
   package report_test

   import (
       "testing"
       "time"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       "github.com/onsi/gomega/format"
   )

   func TestReport(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       RunSpecs(t, "Report Suite")
   }
   ```
   Then remove `TestReport()` and its imports from `pkg/report/report_test.go`.

2. In `pkg/review/review_suite_test.go`, add `time.Local = time.UTC` before `RegisterFailHandler`. Preserve the existing `format.TruncatedDiff = false` line. Add `"time"` import.

3. In `pkg/project/project_suite_test.go`, add both:
   ```go
   time.Local = time.UTC
   format.TruncatedDiff = false
   ```
   Add imports: `"time"` and `"github.com/onsi/gomega/format"`.

4. In `pkg/spec/spec_suite_test.go`, add both:
   ```go
   time.Local = time.UTC
   format.TruncatedDiff = false
   ```
   Add imports: `"time"` and `"github.com/onsi/gomega/format"`.

5. In `pkg/review/poller_test.go`, rename all mock-prefixed variables:
   - `mockFetcher` → `fetcher`
   - `mockPRMerger` → `prMerger`
   - `mockManager` → `manager`
   - `mockGenerator` → `generator`

6. In `pkg/status/status_test.go`, rename:
   - `mockPromptMgr` → `promptMgr`
</requirements>

<constraints>
- Do NOT change test logic — only rename variables and add suite setup
- Do NOT modify any production code
- Ensure the `report_suite_test.go` has the copyright header matching other suite files
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Verify no mock-prefixed variables:
```bash
grep -n "mock[A-Z]" pkg/review/poller_test.go pkg/status/status_test.go
# Expected: no output
```

Verify suite file exists:
```bash
ls pkg/report/report_suite_test.go
# Expected: file exists
```

Verify TestReport removed from report_test.go:
```bash
grep "func TestReport" pkg/report/report_test.go
# Expected: no output
```
</verification>

<success_criteria>
- `pkg/report/report_suite_test.go` exists with proper setup
- `TestReport()` removed from `report_test.go`
- All 4 suite files have `time.Local = time.UTC` and `format.TruncatedDiff = false`
- No `mock`-prefixed variable names in `poller_test.go` and `status_test.go`
- `make precommit` passes
</success_criteria>
