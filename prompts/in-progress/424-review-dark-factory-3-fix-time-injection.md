---
status: approved
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T14:51:20Z"
---

<summary>
- Replaced time.Time and time.Duration in preflight struct fields with libtime.DateTime and libtime.Duration
- Replaced time.Now() calls in preflight.go and formatter.go with injected CurrentDateTimeGetter
- Follows the time injection pattern used throughout the rest of the codebase
</summary>

<objective>
Inject time dependencies in pkg/preflight/ and pkg/formatter/ to follow the libtime pattern used in the rest of the codebase.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-time-injection.md` for the time injection pattern.

Files to read before making changes:
- `pkg/preflight/preflight.go` — lines ~31 (checkedAt time.Time), ~40 (interval time.Duration), ~56 (interval param), ~97 (time.Now())
- `pkg/formatter/formatter.go` — line ~129 (formatTimestamp uses time.Now())
- `pkg/preflight/preflight_test.go` — existing test patterns
- `pkg/formatter/formatter_test.go` — existing test patterns
</context>

<requirements>
1. In `pkg/preflight/preflight.go`:
   - Change `checkedAt time.Time` field to `checkedAt libtime.DateTime`
   - Change `interval time.Duration` field to `interval libtime.Duration`
   - Change `interval time.Duration` parameter to `interval libtime.Duration`
   - Replace `time.Now()` at line ~97 with `currentDateTimeGetter.Now()` (inject via constructor)
   - Add `libtime.CurrentDateTimeGetter` to the struct fields and constructor

2. In `pkg/formatter/formatter.go`:
   - Change `formatTimestamp()` to use injected `libtime.CurrentDateTimeGetter`
   - Add `currentDateTimeGetter libtime.CurrentDateTimeGetter` to the formatter struct
   - Update all callers of `formatTimestamp` to pass the getter

3. Update all callers of changed constructors to pass the datetime getter.
</requirements>

<constraints>
- Only change files in this repo
- Do NOT commit — dark-factory handles git
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
make precommit
</verification>
