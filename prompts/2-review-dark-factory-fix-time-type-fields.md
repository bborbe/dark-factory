---
status: draft
created: "2026-04-06T00:00:00Z"
---

<summary>
- Several structs and local variables use time.Time where libtime.DateTime should be used
- The project standard is to use libtime.DateTime for all timestamp storage
- Affected types are in the status package, processor package, and reindex package
- Replacing time.Time with libtime.DateTime makes types consistent and compatible with libtime utilities
- Callers that do arithmetic on these fields may need short type conversion expressions
</summary>

<objective>
Replace `time.Time` with `libtime.DateTime` for all timestamp-holding struct fields and relevant function return types in `pkg/status/status.go`, `pkg/processor/processor.go`, and `pkg/reindex/reindex.go`. Use `time.Time(dt)` conversions where `time.Time` arithmetic methods are needed.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read the coding plugin's `go-time-injection.md` guide for the DateTime pattern.

Files to read before making changes (read ALL first):
- `pkg/status/status.go` — `CompletedAt time.Time` in `CompletedPrompt` struct (~line 59); `completedTime time.Time` in `promptWithTime` (~line 226); `getCompletionTime` return type `time.Time` (~line 254); `StartedTime time.Time` in `executingPrompt` struct (~line 304); `latestTime time.Time` local variable (~line 436)
- `pkg/processor/processor.go` — `skippedPrompts map[string]time.Time` field (~line 147); `make(map[string]time.Time)` (~lines 107 and 193)
- `pkg/reindex/reindex.go` — `parseCreated` function returning `(time.Time, bool)` (~line 235); zero returns `time.Time{}` at ~lines 237 and 245
</context>

<requirements>
1. In `pkg/status/status.go`:
   a. Change `CompletedAt time.Time` in `CompletedPrompt` to `CompletedAt libtime.DateTime`.
   b. Change `completedTime time.Time` in `promptWithTime` to `completedTime libtime.DateTime`.
   c. Change `getCompletionTime` return type from `time.Time` to `libtime.DateTime`; update `return time.Time{}` to `return libtime.DateTime{}`.
   d. Change `StartedTime time.Time` in `executingPrompt` to `StartedTime libtime.DateTime`.
   e. Change `var latestTime time.Time` to `var latestTime libtime.DateTime`.
   f. Update any sorting, comparison, or arithmetic that uses these fields: use `time.Time(field).Before(...)`, `time.Time(field).After(...)`, `time.Time(field).Sub(...)` for arithmetic where needed, or cast to `time.Time` for comparison.
   g. Add import `libtime "github.com/bborbe/time"`.

2. In `pkg/processor/processor.go`:
   a. Change `skippedPrompts map[string]time.Time` field to `skippedPrompts map[string]libtime.DateTime`.
   b. Change `make(map[string]time.Time)` to `make(map[string]libtime.DateTime)` in all locations.
   c. Update any code that reads from or writes to `skippedPrompts` to use `libtime.DateTime` — for assignments use `libtime.DateTime(time.Now())` or, preferably, use `currentDateTimeGetter.Now()` if available in scope.
   d. Add import `libtime "github.com/bborbe/time"`.

3. In `pkg/reindex/reindex.go`:
   a. Change `parseCreated` signature from `func parseCreated(...) (time.Time, bool)` to `func parseCreated(...) (libtime.DateTime, bool)`.
   b. Change `return time.Time{}, false` to `return libtime.DateTime{}, false` (both occurrences).
   c. Update callers of `parseCreated` to use `libtime.DateTime` return type.
   d. Add import `libtime "github.com/bborbe/time"`.

4. Run `make test` after each file to catch compilation errors early.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `libtime.DateTime` from `github.com/bborbe/time` for timestamp fields
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
