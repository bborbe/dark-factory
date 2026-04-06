---
status: approved
created: "2026-04-06T00:00:00Z"
queued: "2026-04-06T17:04:41Z"
---

<summary>
- All 26 test suite files run without a configured timeout
- Without a suite timeout, a hanging goroutine or blocked test can stall CI indefinitely
- The Ginkgo v2 API provides GinkgoConfiguration() to set a per-suite timeout
- The fix adds a 60-second timeout to every suite file using the standard pattern
- This is a purely mechanical change with no impact on test logic
</summary>

<objective>
Add `GinkgoConfiguration()` and `suiteConfig.Timeout = 60 * time.Second` to every `*_suite_test.go` file and `main_test.go` in the repository so that test suites have an enforced timeout and cannot hang indefinitely.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to update (all suite files — read each before editing):
- `main_test.go`
- All `pkg/*/` suite files: `cmd_suite_test.go`, `config_suite_test.go`, `containerlock_suite_test.go`, `executor_suite_test.go`, `factory_suite_test.go`, `generator_suite_test.go`, `git_suite_test.go`, `globalconfig_suite_test.go`, `locker_suite_test.go`, `notifier_suite_test.go`, `processor_suite_test.go`, `project_suite_test.go`, `prompt_suite_test.go`, `reindex_suite_test.go`, `report_suite_test.go`, `review_suite_test.go`, `runner_suite_test.go`, `server_suite_test.go`, `migrator_suite_test.go`, `spec_suite_test.go`, `specnum_suite_test.go`, `watcher_suite_test.go` (specwatcher), `status_suite_test.go`, `version_suite_test.go`, `watcher_suite_test.go` (watcher)
</context>

<requirements>
1. In every `*_suite_test.go` file, change the runner function from:
   ```go
   func TestXxx(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       RunSpecs(t, "Xxx Suite")
   }
   ```
   to:
   ```go
   func TestXxx(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       RunSpecs(t, "Xxx Suite", suiteConfig, reporterConfig)
   }
   ```

2. Add `"time"` to the import block if not already present.

3. `GinkgoConfiguration` is imported from `"github.com/onsi/ginkgo/v2"` — ensure this import is present (it should already be since `RegisterFailHandler` and `RunSpecs` are from the same package).

4. Apply this pattern to ALL suite files found under `pkg/` and to `main_test.go`. Use `grep -r 'RunSpecs' .` to find all files that need updating.

5. Do NOT modify any test logic, `Describe`, `It`, or `BeforeEach` blocks — only the suite runner function.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Only modify the suite runner function — do not touch test logic
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
