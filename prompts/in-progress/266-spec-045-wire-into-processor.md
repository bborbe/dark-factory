---
status: approved
spec: [045-test-command]
created: "2026-04-06T16:30:00Z"
queued: "2026-04-06T16:40:32Z"
---

<summary>
- The processor injects fast-feedback instructions into every prompt when the test command is configured
- The instructions appear after the changelog section and before the validation-command section
- Setting the test command to empty string suppresses injection entirely
- The new field is passed through the constructor and both factory call sites
- Existing prompts continue to work unchanged with the default applied
- Tests cover injection when set, suppression when empty, and correct ordering relative to other suffixes
</summary>

<objective>
Wire the `testCommand` config value into the prompt execution pipeline. After the changelog suffix is appended, inject the `TestCommandSuffix` (from prompt 1) when `testCommand` is non-empty. Thread `testCommand` through `NewProcessor`, `CreateProcessor`, and the two `CreateProcessor` call sites in factory.go. This is the second prompt for spec 045 — the prerequisite (prompt 1) must be completed first: `TestCommandSuffix` in `pkg/report/suffix.go` and `TestCommand` in `pkg/config/config.go` must already exist.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Read these files before making any changes:
- `pkg/processor/processor.go` — `processor` struct (around line 143), `NewProcessor` constructor (lines ~50-83), `enrichPromptContent` method (around lines 933-953). The test-command suffix must be inserted between `ChangelogSuffix` and `ValidationSuffix` in `enrichPromptContent`.
- `pkg/factory/factory.go` — `CreateProcessor` function signature (around line 495) and its two call sites: `CreateRunner` (calls `CreateProcessor` around line 272) and `CreateOneShotRunner` (calls `CreateProcessor` around line 337). Pass `cfg.TestCommand` after `cfg.ValidationPrompt` in both.
- `pkg/processor/processor_test.go` — existing `NewProcessor(...)` call patterns; all calls need the new `testCommand string` parameter added.
- `pkg/factory/factory_test.go` — existing `CreateProcessor(...)` call patterns; all calls need the new parameter added.
- `pkg/report/suffix.go` — confirm `TestCommandSuffix(cmd string) string` exists (added by prompt 1).
- `pkg/config/config.go` — confirm `TestCommand string` field exists (added by prompt 1).
</context>

<requirements>

## 1. Add `testCommand` field to the `processor` struct in `pkg/processor/processor.go`

In the `processor` struct, add after `validationCommand string`:
```go
testCommand string
```

## 2. Add `testCommand` parameter to `NewProcessor` in `pkg/processor/processor.go`

In the `NewProcessor` function signature, add `testCommand string` after `validationPrompt string`:
```go
validationPrompt  string,
testCommand       string,
```

In the constructor body, wire the field:
```go
testCommand: testCommand,
```
Place it after the `validationPrompt: validationPrompt` line.

## 3. Inject test-command suffix in `enrichPromptContent` in `pkg/processor/processor.go`

The current suffix ordering in `enrichPromptContent` is:
1. Completion report (`report.Suffix()`)
2. Changelog (`report.ChangelogSuffix()`)
3. Validation command (`report.ValidationSuffix(p.validationCommand)`)
4. Validation prompt (`report.ValidationPromptSuffix(criteria)`)

Insert the test-command injection between changelog and validation command:

```go
// Inject project-level test command for fast iteration feedback
if p.testCommand != "" {
    content = content + report.TestCommandSuffix(p.testCommand)
}
```

The final ordering must be:
1. Completion report
2. Changelog (if applicable)
3. **Test command (if configured)** ← NEW
4. Validation command (if configured)
5. Validation prompt (if configured)

## 4. Add `testCommand` parameter to `CreateProcessor` in `pkg/factory/factory.go`

In the `CreateProcessor` function signature, add `testCommand string` after `validationPrompt string`:
```go
validationPrompt  string,
testCommand       string,
```

Pass it through to `processor.NewProcessor(...)` at the matching position (after `validationPrompt`).

## 5. Thread `cfg.TestCommand` through both `CreateProcessor` call sites in `pkg/factory/factory.go`

In `CreateRunner`, find the existing `CreateProcessor(...)` call. After `cfg.ValidationPrompt`, add `cfg.TestCommand`:
```go
cfg.ValidationCommand, cfg.ValidationPrompt, cfg.TestCommand,
```

In `CreateOneShotRunner`, find the existing `CreateProcessor(...)` call. Apply the same change:
```go
cfg.ValidationCommand, cfg.ValidationPrompt, cfg.TestCommand,
```

## 6. Update all existing `NewProcessor` calls in `pkg/processor/processor_test.go`

Find all `processor.NewProcessor(...)` (or `NewProcessor(...)`) calls in the test file. Add `""` (empty string) for the new `testCommand` parameter immediately after the `validationPrompt` argument. This preserves existing test behavior unchanged.

## 7. Update all existing `CreateProcessor` calls in `pkg/factory/factory_test.go`

Find all `CreateProcessor(...)` calls in the factory test file. Add `""` (empty string) for the new `testCommand` parameter immediately after the `validationPrompt` argument.

## 8. Add tests in `pkg/processor/processor_test.go`

Using the existing Ginkgo/Gomega test patterns:

**Test A: testCommand is injected when set**

Create a processor with `testCommand: "make test"`. Call `enrichPromptContent` (or capture the content passed to the executor mock). Assert the returned content:
- Contains `"make test"` (from `TestCommandSuffix`)
- Contains `"Fast Feedback"` (confirms the right suffix section header)

Hint: find an existing test that calls `processPrompt` or captures executor input content, and follow the same pattern.

**Test B: testCommand is suppressed when empty**

Create a processor with `testCommand: ""`. Assert the returned content does NOT contain `"Fast Feedback"` (the TestCommandSuffix header).

**Test C: test-command suffix appears before validation-command suffix**

Create a processor with both `testCommand: "make test"` and `validationCommand: "make precommit"`. Assert that the index of `"Fast Feedback"` in the content is less than the index of `"make precommit"` (or the validation-command section header).

</requirements>

<constraints>
- The `validationCommand` field, its default (`make precommit`), and its role as the authoritative success/failure signal must not change
- The completion report format (markers, JSON structure) must not change
- Existing `.dark-factory.yaml` files without `testCommand` continue to work — `cfg.TestCommand` defaults to `"make test"` from `Defaults()`
- Do NOT modify the executor interface or any executor implementations
- Empty `testCommand` → no injection, no log output, zero overhead
- Do NOT commit — dark-factory handles git
- Existing tests must still pass; only add the new parameter (pass `""`) to existing `NewProcessor` and `CreateProcessor` calls in test files
- Follow existing error wrapping: `errors.Errorf(ctx, ...)` — never `fmt.Errorf`
- `make precommit` must pass
</constraints>

<verification>
```bash
# Confirm testCommand field in processor struct
grep -n "testCommand" pkg/processor/processor.go

# Confirm injection in enrichPromptContent
grep -n "TestCommandSuffix\|testCommand" pkg/processor/processor.go

# Confirm threading in factory
grep -n "TestCommand\|testCommand" pkg/factory/factory.go

make precommit
```
Must pass with no errors.
</verification>
