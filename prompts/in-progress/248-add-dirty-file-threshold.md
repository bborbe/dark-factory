---
status: executing
container: dark-factory-248-add-dirty-file-threshold
dark-factory-version: v0.95.0
created: "2026-04-04T16:00:00Z"
queued: "2026-04-04T21:13:56Z"
started: "2026-04-04T21:13:57Z"
---

<summary>
- Projects can configure a dirty-file threshold that prevents prompt execution when the git working tree has too many uncommitted changes
- When the threshold is exceeded, the prompt is skipped (not failed) and re-checked on the next poll cycle
- The check runs on the host before any container is started, so it completes in seconds even on large repos
- Default is 0 (disabled) -- only repos that opt in are affected
- Existing behavior is completely unchanged for projects without this setting
- No auto-cleanup -- users must manually resolve dirty files before prompts resume
</summary>

<objective>
Add a `dirtyFileThreshold` config field that prevents starting prompt containers when the git working tree has too many dirty files. This solves the problem where large repos (500K+ files in index) with dirty vendor dirs cause `git status` inside the container to take 10+ minutes. The check runs on the host (fast) and skips the prompt without marking it failed, so it retries automatically on the next poll cycle.
</objective>

<context>
Read CLAUDE.md for project conventions.

Key files to read before making changes:
- `pkg/config/config.go` -- `Config` struct (add field here), `Defaults()`, `Validate()`. Follow the pattern of existing fields like `MaxContainers` for validation.
- `pkg/config/config_test.go` -- existing config validation tests (Ginkgo/Gomega pattern)
- `pkg/processor/processor.go` -- `processor` struct, `NewProcessor()` constructor, `processPrompt()` method (the dirty check goes here, before container start), `processExistingQueued()` loop (calls `processPrompt`), `shouldSkipPrompt()` (existing skip pattern to study but NOT reuse -- dirty threshold is a different kind of skip)
- `pkg/processor/processor_test.go` -- existing processor tests
- `pkg/factory/factory.go` -- `createProcessor()` function (wire the new field through)
- `docs/configuration.md` -- document the new field

Reference coding guidelines:
- Error wrapping: `github.com/bborbe/errors` (never `fmt.Errorf`)
- Testing: Ginkgo v2 / Gomega
- Architecture: Interface -> Constructor -> Struct -> Method pattern
</context>

<requirements>
1. **Add `DirtyFileThreshold` to `Config` in `pkg/config/config.go`**:

   Add a new field to the `Config` struct:
   ```go
   DirtyFileThreshold int `yaml:"dirtyFileThreshold,omitempty"`
   ```

   Place it near `MaxContainers` (both are operational limits).

   Do NOT set a default in `Defaults()` -- the zero value (0) means disabled.

   Add validation in `Validate()` -- add a `validation.Name("dirtyFileThreshold", ...)` entry that rejects negative values:
   ```go
   validation.Name("dirtyFileThreshold", validation.HasValidationFunc(func(ctx context.Context) error {
       if c.DirtyFileThreshold < 0 {
           return errors.Errorf(ctx, "dirtyFileThreshold must not be negative, got %d", c.DirtyFileThreshold)
       }
       return nil
   })),
   ```

2. **Add config tests in `pkg/config/config_test.go`**:

   Add Ginkgo test cases for:
   - `dirtyFileThreshold: 0` -- valid (disabled)
   - `dirtyFileThreshold: 10` -- valid
   - `dirtyFileThreshold: -1` -- invalid, validation error

   Follow the existing test patterns in that file.

3. **Create a `DirtyFileChecker` in `pkg/processor/dirty.go`**:

   Create a new file with a small interface and implementation:

   ```go
   package processor

   import (
       "context"
       "os/exec"
       "strconv"
       "strings"

       "github.com/bborbe/errors"
   )

   //counterfeiter:generate -o ../../mocks/dirty-file-checker.go --fake-name DirtyFileChecker . DirtyFileChecker

   // DirtyFileChecker counts dirty files in a git working tree.
   type DirtyFileChecker interface {
       CountDirtyFiles(ctx context.Context) (int, error)
   }

   // NewDirtyFileChecker creates a DirtyFileChecker that runs git status on the host.
   func NewDirtyFileChecker(repoDir string) DirtyFileChecker {
       return &gitDirtyFileChecker{repoDir: repoDir}
   }

   type gitDirtyFileChecker struct {
       repoDir string
   }

   func (c *gitDirtyFileChecker) CountDirtyFiles(ctx context.Context) (int, error) {
       cmd := exec.CommandContext(ctx, "git", "status", "--short")
       cmd.Dir = c.repoDir
       output, err := cmd.Output()
       if err != nil {
           return 0, errors.Wrap(ctx, err, "git status --short")
       }
       trimmed := strings.TrimSpace(string(output))
       if trimmed == "" {
           return 0, nil
       }
       return len(strings.Split(trimmed, "\n")), nil
   }
   ```

4. **Add dirty file check to `processPrompt()` in `pkg/processor/processor.go`**:

   Add `dirtyFileThreshold int` and `dirtyFileChecker DirtyFileChecker` fields to the `processor` struct.

   Add corresponding parameters to `NewProcessor()` and wire them in the constructor.

   In `processPrompt()`, add the dirty file check as the FIRST thing, before the git fetch/merge and before `prepareContainerSlot()`:

   ```go
   func (p *processor) processPrompt(ctx context.Context, pr prompt.Prompt) error {
       // Check dirty file threshold before starting any work
       if skip, err := p.checkDirtyFileThreshold(ctx); err != nil {
           return errors.Wrap(ctx, err, "check dirty file threshold")
       } else if skip {
           return nil // skip this cycle, re-check on next poll
       }

       // Sync with remote before execution (existing code continues here)
       ...
   ```

   Add the helper method:
   ```go
   // checkDirtyFileThreshold returns (true, nil) when the prompt should be skipped
   // because the working tree has too many dirty files. Returns (false, nil) when
   // the check is disabled or the count is within threshold.
   func (p *processor) checkDirtyFileThreshold(ctx context.Context) (bool, error) {
       if p.dirtyFileThreshold <= 0 {
           return false, nil
       }
       count, err := p.dirtyFileChecker.CountDirtyFiles(ctx)
       if err != nil {
           return false, err
       }
       if count > p.dirtyFileThreshold {
           slog.Warn(
               "dirty file threshold exceeded, skipping prompt",
               "dirtyFiles", count,
               "threshold", p.dirtyFileThreshold,
           )
           return true, nil
       }
       return false, nil
   }
   ```

   IMPORTANT: When `checkDirtyFileThreshold` returns `(true, nil)`, `processPrompt` returns `nil` (not an error). This means the prompt is NOT marked as failed -- it stays queued and will be re-checked on the next poll cycle (5-second ticker in `Process()`).

5. **Wire through factory in `pkg/factory/factory.go`**:

   Add `dirtyFileThreshold int` and `dirtyFileChecker processor.DirtyFileChecker` parameters to `CreateProcessor()` (line 467). Pass them through to `processor.NewProcessor()`.

   Create the `DirtyFileChecker` in the factory before calling `CreateProcessor`:
   ```go
   dirtyFileChecker := processor.NewDirtyFileChecker(".")
   ```

   Always create the checker (the processor short-circuits on threshold <= 0, so nil-checking is unnecessary).

   `CreateProcessor` is called from two locations — update both:
   - `CreateRunner` (line 264): pass `cfg.DirtyFileThreshold, dirtyFileChecker`
   - `CreateOneShotRunner` (line 331): pass `cfg.DirtyFileThreshold, dirtyFileChecker`

6. **Generate the counterfeiter fake**:

   Run `go generate ./pkg/processor/...` to create the fake in `mocks/dirty-file-checker.go`.

7. **Add processor tests in `pkg/processor/processor_test.go` or `pkg/processor/processor_internal_test.go`**:

   Test the `checkDirtyFileThreshold` method using the counterfeiter fake:
   - Threshold 0 (disabled) -- returns `(false, nil)` without calling checker
   - Threshold 10, dirty count 5 -- returns `(false, nil)` (under threshold)
   - Threshold 10, dirty count 10 -- returns `(false, nil)` (at threshold, not exceeded)
   - Threshold 10, dirty count 11 -- returns `(true, nil)` (over threshold, skip)
   - Threshold 10, checker returns error -- returns `(false, err)`

   Use Ginkgo/Gomega. Follow existing test patterns in the file.

8. **Add unit tests for `gitDirtyFileChecker` in `pkg/processor/dirty_test.go`**:

   Use a temp directory initialized as a git repo:
   - Empty repo -- returns 0
   - Create untracked files -- returns correct count
   - Modified tracked files -- returns correct count

   Use Ginkgo/Gomega with `GinkgoT()` and `os.MkdirTemp`.

9. **Update `docs/configuration.md`**:

   Add a section for `dirtyFileThreshold` near the `maxContainers` documentation:

   ```markdown
   ### Dirty File Threshold

   Skip prompt execution when the git working tree has too many dirty (uncommitted) files.
   Useful for large repos where dirty vendor directories cause slow `git status` inside containers.

   ```yaml
   dirtyFileThreshold: 50
   ```

   - `0` (default): disabled -- no check performed
   - When exceeded: prompt is skipped (not failed) and re-checked on next poll cycle
   - User must clean up dirty files manually -- no auto-cleanup
   ```

10. **Update `CHANGELOG.md`**:

    Add an entry under the Unreleased section:
    ```
    - Add `dirtyFileThreshold` config to skip prompts when git working tree has too many dirty files
    ```
</requirements>

<constraints>
- Do NOT commit -- dark-factory handles git
- Do NOT mark the prompt as failed when threshold is exceeded -- just skip it (return nil from processPrompt)
- Do NOT auto-clean dirty files
- Do NOT change existing tests -- they must still pass
- Default 0 = disabled -- no behavior change for existing projects
- Use `github.com/bborbe/errors` for error wrapping (never `fmt.Errorf`)
- Use Ginkgo v2 / Gomega for all tests
- The dirty file check must run BEFORE `prepareContainerSlot()` and BEFORE git fetch/merge -- it's a pre-flight check
- Use `git status --short` (not `--porcelain`) and count output lines
- The `DirtyFileChecker` must be an interface with a counterfeiter fake for testability
</constraints>

<verification>
Run `make precommit` -- must pass with exit code 0.

Verify manually:
```bash
# In a repo with dirtyFileThreshold configured:
# 1. Create files exceeding threshold
# 2. Run dark-factory run -- should log skip message and exit cleanly
# 3. Remove extra files
# 4. Run dark-factory run -- should proceed normally
```
</verification>
