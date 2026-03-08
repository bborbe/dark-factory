---
status: created
---

<objective>
Replace `nowFunc func() time.Time` in `PromptFile` and `SpecFile` with `libtime.CurrentDateTimeGetter` from `github.com/bborbe/time`. This aligns with the bborbe ecosystem pattern for time injection and enables proper testability without nil-guard fallbacks.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/prompt/prompt.go` — `PromptFile` struct (line 195), `nowFunc` field (line 199), `now()` method (line 287-292), `Load` function (lines 216-232) which sets `nowFunc: time.Now`.
Read `pkg/spec/spec.go` — `SpecFile` struct (line 67), `nowFunc` field (line 72), `now()` method (line 75-80), `SetNowFunc` (line 82-84), `Load` function (lines 100-128) which sets `nowFunc: time.Now`.
Read `pkg/prompt/prompt_test.go` — how tests currently set `nowFunc` on `PromptFile`.
Read `pkg/spec/spec_test.go` — how tests use `SetNowFunc`.
Read `pkg/factory/factory.go` — `createPromptManager` (line 40+) and any factory functions that create PromptFile/SpecFile.
Read `/home/node/.claude/docs/go-patterns.md` and `/home/node/.claude/docs/go-testing.md`.

The `github.com/bborbe/time` library provides:
```go
import libtime "github.com/bborbe/time"

// Interface — accept in constructors
type CurrentDateTimeGetter interface {
    Now() DateTime
}

// DateTime is type DateTime time.Time — convert with:
//   time.Time(dateTime)     // DateTime → time.Time
//   libtime.DateTime(t)     // time.Time → DateTime

// Create real implementation:
libtime.NewCurrentDateTime() // returns CurrentDateTime (implements both Getter and Setter)

// In tests — use real implementation with SetNow:
currentDateTime := libtime.NewCurrentDateTime()
currentDateTime.SetNow(libtime.DateTime(fixedTime))
```
</context>

<requirements>
1. Update `go.mod` to make `github.com/bborbe/time` a direct dependency:
   ```bash
   go get github.com/bborbe/time@latest
   ```

2. In `pkg/prompt/prompt.go`:

   a. Add import: `libtime "github.com/bborbe/time"`

   b. Replace `nowFunc` field in `PromptFile` struct (line 199):
   ```go
   // Before:
   nowFunc     func() time.Time

   // After:
   currentDateTimeGetter libtime.CurrentDateTimeGetter
   ```

   c. Replace `now()` method (lines 287-292):
   ```go
   // Before:
   func (pf *PromptFile) now() time.Time {
       if pf.nowFunc == nil {
           return time.Now()
       }
       return pf.nowFunc()
   }

   // After:
   func (pf *PromptFile) now() time.Time {
       return time.Time(pf.currentDateTimeGetter.Now())
   }
   ```
   No nil guard needed — the getter is always set.

   d. In `Load` function, replace both `nowFunc: time.Now` occurrences (lines 219, 229):
   ```go
   // Before:
   nowFunc: time.Now,

   // After:
   currentDateTimeGetter: libtime.NewCurrentDateTime(),
   ```

   e. Remove `SetNowFunc` if it exists on `PromptFile`. If tests need to set time, they should use `libtime.NewCurrentDateTime()` with `SetNow()`.

   f. Add a `SetCurrentDateTimeGetter` method for test injection:
   ```go
   // SetCurrentDateTimeGetter sets the time source for testability.
   func (pf *PromptFile) SetCurrentDateTimeGetter(getter libtime.CurrentDateTimeGetter) {
       pf.currentDateTimeGetter = getter
   }
   ```

3. In `pkg/spec/spec.go`:

   a. Add import: `libtime "github.com/bborbe/time"`

   b. Replace `nowFunc` field in `SpecFile` struct (line 72):
   ```go
   // Before:
   nowFunc     func() time.Time

   // After:
   currentDateTimeGetter libtime.CurrentDateTimeGetter
   ```

   c. Replace `now()` method (lines 75-80):
   ```go
   func (s *SpecFile) now() time.Time {
       return time.Time(s.currentDateTimeGetter.Now())
   }
   ```

   d. Replace `SetNowFunc` method (lines 82-84):
   ```go
   // Before:
   func (s *SpecFile) SetNowFunc(f func() time.Time) {
       s.nowFunc = f
   }

   // After:
   func (s *SpecFile) SetCurrentDateTimeGetter(getter libtime.CurrentDateTimeGetter) {
       s.currentDateTimeGetter = getter
   }
   ```

   e. In `Load` function, replace both `nowFunc: time.Now` occurrences (lines 117, 126):
   ```go
   // Before:
   nowFunc: time.Now,

   // After:
   currentDateTimeGetter: libtime.NewCurrentDateTime(),
   ```

4. Update tests in `pkg/prompt/prompt_test.go`:

   Replace any `pf.nowFunc = func() time.Time { return fixedTime }` with:
   ```go
   currentDateTime := libtime.NewCurrentDateTime()
   currentDateTime.SetNow(libtime.DateTime(fixedTime))
   pf.SetCurrentDateTimeGetter(currentDateTime)
   ```
   Add import: `libtime "github.com/bborbe/time"`

5. Update tests in `pkg/spec/spec_test.go`:

   Replace any `sf.SetNowFunc(func() time.Time { return fixedTime })` with:
   ```go
   currentDateTime := libtime.NewCurrentDateTime()
   currentDateTime.SetNow(libtime.DateTime(fixedTime))
   sf.SetCurrentDateTimeGetter(currentDateTime)
   ```
   Add import: `libtime "github.com/bborbe/time"`

6. Run `go mod tidy` to clean up.
</requirements>

<constraints>
- Do NOT change the `Load` function signatures for `PromptFile` or `SpecFile` — callers must not change
- Do NOT inject `CurrentDateTimeGetter` via the `Load` constructor — keep injection via setter method (matches current pattern where `Load` returns a fully constructed object)
- Do NOT touch `pkg/watcher/` or `pkg/git/pr_merger.go` — those are a separate prompt
- Do NOT use counterfeiter mock for `CurrentDateTimeGetter` — use `libtime.NewCurrentDateTime()` with `SetNow()` in tests (per coding guidelines: "Don't mock CurrentDateTime")
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Verify no `nowFunc` remains in prompt.go or spec.go:
```bash
grep -n "nowFunc" pkg/prompt/prompt.go pkg/spec/spec.go
# Expected: no output
```

Verify libtime is imported:
```bash
grep "bborbe/time" pkg/prompt/prompt.go pkg/spec/spec.go
# Expected: both files show the import
```

Run targeted tests:
```bash
go test -v ./pkg/prompt/... -run "PromptFile|Frontmatter|Status"
go test -v ./pkg/spec/... -run "SpecFile|Load|Status"
```
</verification>

<success_criteria>
- `nowFunc func() time.Time` replaced with `currentDateTimeGetter libtime.CurrentDateTimeGetter` in both structs
- No nil-guard fallback to `time.Now()` — getter is always set
- All tests use `libtime.NewCurrentDateTime()` with `SetNow()` instead of anonymous `func() time.Time`
- `github.com/bborbe/time` is a direct dependency in `go.mod`
- `make precommit` passes
</success_criteria>
