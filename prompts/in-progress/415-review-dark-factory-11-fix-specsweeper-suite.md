---
status: approved
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T14:51:20Z"
---

<summary>
- Fixed missing suite setup in pkg/specsweeper/sweeper_suite_test.go
- Added time.Local = time.UTC, format.TruncatedDiff = false, suiteConfig with Timeout
- Added missing //go:generate counterfeiter directive
</summary>

<objective>
Fix the specsweeper test suite file that is missing standard Ginkgo setup and the counterfeiter generate directive.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `pkg/specsweeper/sweeper_suite_test.go` — current content (missing time.Local, TruncatedDiff, Timeout, go:generate)
- Another suite file in pkg/ for reference (e.g., `pkg/prompt/prompt_suite_test.go`)
</context>

<requirements>
1. In `pkg/specsweeper/sweeper_suite_test.go`, replace the content with proper suite setup:
   ```go
   // Copyright header
   package specsweeper_test

   import (
       "testing"
       "time"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       "github.com/onsi/gomega/format"
   )

   //go:generate go run -mod=mod github.com/maxbrunsfeld/counterfeiter/v6 -generate
   func TestSuite(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       RunSpecs(t, "Specsweeper Suite", suiteConfig, reporterConfig)
   }
   ```
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
make precommit
</verification>
