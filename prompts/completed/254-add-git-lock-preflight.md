---
status: completed
summary: 'Added git index lock preflight check: daemon startup aborts on .git/index.lock, prompt and spec-generation cycles skip and retry, with GitLockChecker interface, tests, and mock'
container: dark-factory-254-add-git-lock-preflight
dark-factory-version: v0.101.0
created: "2026-04-04T15:30:00Z"
queued: "2026-04-05T22:09:22Z"
started: "2026-04-05T22:11:46Z"
completed: "2026-04-05T22:37:14Z"
---

<summary>
- Daemon startup aborts with a clear error if `.git/index.lock` exists, preventing a doomed run
- Each prompt execution checks for the lock file before starting, skipping the prompt and retrying on the next poll cycle
- Each spec generation checks for the lock file before launching a container, skipping and retrying on the next poll cycle
- The check is always active with no configuration needed
- Successful prompts can no longer appear as failed due to `git add` exit 128 from a stale lock file
</summary>

<objective>
Add a git index lock preflight check (`os.Stat(".git/index.lock")`) to three locations: daemon startup (hard error), before prompt execution (skip and retry), and before spec generation (skip and retry). This prevents `git add` exit 128 failures that make successful prompts appear failed.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before making any changes:
- `pkg/processor/processor.go` — `checkDirtyFileThreshold` method (~line 696) for the skip pattern, `processPrompt` method (~line 728) for where to add the check
- `pkg/processor/dirty.go` — `DirtyFileChecker` interface and `NewDirtyFileChecker` for structural reference
- `pkg/runner/runner.go` — `Run` method (~line 114) for daemon startup sequence
- `pkg/generator/generator.go` — `Generate` method (~line 84) for spec generation entry point
- `pkg/specwatcher/watcher.go` — `handleFileEvent` method (~line 132) for how spec generation is triggered
- `pkg/factory/factory.go` — `CreateRunner` (~line 216) and `CreateProcessor` (~line 490) for wiring
</context>

<requirements>
**Step 1 — Create `pkg/processor/gitlock.go`**

Create a new file with a `GitLockChecker` interface and implementation, following the same pattern as `dirty.go`:

```go
package processor

import (
	"os"
	"path/filepath"
)

//counterfeiter:generate -o ../../mocks/git-lock-checker.go --fake-name GitLockChecker . GitLockChecker

// GitLockChecker checks whether .git/index.lock exists in the working tree.
type GitLockChecker interface {
	Exists() bool
}

// NewGitLockChecker creates a GitLockChecker for the given repo directory.
func NewGitLockChecker(repoDir string) GitLockChecker {
	return &gitLockChecker{repoDir: repoDir}
}

type gitLockChecker struct {
	repoDir string
}

func (c *gitLockChecker) Exists() bool {
	_, err := os.Stat(filepath.Join(c.repoDir, ".git", "index.lock"))
	return err == nil
}
```

**Step 2 — Add `checkGitIndexLock` to the processor**

In `pkg/processor/processor.go`:

2a. Add a `gitLockChecker GitLockChecker` field to the `processor` struct (next to `dirtyFileChecker`).

2b. Add a `gitLockChecker GitLockChecker` parameter to `NewProcessor` (after `dirtyFileChecker`), and wire it in the constructor body.

2c. Add a new method on `processor`, placed right after `checkDirtyFileThreshold`:

```go
// checkGitIndexLock returns (true, nil) when the prompt should be skipped
// because .git/index.lock exists. Returns (false, nil) when no lock file is present.
func (p *processor) checkGitIndexLock(ctx context.Context) (bool, error) {
	if p.gitLockChecker.Exists() {
		slog.Warn("git index lock exists, skipping prompt — will retry next cycle")
		return true, nil
	}
	return false, nil
}
```

2d. In `processPrompt`, add the lock check **before** the existing `checkDirtyFileThreshold` call:

Old pattern at the top of `processPrompt`:
```go
func (p *processor) processPrompt(ctx context.Context, pr prompt.Prompt) error {
	if skip, err := p.checkDirtyFileThreshold(ctx); err != nil {
```

New pattern:
```go
func (p *processor) processPrompt(ctx context.Context, pr prompt.Prompt) error {
	if skip, err := p.checkGitIndexLock(ctx); err != nil {
		return errors.Wrap(ctx, err, "check git index lock")
	} else if skip {
		return nil // skip this cycle, re-check on next poll
	}

	if skip, err := p.checkDirtyFileThreshold(ctx); err != nil {
```

**Step 3 — Add startup check in `pkg/runner/runner.go`**

In the `Run` method, add a check **after** acquiring the instance lock and **before** signal setup. This is a hard error (abort), not a skip:

Old:
```go
	slog.Info("acquired lock", "file", ".dark-factory.lock")

	// Set up signal handling
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
```

New:
```go
	slog.Info("acquired lock", "file", ".dark-factory.lock")

	// Abort if .git/index.lock exists — all git operations will fail
	if _, err := os.Stat(filepath.Join(".", ".git", "index.lock")); err == nil {
		return errors.Errorf(ctx, ".git/index.lock exists — remove it before starting the daemon (another git process may be running)")
	}

	// Set up signal handling
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
```

Add `"os"` and `"path/filepath"` to the import block if not already present.

**Step 4 — Add lock check before spec generation**

In `pkg/generator/generator.go`, at the top of the `Generate` method, add a lock check before any work is done. The generator does not have access to the processor's `GitLockChecker`, so use a direct `os.Stat` call (same as the runner startup check), but return `nil` (skip) instead of an error:

Old:
```go
func (g *dockerSpecGenerator) Generate(ctx context.Context, specPath string) error {
	// a. Build prompt content
	promptContent := g.buildPromptContent(specPath)
```

New:
```go
func (g *dockerSpecGenerator) Generate(ctx context.Context, specPath string) error {
	// Skip if .git/index.lock exists — will retry on next poll cycle
	if _, err := os.Stat(filepath.Join(".", ".git", "index.lock")); err == nil {
		slog.Warn("git index lock exists, skipping spec generation — will retry next cycle", "spec", filepath.Base(specPath))
		return nil
	}

	// a. Build prompt content
	promptContent := g.buildPromptContent(specPath)
```

Ensure `"path/filepath"` is in the import block (it already is). Ensure `"os"` is in the import block.

**Step 5 — Wire the checker in the factory**

In `pkg/factory/factory.go`:

5a. In `CreateRunner` (~line 216), create the checker alongside the existing `dirtyFileChecker`:

Old:
```go
	dirtyFileChecker := processor.NewDirtyFileChecker(".")
```

New:
```go
	dirtyFileChecker := processor.NewDirtyFileChecker(".")
	gitLockChecker := processor.NewGitLockChecker(".")
```

5b. Pass `gitLockChecker` to `CreateProcessor` as a new argument after `dirtyFileChecker`.

5c. Do the same in `CreateOneShotRunner` (the `run` command path) — find the `dirtyFileChecker` creation there and add `gitLockChecker` the same way.

5d. In the `CreateProcessor` function signature and body, add the `gitLockChecker processor.GitLockChecker` parameter after `dirtyFileChecker processor.DirtyFileChecker` and pass it through to `NewProcessor`.

**Step 6 — Tests**

6a. Create `pkg/processor/gitlock_test.go` following the same pattern as `pkg/processor/dirty_test.go`:
- Test `Exists()` returns `false` when no lock file exists (create a temp git repo)
- Test `Exists()` returns `true` when `.git/index.lock` is present (create the file)

6b. In `pkg/processor/processor_internal_test.go`, add a `fakeGitLockChecker` test stub following the same pattern as `fakeDirtyFileChecker`:

```go
type fakeGitLockChecker struct {
	exists bool
}

func (f *fakeGitLockChecker) Exists() bool {
	return f.exists
}
```

Update any existing test helpers that construct a `processor` to include the new field. Search for all calls to `NewProcessor` in test files and add the new parameter.

6c. Add a test in `pkg/processor/processor_internal_test.go` for `checkGitIndexLock`:
- When `Exists()` returns `false` → returns `(false, nil)`
- When `Exists()` returns `true` → returns `(true, nil)`

6d. In `pkg/runner/runner_test.go` (or `runner_internal_test.go` if it exists), add a test for the startup check:
- Create a temp dir with `.git/index.lock` present
- Verify `Run` returns an error containing `.git/index.lock`

**Step 7 — Generate counterfeiter mock**

Run `go generate ./pkg/processor/...` to generate the counterfeiter mock for `GitLockChecker`. If counterfeiter is not available, create the mock manually in `mocks/git-lock-checker.go` following the pattern of `mocks/dirty-file-checker.go`.

Update any mock usage in `pkg/processor/processor_test.go` — search for `mocks.DirtyFileChecker` usage patterns and add `GitLockChecker` mock initialization alongside them.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- No configuration field needed — this check is always active
- Use `errors.Wrap(ctx, err, "message")` or `errors.Errorf(ctx, "message")` — never `fmt.Errorf`
- Follow existing test patterns: Ginkgo v2/Gomega, external test packages for `_test.go`, internal for `_internal_test.go`
- The lock file path is always relative to `"."` (the project working directory), same as `NewDirtyFileChecker(".")`
- The `checkGitIndexLock` method signature must match `checkDirtyFileThreshold`: return `(bool, error)`
- For spec generation, return `nil` (not an error) to allow retry on next cycle — the specwatcher treats errors as generation failures and resets spec status
</constraints>

<verification>
```bash
# Verify the new file exists
test -f pkg/processor/gitlock.go && echo "OK" || echo "MISSING"

# Verify the interface is defined
grep -n 'type GitLockChecker interface' pkg/processor/gitlock.go

# Verify checkGitIndexLock exists on processor
grep -n 'func (p \*processor) checkGitIndexLock' pkg/processor/processor.go

# Verify it's called before checkDirtyFileThreshold in processPrompt
grep -A2 'checkGitIndexLock' pkg/processor/processor.go

# Verify startup check in runner
grep -n 'index.lock' pkg/runner/runner.go

# Verify spec generation check
grep -n 'index.lock' pkg/generator/generator.go

# Verify factory wiring
grep -n 'GitLockChecker\|gitLockChecker' pkg/factory/factory.go

# Run full precommit
make precommit
```
All must pass with no errors.
</verification>
