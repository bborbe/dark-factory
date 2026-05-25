---
status: completed
summary: Replaced cancelledByUser bool with sync/atomic.Bool to fix data race in processor.go
container: dark-factory-exec-413-review-dark-factory-1-fix-processor-race
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T14:51:20Z"
started: "2026-05-25T14:52:36Z"
completed: "2026-05-25T14:55:52Z"
---

<summary>
- Fixed data race in processor.go where cancelledByUser bool was read/written unsynchronized across goroutines
- Replaced raw bool with sync/atomic.Bool for safe cross-goroutine access
- The race occurred when Execute() returned quickly due to context cancellation before the monitoring goroutine could set the flag
</summary>

<objective>
Fix the race condition in pkg/processor/processor.go where a raw bool is written by one goroutine and read by another without synchronization.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `pkg/processor/processor.go` — line ~389, `cancelledByUser` variable and its goroutine, line ~395 write, line ~402 read
- `pkg/processor/processor_test.go` — existing test patterns for this area
</context>

<requirements>
1. In `pkg/processor/processor.go`, replace the raw `bool` type for `cancelledByUser` with `atomic.Bool`:
   - Import `sync/atomic`
   - Change `cancelledByUser := false` to `var cancelledByUser atomic.Bool`
   - Change `cancelledByUser = true` to `cancelledByUser.Store(true)`
   - Change `if cancelledByUser { ... }` to `if cancelledByUser.Load() { ... }`

2. Verify no other files reference `cancelledByUser` — if they do, update them to use `.Load()`.
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
make precommit
</verification>
