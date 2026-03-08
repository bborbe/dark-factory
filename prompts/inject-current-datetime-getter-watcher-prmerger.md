---
status: created
created: "2026-03-08T20:54:02Z"
---

<objective>
Inject `libtime.CurrentDateTimeGetter` into `watcher` and `prMerger` structs, replacing direct `time.Now()` calls. This completes the time injection migration started by the previous prompt (which migrated `PromptFile` and `SpecFile`). After this prompt, no production code calls `time.Now()` directly.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/watcher/watcher.go` — `watcher` struct (line 30-36), `NewWatcher` constructor (line 38-53), `stampCreatedTimestamps` method which calls `time.Now()` directly (line 189).
Read `pkg/watcher/watcher_test.go` — existing tests for watcher.
Read `pkg/git/pr_merger.go` — `prMerger` struct (line 25-29), `NewPRMerger` constructor (line 32-38), `WaitAndMerge` which calls `time.Now()` on lines 47 and 56.
Read `pkg/git/git_test.go` — existing tests for PRMerger if any.
Read `pkg/factory/factory.go` — `CreateWatcher` (line 153+) and `NewPRMerger` calls (lines 201, 234) to update factory wiring.
Read `/home/node/.claude/docs/go-patterns.md` and `/home/node/.claude/docs/go-testing.md`.

The previous prompt already added `github.com/bborbe/time` as a direct dependency and established the pattern:
```go
import libtime "github.com/bborbe/time"

// Accept in constructor:
currentDateTimeGetter libtime.CurrentDateTimeGetter

// Use:
now := time.Time(s.currentDateTimeGetter.Now())

// In tests:
currentDateTime := libtime.NewCurrentDateTime()
currentDateTime.SetNow(libtime.DateTime(fixedTime))
```
</context>

<requirements>
1. In `pkg/watcher/watcher.go`:

   a. Add import: `libtime "github.com/bborbe/time"`

   b. Add field to `watcher` struct:
   ```go
   type watcher struct {
       inProgressDir         string
       inboxDir              string
       promptManager         prompt.Manager
       ready                 chan<- struct{}
       debounce              time.Duration
       currentDateTimeGetter libtime.CurrentDateTimeGetter
   }
   ```

   c. Update `NewWatcher` constructor to accept the getter:
   ```go
   func NewWatcher(
       inProgressDir string,
       inboxDir string,
       promptManager prompt.Manager,
       ready chan<- struct{},
       debounce time.Duration,
       currentDateTimeGetter libtime.CurrentDateTimeGetter,
   ) Watcher {
       return &watcher{
           inProgressDir:         inProgressDir,
           inboxDir:              inboxDir,
           promptManager:         promptManager,
           ready:                 ready,
           debounce:              debounce,
           currentDateTimeGetter: currentDateTimeGetter,
       }
   }
   ```

   d. Replace `time.Now()` in `stampCreatedTimestamps` (line 189):
   ```go
   // Before:
   pf.Frontmatter.Created = time.Now().UTC().Format(time.RFC3339)

   // After:
   pf.Frontmatter.Created = time.Time(w.currentDateTimeGetter.Now()).UTC().Format(time.RFC3339)
   ```

2. In `pkg/git/pr_merger.go`:

   a. Add import: `libtime "github.com/bborbe/time"`

   b. Add field to `prMerger` struct:
   ```go
   type prMerger struct {
       ghToken               string
       pollInterval          time.Duration
       mergeTimeout          time.Duration
       currentDateTimeGetter libtime.CurrentDateTimeGetter
   }
   ```

   c. Update `NewPRMerger` constructor:
   ```go
   func NewPRMerger(
       ghToken string,
       currentDateTimeGetter libtime.CurrentDateTimeGetter,
   ) PRMerger {
       return &prMerger{
           ghToken:               ghToken,
           pollInterval:          30 * time.Second,
           mergeTimeout:          30 * time.Minute,
           currentDateTimeGetter: currentDateTimeGetter,
       }
   }
   ```

   d. Replace `time.Now()` calls in `WaitAndMerge`:
   ```go
   // Line 47 — Before:
   deadline := time.Now().Add(p.mergeTimeout)
   // After:
   deadline := time.Time(p.currentDateTimeGetter.Now()).Add(p.mergeTimeout)

   // Line 56 — Before:
   if time.Now().After(deadline) {
   // After:
   if time.Time(p.currentDateTimeGetter.Now()).After(deadline) {
   ```

3. In `pkg/factory/factory.go`:

   a. Add import if not present: `libtime "github.com/bborbe/time"`

   b. Update `CreateWatcher` call (line 161) to pass `libtime.NewCurrentDateTime()`:
   ```go
   return watcher.NewWatcher(inProgressDir, inboxDir, promptManager, ready, debounce, libtime.NewCurrentDateTime())
   ```

   c. Update both `git.NewPRMerger` calls (lines 201, 234) to pass `libtime.NewCurrentDateTime()`:
   ```go
   git.NewPRMerger(ghToken, libtime.NewCurrentDateTime()),
   ```

4. Update tests in `pkg/watcher/watcher_test.go`:

   Find all `NewWatcher(...)` calls in tests and add `libtime.NewCurrentDateTime()` as the last argument. If tests need deterministic time, use:
   ```go
   currentDateTime := libtime.NewCurrentDateTime()
   currentDateTime.SetNow(libtime.DateTime(fixedTime))
   ```
   and pass `currentDateTime` to `NewWatcher`.

5. Update tests in `pkg/git/` for `NewPRMerger` if any exist — add `libtime.NewCurrentDateTime()` as second argument.

6. Verify no direct `time.Now()` calls remain in production code:
   ```bash
   grep -rn "time\.Now()" pkg/ --include="*.go" | grep -v "_test.go"
   ```
   This should return zero results.
</requirements>

<constraints>
- Do NOT change the `Watcher` or `PRMerger` interfaces — only the structs and constructors change
- Do NOT touch `pkg/prompt/` or `pkg/spec/` — those were migrated in the previous prompt
- Do NOT use counterfeiter mock for `CurrentDateTimeGetter` — use `libtime.NewCurrentDateTime()` with `SetNow()` in tests
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Verify no `time.Now()` in production code:
```bash
grep -rn "time\.Now()" pkg/ --include="*.go" | grep -v "_test.go"
# Expected: no output
```

Verify libtime imported in changed files:
```bash
grep "bborbe/time" pkg/watcher/watcher.go pkg/git/pr_merger.go pkg/factory/factory.go
# Expected: all three files show the import
```

Run targeted tests:
```bash
go test -v ./pkg/watcher/...
go test -v ./pkg/git/... -run "PRMerger|Merger"
```
</verification>

<success_criteria>
- Zero `time.Now()` calls in production code (only in test files)
- `watcher` and `prMerger` accept `CurrentDateTimeGetter` in constructors
- Factory passes `libtime.NewCurrentDateTime()` to both
- All tests pass with deterministic time where needed
- `make precommit` passes
</success_criteria>
