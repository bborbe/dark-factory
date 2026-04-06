---
status: approved
created: "2026-04-06T14:30:00Z"
queued: "2026-04-06T13:55:36Z"
---

<summary>
- Prompt timeouts fire on time even when dark-factory runs as a macOS background process
- The timeout mechanism uses wall-clock deadline polling instead of a single long sleep
- Timer coalescing and App Nap can no longer delay the timeout by 20-30 minutes
- The completion-report grace period also uses wall-clock polling for consistency
- Time is injected via CurrentDateTimeGetter for testability — no direct time.Now() calls
- Existing behavior is unchanged when timeouts are disabled (duration == 0)
</summary>

<objective>
Replace `time.After(duration)` in `timeoutKiller` with a wall-clock deadline check that polls periodically. macOS timer coalescing delays long `time.After` calls by 20-30 minutes when the process runs in the background. Polling every 30 seconds with a wall-clock comparison is immune to this. Apply the same fix to the grace period sleep in `watchForCompletionReport`. Inject `CurrentDateTimeGetter` following the project's time injection pattern.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before making changes:
- `pkg/executor/executor.go` — `NewDockerExecutor` constructor (~line 41), `dockerExecutor` struct (~line 67), `buildRunFuncs` (~line 167), `timeoutKiller` function (~line 223), `watchForCompletionReport` function (~line 255)
- `pkg/executor/executor_internal_test.go` — existing test structure for pattern reference (Ginkgo v2 `Describe`/`It` blocks)
- `pkg/factory/factory.go` — `createDockerExecutor` function to see how `NewDockerExecutor` is wired

Follow the time injection pattern from the coding guidelines:
- Import: `libtime "github.com/bborbe/time"`
- Use `libtime.CurrentDateTimeGetter` interface, never call `time.Now()` directly
- Receive getter as constructor parameter, store in struct

**This prompt depends on prompt 1** (`1-fix-command-runner-ctx-cancellation.md`). It modifies `timeoutKiller` and `watchForCompletionReport` which are both in `executor.go`.
</context>

<requirements>
1. **Add `currentDateTimeGetter` to `dockerExecutor`** in `pkg/executor/executor.go`:

   Add import:
   ```go
   libtime "github.com/bborbe/time"
   ```

   Add parameter to `NewDockerExecutor`:
   ```go
   func NewDockerExecutor(
       // ... existing params ...
       maxPromptDuration time.Duration,
       currentDateTimeGetter libtime.CurrentDateTimeGetter,
   ) Executor {
   ```

   Add field to `dockerExecutor` struct:
   ```go
   type dockerExecutor struct {
       // ... existing fields ...
       currentDateTimeGetter libtime.CurrentDateTimeGetter
   }
   ```

   Set it in the constructor return.

2. **Create `waitUntilDeadline` helper** in `pkg/executor/executor.go`:

   ```go
   // waitUntilDeadline blocks until the deadline is reached or ctx is cancelled.
   // tickInterval controls how often the wall-clock is checked.
   // Returns true if the deadline was reached, false if ctx was cancelled.
   func waitUntilDeadline(ctx context.Context, currentDateTimeGetter libtime.CurrentDateTimeGetter, deadline time.Time, tickInterval time.Duration) bool {
       ticker := time.NewTicker(tickInterval)
       defer ticker.Stop()
       for {
           select {
           case <-ctx.Done():
               return false
           case <-ticker.C:
               if !time.Time(currentDateTimeGetter.Now()).Before(deadline) {
                   return true
               }
           }
       }
   }
   ```

   Uses `libtime.CurrentDateTimeGetter` directly per time injection conventions — no `func() time.Time` wrappers.

3. **Replace the timer in `timeoutKiller`**:

   Add `currentDateTimeGetter libtime.CurrentDateTimeGetter` parameter to `timeoutKiller`:
   ```go
   func timeoutKiller(
       ctx context.Context,
       duration time.Duration,
       containerName string,
       runner commandRunner,
       currentDateTimeGetter libtime.CurrentDateTimeGetter,
   ) error {
       deadline := time.Time(currentDateTimeGetter.Now()).Add(duration)
       if !waitUntilDeadline(ctx, currentDateTimeGetter, deadline, 30*time.Second) {
           return nil // ctx cancelled — normal container exit
       }
       // ... rest unchanged (docker stop / kill / error return) ...
   }
   ```

   Remove the `time.After(duration)` select block entirely.

4. **Replace the grace period sleep in `watchForCompletionReport`**:

   Add `currentDateTimeGetter libtime.CurrentDateTimeGetter` parameter to `watchForCompletionReport`. Find the `time.After(gracePeriod)` call and replace with:
   ```go
   if !waitUntilDeadline(ctx, currentDateTimeGetter, time.Time(currentDateTimeGetter.Now()).Add(gracePeriod), 30*time.Second) {
       return nil
   }
   ```

5. **Update `buildRunFuncs`** to pass `e.currentDateTimeGetter` to both `timeoutKiller` and `watchForCompletionReport` calls.

6. **Update `NewDockerExecutor` callers in `pkg/factory/factory.go`**:

   The factory already has `currentDateTimeGetter` (parameter of `CreateFactory` at ~line 68). There are two call sites:

   a. **Direct call in `CreateSpecGenerator`** (~line 391): `executor.NewDockerExecutor(...)` — add `currentDateTimeGetter` as last argument. The `CreateSpecGenerator` function (~line 383) already receives `currentDateTimeGetter`.

   b. **Via `createDockerExecutor` helper** (~line 457): Add `currentDateTimeGetter libtime.CurrentDateTimeGetter` parameter to `createDockerExecutor` and pass it through to `NewDockerExecutor`. Update the caller at ~line 535 to pass `currentDateTimeGetter`.

7. **Update tests in `pkg/executor/executor_internal_test.go`**:

   Add a `Describe("waitUntilDeadline")` block with fast tests using a short tick interval. Use `libtime.NewCurrentDateTime()` for real-time tests and a fake `CurrentDateTimeGetter` for controlled tests:

   ```go
   Describe("waitUntilDeadline", func() {
       It("returns true when deadline is reached", func() {
           ctx := context.Background()
           getter := libtime.NewCurrentDateTime()
           deadline := time.Time(getter.Now()).Add(50 * time.Millisecond)
           result := waitUntilDeadline(ctx, getter, deadline, 5*time.Millisecond)
           Expect(result).To(BeTrue())
       })

       It("returns false when context is cancelled", func() {
           ctx, cancel := context.WithCancel(context.Background())
           getter := libtime.NewCurrentDateTime()
           deadline := time.Time(getter.Now()).Add(10 * time.Second)
           go func() {
               time.Sleep(50 * time.Millisecond)
               cancel()
           }()
           result := waitUntilDeadline(ctx, getter, deadline, 5*time.Millisecond)
           Expect(result).To(BeFalse())
       })

       It("returns true immediately when deadline is already past", func() {
           ctx := context.Background()
           getter := libtime.NewCurrentDateTime()
           deadline := time.Time(getter.Now()).Add(-1 * time.Second)
           result := waitUntilDeadline(ctx, getter, deadline, 5*time.Millisecond)
           Expect(result).To(BeTrue())
       })

       It("uses injected time getter for deadline comparison", func() {
           ctx := context.Background()
           calls := 0
           getter := libtime.CurrentDateTimeGetterFunc(func() libtime.DateTime {
               calls++
               if calls <= 2 {
                   return libtime.DateTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
               }
               return libtime.DateTime(time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)) // past deadline
           })
           deadline := time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC)
           result := waitUntilDeadline(ctx, getter, deadline, 5*time.Millisecond)
           Expect(result).To(BeTrue())
           Expect(calls).To(BeNumerically(">=", 3))
       })
   })
   ```

   Update existing `timeoutKiller` tests (if any exist after prompt 1) to pass `currentDateTimeGetter` using `libtime.NewCurrentDateTime()`.

8. **Do NOT change the function signatures** of `timeoutKiller` or `watchForCompletionReport` beyond adding the `currentDateTimeGetter` parameter. The tick interval is hardcoded at 30 seconds in production — only `waitUntilDeadline` is parameterized.

9. **Do NOT remove the `time` import** — it is still used for `time.Duration`, `time.NewTicker`, etc.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Do NOT change `buildRunFuncs` signature or `Execute`/`Reattach` signatures
- Do NOT change the wiring of timeoutKiller into CancelOnFirstFinish
- When `maxPromptDuration == 0`, no timeout goroutine is started — this is handled in `buildRunFuncs` and must not change
- Use `errors.Errorf(ctx, ...)` for errors — never `fmt.Errorf`
- Follow time injection pattern: `libtime.CurrentDateTimeGetter` in constructors, never `time.Now()` in production code
- Follow project conventions: Ginkgo v2/Gomega tests, doc comments on exports, structured logging
</constraints>

<verification>
```bash
# Verify waitUntilDeadline exists
grep -n 'func waitUntilDeadline' pkg/executor/executor.go

# Verify time.After is no longer used in timeoutKiller or watchForCompletionReport
grep -n 'time.After' pkg/executor/executor.go
# Expected: no results, or results only outside timeoutKiller/watchForCompletionReport

# Verify 30-second tick interval in production use
grep -n '30 \* time.Second' pkg/executor/executor.go

# Verify no time.Now() in production code
grep -n 'time.Now()' pkg/executor/executor.go
# Expected: no results

# Verify CurrentDateTimeGetter is used
grep -n 'CurrentDateTimeGetter' pkg/executor/executor.go

# Run tests
go test ./pkg/executor/ -run "waitUntilDeadline" -v -count=1
make precommit
```
All must pass with no errors.
</verification>
