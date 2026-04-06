---
status: draft
created: "2026-04-06T00:00:00Z"
---

<summary>
- Three structs call time.Now() directly instead of using an injected time getter
- Direct time.Now() calls make the logic untestable without real time passing
- The project uses github.com/bborbe/time's CurrentDateTimeGetter for injectable time
- Affected structs are bitbucketPRMerger, dockerContainerChecker, and the status checker
- Each struct needs a currentDateTimeGetter field added and injected through its constructor
</summary>

<objective>
Inject `libtime.CurrentDateTimeGetter` into `bitbucketPRMerger` (`pkg/git/bitbucket_pr_merger.go`), `dockerContainerChecker` (`pkg/executor/checker.go`), and the status `checker` (`pkg/status/status.go`), replacing all direct `time.Now()` calls with calls to the injected getter. Update each constructor and the factory wiring accordingly.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read the coding plugin's `go-time-injection.md` guide for the time injection pattern.

Files to read before making changes (read ALL first):
- `pkg/git/bitbucket_pr_merger.go` — `time.Now()` at ~lines 56 and 65 computing merge deadline
- `pkg/executor/checker.go` — `time.Now()` at ~lines 41 and 50 in `WaitUntilRunning`
- `pkg/status/status.go` — `time.Since(executing.StartedTime)` at ~line 384 (implicitly calls time.Now())
- `pkg/factory/factory.go` — find where `NewBitbucketPRMerger`, `NewDockerContainerChecker`, and `status.NewChecker` are called to add the `currentDateTimeGetter` argument
</context>

<requirements>
1. In `pkg/git/bitbucket_pr_merger.go`:
   a. Add `currentDateTimeGetter libtime.CurrentDateTimeGetter` field to `bitbucketPRMerger` struct.
   b. Add `currentDateTimeGetter libtime.CurrentDateTimeGetter` parameter to `NewBitbucketPRMerger(...)` constructor.
   c. Replace `deadline := time.Now().Add(b.mergeTimeout)` with `deadline := time.Time(b.currentDateTimeGetter.Now()).Add(b.mergeTimeout)`.
   d. Replace `time.Now().After(deadline)` with `time.Time(b.currentDateTimeGetter.Now()).After(deadline)`.
   e. Add import `libtime "github.com/bborbe/time"`.

2. In `pkg/executor/checker.go`:
   a. Add `currentDateTimeGetter libtime.CurrentDateTimeGetter` field to `dockerContainerChecker` struct.
   b. Add `currentDateTimeGetter libtime.CurrentDateTimeGetter` parameter to `NewDockerContainerChecker(...)`.
   c. In `WaitUntilRunning`, replace `deadline := time.Now().Add(timeout)` with `deadline := time.Time(c.currentDateTimeGetter.Now()).Add(timeout)`.
   d. Replace `time.Now().After(deadline)` with `time.Time(c.currentDateTimeGetter.Now()).After(deadline)`.
   e. Add import `libtime "github.com/bborbe/time"`.

3. In `pkg/status/status.go`:
   a. Confirm whether `checker` struct already has a `currentDateTimeGetter` field; if not, add `currentDateTimeGetter libtime.CurrentDateTimeGetter` and update `NewChecker(...)`.
   b. Replace `time.Since(executing.StartedTime)` with `time.Time(c.currentDateTimeGetter.Now()).Sub(time.Time(executing.StartedTime))` (the field type will be changed in a separate prompt; use `time.Time(...)` conversion as needed to make it compile).
   e. Add import `libtime "github.com/bborbe/time"` if not present.

4. In `pkg/factory/factory.go`:
   - Locate the `currentDateTimeGetter` variable (already used elsewhere in the factory) and pass it to the updated constructors: `NewBitbucketPRMerger(ctx, ..., currentDateTimeGetter)`, `NewDockerContainerChecker(..., currentDateTimeGetter)`, and `status.NewChecker(..., currentDateTimeGetter)`.

5. Update tests that call these constructors directly to pass `libtime.NewCurrentDateTimeGetterMock()` or equivalent.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `libtime.CurrentDateTimeGetter` from `github.com/bborbe/time` — never `time.Now()` in business logic
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
