---
status: draft
created: "2026-04-06T00:00:00Z"
---

<summary>
- Two test suite files are missing the standard time zone and diff format setup
- Without time.Local = time.UTC tests can produce timezone-dependent failures
- Without format.TruncatedDiff = false assertion failures show truncated diffs that hide the root cause
- Every other suite in the project already includes these two setup lines
- The fix adds the missing setup lines to bring these suites in line with the project standard
</summary>

<objective>
Add `time.Local = time.UTC` and `format.TruncatedDiff = false` to the suite runner functions in `pkg/globalconfig/globalconfig_suite_test.go` and `pkg/reindex/reindex_suite_test.go`.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read and update:
- `pkg/globalconfig/globalconfig_suite_test.go`
- `pkg/reindex/reindex_suite_test.go`
</context>

<requirements>
1. In `pkg/globalconfig/globalconfig_suite_test.go`, update the runner function to:
   ```go
   func TestGlobalconfig(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       RunSpecs(t, "Globalconfig Suite", suiteConfig, reporterConfig)
   }
   ```
   Add imports: `"time"`, `"github.com/onsi/gomega/format"`.

2. In `pkg/reindex/reindex_suite_test.go`, apply the same pattern:
   ```go
   func TestReindex(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       RunSpecs(t, "Reindex Suite", suiteConfig, reporterConfig)
   }
   ```
   Add imports: `"time"`, `"github.com/onsi/gomega/format"`.

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
