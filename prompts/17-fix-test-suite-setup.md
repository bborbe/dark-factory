---
status: created
---

<summary>
- All test suite files have consistent setup with `time.Local = time.UTC` and `format.TruncatedDiff = false`
- Missing `//go:generate` directives are added to suite files for counterfeiter mock generation
- Test infrastructure is uniform across all packages
</summary>

<objective>
Fix test suite setup inconsistencies: add missing `time.Local = time.UTC` and `format.TruncatedDiff = false` to `specnum_suite_test.go`, and add missing `//go:generate` directives to four suite files.
</objective>

<context>
Read CLAUDE.md for project conventions.
The standard suite file pattern used by all other packages is:
```go
//go:generate go run -mod=mod github.com/maxbrunsfeld/counterfeiter/v6 -generate

package foo_test

import (
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/onsi/gomega/format"
)

func TestFoo(t *testing.T) {
    time.Local = time.UTC
    format.TruncatedDiff = false
    RegisterFailHandler(Fail)
    RunSpecs(t, "Foo Suite")
}
```
</context>

<requirements>
1. In `pkg/specnum/specnum_suite_test.go`, add `time.Local = time.UTC` and `format.TruncatedDiff = false` before `RegisterFailHandler(Fail)`. Add the necessary imports (`"time"` and `"github.com/onsi/gomega/format"`).

2. In `pkg/specnum/specnum_suite_test.go`, add the `//go:generate` directive above the package declaration:
   ```go
   //go:generate go run -mod=mod github.com/maxbrunsfeld/counterfeiter/v6 -generate
   ```

3. In `pkg/report/report_suite_test.go`, add the `//go:generate` directive if missing.

4. In `pkg/project/project_suite_test.go`, add the `//go:generate` directive if missing.

5. In `main_test.go` (root), add the `//go:generate` directive if missing.

6. For each file, read it first to check what is already present — only add what is missing.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Place `//go:generate` on the line directly above the `package` declaration.
- The `//go:generate` directive must be exactly: `//go:generate go run -mod=mod github.com/maxbrunsfeld/counterfeiter/v6 -generate`
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
