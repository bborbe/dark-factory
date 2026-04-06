---
status: approved
created: "2026-04-06T00:00:00Z"
queued: "2026-04-06T17:05:26Z"
---

<summary>
- Two test suite files are missing the standard time zone and diff format setup
- Without the time zone override, tests can produce timezone-dependent failures
- Without full diff output, assertion failures show truncated diffs that hide the root cause
- Every other suite in the project already includes these two setup lines
- The fix adds the missing setup lines to bring these suites in line with the project standard
</summary>

<objective>
Add the standard time zone and diff format setup to the suite runner functions in `pkg/globalconfig/globalconfig_suite_test.go` and `pkg/reindex/reindex_suite_test.go`, matching the pattern used in every other suite file.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read and update:
- `pkg/globalconfig/globalconfig_suite_test.go`
- `pkg/reindex/reindex_suite_test.go`
</context>

<requirements>
1. In `pkg/globalconfig/globalconfig_suite_test.go`, add `time.Local = time.UTC` and `format.TruncatedDiff = false` before the `RegisterFailHandler` call. Add imports: `"time"`, `"github.com/onsi/gomega/format"`. Do NOT add GinkgoConfiguration/Timeout — a separate prompt handles that for all suites.

2. In `pkg/reindex/reindex_suite_test.go`, apply the same pattern: add `time.Local = time.UTC` and `format.TruncatedDiff = false` before `RegisterFailHandler`. Add imports: `"time"`, `"github.com/onsi/gomega/format"`. Do NOT add GinkgoConfiguration/Timeout.

3. Verify `//go:generate go run -mod=mod github.com/maxbrunsfeld/counterfeiter/v6 -generate` is present in both files (add if missing).
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Only modify the suite runner function and its imports
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
