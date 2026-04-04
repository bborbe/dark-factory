---
status: approved
spec: ["044"]
created: "2026-04-04T20:50:00Z"
queued: "2026-04-04T21:49:26Z"
---

<summary>
- Two new config fields control timeout and auto-retry (both disabled by default)
- Existing projects get zero-config upgrade — no config changes needed, timeout is off unless explicitly set
- Invalid duration strings are rejected at daemon startup with a clear error
- A new permanently-failed status is added and handled everywhere statuses are consumed
- A new failure reason field in prompt frontmatter records why the last failure happened
- Prompts without the new field continue to parse correctly (zero-value default)
- New methods allow callers to mark prompts as permanently failed with a reason
- A config helper returns the parsed timeout duration, returning 0 when disabled (empty or "0")
</summary>

<objective>
Add the data model foundations for prompt timeout and auto-retry: new config fields with defaults and validation, a `permanently_failed` status, and a `lastFailReason` frontmatter field. Subsequent prompts build executor timeout and processor retry on top of these foundations.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before making any changes:
- `pkg/config/config.go` — `Config` struct, `Defaults()` function, `Validate()` method
- `pkg/prompt/prompt.go` — `PromptStatus` constants, `AvailablePromptStatuses`, `Frontmatter` struct, `MarkFailed`, `MarkApproved` methods
- `pkg/cmd/list.go` — to understand which statuses are handled in status filtering/display
- `pkg/cmd/requeue.go` — to see the `FailedPromptStatus` check that will later need to include `PermanentlyFailedPromptStatus`
- `pkg/cmd/prompt_complete.go` — to see which statuses are used for transitions
- `docs/configuration.md` — for config field naming conventions
</context>

<requirements>
**Step 1: Add config fields to `pkg/config/config.go`**

Add two fields to the `Config` struct:
```go
MaxPromptDuration string `yaml:"maxPromptDuration"`
AutoRetryLimit    int    `yaml:"autoRetryLimit"`
```

Do NOT set defaults for either field — the zero values mean disabled:
- `MaxPromptDuration: ""` (empty) = no timeout
- `AutoRetryLimit: 0` = no auto-retry

Projects opt in by setting e.g. `maxPromptDuration: "90m"` and `autoRetryLimit: 3`.

Add a `ParsedMaxPromptDuration() time.Duration` method on `Config`:
```go
// ParsedMaxPromptDuration returns the parsed duration from MaxPromptDuration.
// Returns 0 when MaxPromptDuration is empty or unparseable (disables timeout).
// Safe to call at any time — returns 0 on error, never panics.
func (c Config) ParsedMaxPromptDuration() time.Duration {
    if c.MaxPromptDuration == "" {
        return 0
    }
    d, err := time.ParseDuration(c.MaxPromptDuration)
    if err != nil {
        return 0
    }
    return d
}
```

Add validation for `maxPromptDuration` inside `Validate()` using the same `validation.Name` + `validation.HasValidationFunc` pattern as other fields:
```go
validation.Name("maxPromptDuration", validation.HasValidationFunc(func(ctx context.Context) error {
    if c.MaxPromptDuration == "" {
        return nil
    }
    if _, err := time.ParseDuration(c.MaxPromptDuration); err != nil {
        return errors.Errorf(ctx, "maxPromptDuration %q is not a valid duration: %v", c.MaxPromptDuration, err)
    }
    return nil
})),
```

Add validation for `autoRetryLimit` inside `Validate()`:
```go
validation.Name("autoRetryLimit", validation.HasValidationFunc(func(ctx context.Context) error {
    if c.AutoRetryLimit < 0 {
        return errors.Errorf(ctx, "autoRetryLimit must not be negative, got %d", c.AutoRetryLimit)
    }
    return nil
})),
```

**Step 2: Add `permanently_failed` status to `pkg/prompt/prompt.go`**

Add the new status constant after `CancelledPromptStatus`:
```go
// PermanentlyFailedPromptStatus indicates the prompt exhausted all auto-retries and will not be retried automatically.
PermanentlyFailedPromptStatus PromptStatus = "permanently_failed"
```

Add it to `AvailablePromptStatuses`:
```go
var AvailablePromptStatuses = PromptStatuses{
    IdeaPromptStatus,
    DraftPromptStatus,
    ApprovedPromptStatus,
    ExecutingPromptStatus,
    CompletedPromptStatus,
    FailedPromptStatus,
    InReviewPromptStatus,
    PendingVerificationPromptStatus,
    CancelledPromptStatus,
    PermanentlyFailedPromptStatus, // NEW
}
```

**Step 3: Add `LastFailReason` to `Frontmatter` in `pkg/prompt/prompt.go`**

Add to the `Frontmatter` struct:
```go
LastFailReason string `yaml:"lastFailReason,omitempty"`
```

Place it after `RetryCount`.

**Step 4: Add `SetLastFailReason` and `MarkPermanentlyFailed` methods on `PromptFile` in `pkg/prompt/prompt.go`**

After the existing `MarkFailed` method:
```go
// SetLastFailReason records the human-readable reason for the last failure.
func (pf *PromptFile) SetLastFailReason(reason string) {
    pf.Frontmatter.LastFailReason = reason
}

// MarkPermanentlyFailed sets status to permanently_failed and records the reason.
func (pf *PromptFile) MarkPermanentlyFailed(reason string) {
    pf.Frontmatter.Status = string(PermanentlyFailedPromptStatus)
    pf.Frontmatter.Completed = pf.now().UTC().Format(time.RFC3339)
    pf.Frontmatter.LastFailReason = reason
}
```

**Step 5: Update `pkg/cmd/prompt_complete.go` to allow `permanently_failed` in allowed source statuses**

In `prompt_complete.go`, find the slice of allowed statuses that contains `prompt.FailedPromptStatus` (used to validate which statuses can transition to completed). Add `prompt.PermanentlyFailedPromptStatus` to that same slice so `dark-factory prompt complete` can move a permanently-failed prompt.

**Step 6: Update `pkg/cmd/list.go` to handle `permanently_failed`**

In `list.go`, find where `prompt.FailedPromptStatus` is used for the `--failed` flag filter. Add `prompt.PermanentlyFailedPromptStatus` to the same filter condition so `dark-factory prompt list --failed` shows both failed and permanently-failed prompts.

**Step 7: Update `docs/configuration.md`**

Add documentation for the two new config fields (`maxPromptDuration`, `autoRetryLimit`) near the existing `maxContainers` documentation. Include defaults (0 = disabled) and behavior description.

**Step 8: Add tests**

In `pkg/config/config_test.go` (following existing Ginkgo patterns):
- `maxPromptDuration: "invalid"` → `Validate` returns error
- `maxPromptDuration: "90m"` → `Validate` passes, `ParsedMaxPromptDuration()` returns `90 * time.Minute`
- `maxPromptDuration: "0"` → `Validate` passes, `ParsedMaxPromptDuration()` returns 0 (disabled)
- `maxPromptDuration: ""` (default, no value set) → `Validate` passes, `ParsedMaxPromptDuration()` returns 0 (disabled)
- `autoRetryLimit: -1` → `Validate` returns error
- `autoRetryLimit: 0` (default) → `Validate` passes (disabled)
- `autoRetryLimit: 3` → `Validate` passes (enabled, 3 retries)

In `pkg/prompt/prompt_test.go` (following existing patterns):
- `PermanentlyFailedPromptStatus.Validate(ctx)` → no error (it's in AvailablePromptStatuses)
- `MarkPermanentlyFailed("some reason")` → status is `permanently_failed`, `LastFailReason` is `"some reason"`, `Completed` is set
- `SetLastFailReason("msg")` → `LastFailReason` is `"msg"`
- Frontmatter without `lastFailReason` parses correctly (zero value)
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Do NOT change the `MarkFailed()` method signature — it remains `MarkFailed()` with no arguments
- `permanently_failed` prompts must be added to `AvailablePromptStatuses` so status validation passes
- Keep `retryCount` already existing in `Frontmatter` — do NOT add it again
- Follow existing error wrapping: `errors.Errorf(ctx, ...)` — never `fmt.Errorf`
- Zero values are the correct defaults — do NOT add entries to `Defaults()` for `MaxPromptDuration` or `AutoRetryLimit`
- `ParsedMaxPromptDuration()` must not panic if called after successful `Validate()` — the validation ensures only valid strings are stored
</constraints>

<verification>
```bash
# Confirm new status is added
grep -n "permanently_failed\|PermanentlyFailed" pkg/prompt/prompt.go

# Confirm new config fields
grep -n "MaxPromptDuration\|AutoRetryLimit" pkg/config/config.go

# Confirm LastFailReason in Frontmatter
grep -n "LastFailReason\|lastFailReason" pkg/prompt/prompt.go

make precommit
```
Must pass with no errors.
</verification>
