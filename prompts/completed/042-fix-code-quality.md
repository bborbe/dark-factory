---
status: completed
container: dark-factory-042-fix-code-quality
dark-factory-version: v0.10.2
---





# Fix code quality issues from code review

## Tasks

### 1. Wrap bare errors

Add `errors.Wrap(ctx, err, "description")` at these locations:
- `pkg/executor/executor.go:60` — `"prepare log file"`
- `pkg/executor/executor.go:67` — `"create prompt temp file"`
- `pkg/cmd/queue.go:69,92` — `"move to queue"`
- `pkg/server/queue_helpers.go:30,35,62` — appropriate context strings
- `pkg/lock/locker.go:128` — `"write pid"` (add `ctx` param to `writePID` if needed)
- `pkg/prompt/prompt.go:320` — `"set field"`

Use `github.com/bborbe/errors` package (already imported in most files).

### 2. Fix YAML unmarshal defaults bug

`pkg/config/loader.go:51` — `yaml.Unmarshal` into pre-populated defaults struct zeroes missing numeric/bool fields.

Fix: Unmarshal into a separate struct, then merge non-zero values onto defaults. Or use a pointer-based approach where `nil` means "use default".

### 3. Add missing suite setup

Add `time.Local = time.UTC` and `format.TruncatedDiff = false` to these suite files:
- `main_test.go`
- `pkg/lock/locker_suite_test.go`
- `pkg/version/version_suite_test.go`
- `pkg/status/status_suite_test.go`
- `pkg/server/server_suite_test.go`
- `pkg/cmd/cmd_suite_test.go`

Import `"time"` and `"github.com/onsi/gomega/format"` as needed.

### 4. Add `//go:generate` directive to `cmd_suite_test.go`

Add `//go:generate go run -mod=mod github.com/maxbrunsfeld/counterfeiter/v6 -generate` before `TestCmd`.

### 5. Fix counterfeiter directive placement

Move `//counterfeiter:generate` directives ABOVE the GoDoc comment (separated by blank line) in:
- `pkg/server/server.go` (`PromptManager` interface)
- `pkg/cmd/status.go` (`StatusCommand` interface)
- `pkg/cmd/queue.go` (`QueueCommand` interface)

### 6. Add GoDoc to `Locker` interface

`pkg/lock/locker.go:22` — add `// Locker provides exclusive access control to prevent concurrent dark-factory instances.`

### 7. Extract `"prompts/ideas"` constant

`pkg/factory/factory.go` — extract duplicated `"prompts/ideas"` to `const defaultIdeasDir = "prompts/ideas"` or add `IdeasDir` field to `config.Config`.

## Verification

Run `make precommit` — must pass.
