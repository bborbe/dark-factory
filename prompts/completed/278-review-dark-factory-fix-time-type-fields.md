---
status: completed
summary: Replaced time.Time with libtime.DateTime for timestamp fields in CompletedPrompt, promptWithTime, executingPrompt, skippedPrompts map, and parseCreated across pkg/status, pkg/processor, and pkg/reindex, updating callers and test files accordingly.
container: dark-factory-278-review-dark-factory-fix-time-type-fields
dark-factory-version: v0.104.2-dirty
created: "2026-04-06T00:00:00Z"
queued: "2026-04-06T18:07:25Z"
started: "2026-04-06T19:14:48Z"
completed: "2026-04-06T19:28:01Z"
---

<summary>
- Several components store timestamps using the standard library type instead of the project type
- The project standard is to use the library date-time type for all timestamp storage
- Affected components are in the status, processor, and reindex packages
- Replacing the standard type makes storage consistent and compatible with project utilities
- Callers that do arithmetic on these fields need short type conversion expressions
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
   f. Update all callers that use these changed fields:
      - `populateLogInfo` (~line 436): `latestTime` is compared with `info.ModTime().After(latestTime)` — use `info.ModTime().After(time.Time(latestTime))`.
      - `findExecutingPrompt` (~line 326): creates `startedTime := time.Time{}` and parses into it — change to `libtime.DateTime{}`.
      - `populateExecutingPrompt` (~line 383): uses `.IsZero()` and `time.Since()` on `StartedTime` — convert: `time.Time(executing.StartedTime).IsZero()` and `time.Since(time.Time(executing.StartedTime))`.
      - Any sorting or comparison: use `time.Time(field).Before(...)`, `time.Time(field).After(...)`, `time.Time(field).Sub(...)` where needed.
   g. Add import `libtime "github.com/bborbe/time"`.

2. In `pkg/processor/processor.go`:
   a. Change `skippedPrompts map[string]time.Time` field to `skippedPrompts map[string]libtime.DateTime`.
   b. Change `make(map[string]time.Time)` to `make(map[string]libtime.DateTime)` in all locations.
   c. Update all code that reads from or writes to `skippedPrompts`:
      - Line ~519: `p.skippedPrompts[pr.Path] = fileInfo.ModTime()` — change to `p.skippedPrompts[pr.Path] = libtime.DateTime(fileInfo.ModTime())`.
      - Line ~499: `lastSkipped, wasSkipped := p.skippedPrompts[pr.Path]` — the comparison with `fileInfo.ModTime()` needs conversion: compare `time.Time(lastSkipped)` with `fileInfo.ModTime()`.
   d. Add import `libtime "github.com/bborbe/time"`.

3. In `pkg/reindex/reindex.go`:
   a. Change `parseCreated` signature from `func parseCreated(...) (time.Time, bool)` to `func parseCreated(...) (libtime.DateTime, bool)`.
   b. Change `return time.Time{}, false` to `return libtime.DateTime{}, false` (both occurrences).
   c. Update callers of `parseCreated` — `entriesLess` (~line 155) calls `parseCreated` and uses `.Equal()`, `.Before()` on the result. Convert: `time.Time(ta).Equal(time.Time(tb))`, `time.Time(ta).Before(time.Time(tb))`.
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
