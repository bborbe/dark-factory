---
status: approved
spec: [045-test-command]
container: dark-factory-265-spec-045-config-and-suffix
dark-factory-version: v0.104.2-dirty
created: "2026-04-06T16:30:00Z"
queued: "2026-04-06T16:40:32Z"
started: "2026-04-06T16:40:34Z"
---

<summary>
- Projects can configure a fast feedback command (`testCommand`) separately from the final validation gate
- The new field defaults to `make test` — existing configs work unchanged
- Setting `testCommand: ""` explicitly opts out of test-command injection
- A new suffix function provides the agent with clear fast-iteration instructions
- The validation-command suffix wording is updated to clarify it runs exactly once at the end
- The new field is documented in docs/configuration.md following existing patterns
- All new behavior is covered by tests
</summary>

<objective>
Add the `testCommand` config field (default `make test`) and the `TestCommandSuffix` function to the report package. Update `ValidationSuffix` wording to emphasize it is a final gate run once at the end. These are the data-model foundations for spec 045 — processor injection is handled by the next prompt.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Read these files before making any changes:
- `pkg/config/config.go` — `Config` struct, `Defaults()`, `Validate()`. Note: `ValidationCommand` already has default `"make precommit"` in `Defaults()`. Follow the same pattern for `testCommand`.
- `pkg/report/suffix.go` — current `Suffix()`, `ValidationSuffix()`, `ChangelogSuffix()`, `ValidationPromptSuffix()` functions. The new `TestCommandSuffix(cmd string) string` follows the same string-return pattern.
- `pkg/config/config_test.go` — existing Ginkgo test patterns for config validation and defaults.
- `pkg/report/report_test.go` — existing suffix test patterns. Add new test cases here (do NOT create a new suffix_test.go).
- `docs/configuration.md` — look for the `### validationCommand` section to understand documentation style for the new `testCommand` section.
</context>

<requirements>

## 1. Add `TestCommand` field to `pkg/config/config.go`

Add the field to the `Config` struct after `ValidationPrompt`:
```go
TestCommand string `yaml:"testCommand"`
```

Set the default in `Defaults()` after `ValidationCommand`:
```go
TestCommand: "make test",
```

No additional validation is needed — empty string is a valid opt-out. Do NOT add a `validation.Name("testCommand", ...)` entry to `Validate()`.

## 2. Add `TestCommandSuffix` to `pkg/report/suffix.go`

Add the following function after `ChangelogSuffix()` (before `ValidationSuffix`):

```go
// TestCommandSuffix returns the markdown text injected when a project-level test command is configured.
// It instructs the agent to use the fast command repeatedly during development for quick feedback,
// as opposed to the validation command which is the authoritative final gate run once at the end.
func TestCommandSuffix(cmd string) string {
	return "\n\n---\n\n## Fast Feedback Command (Run Repeatedly During Development)\n\nAfter each code change, run the following command for fast build/test feedback:\n\n```\n" + cmd + "\n```\n\nUse this command frequently while iterating — run it after each meaningful code change to catch compile errors and test failures early. Do NOT wait until the very end.\n"
}
```

## 3. Update `ValidationSuffix` wording in `pkg/report/suffix.go`

Replace the current `ValidationSuffix` function body with updated wording that makes clear it is a final gate run exactly once at the end:

```go
// ValidationSuffix returns the markdown text injected when a project-level validation command is configured.
// It instructs the agent to run the command exactly once as the authoritative final check,
// overriding any <verification> section in the prompt. The exit code determines success or failure.
func ValidationSuffix(cmd string) string {
	return "\n\n---\n\n## Project Validation Command (REQUIRED — run ONCE at the end)\n\nWhen all code changes are complete, run the following command exactly once as the authoritative final validation step:\n\n```\n" + cmd + "\n```\n\nThis overrides any `<verification>` section in this prompt. Report `\"status\":\"success\"` if and only if this command exits 0. Do NOT run this command repeatedly during iteration — use the fast feedback command for that.\n"
}
```

## 4. Update `docs/configuration.md`

In the Validation section, add a `### testCommand (fast iteration feedback)` subsection immediately before `### validationCommand`. Insert it so the order in the doc matches the intended execution order (test command used during iteration, validation command at the end):

```markdown
### testCommand (fast iteration feedback)

A shell command the YOLO agent runs repeatedly during development for fast build/test feedback. Unlike `validationCommand`, this is not authoritative — it is a development aid that runs frequently while the agent iterates on code changes.

```yaml
testCommand: "make test"
```

Default: `make test`. Set to empty string to disable injection entirely.
```

Also update the `### validationCommand` description to note it is the **final gate**:

Find the line:
```
A shell command whose exit code determines success or failure. Runs first.
```
Replace with:
```
A shell command whose exit code determines success or failure. Runs exactly once at the end as the authoritative final gate — not during iteration.
```

## 5. Add tests

### `pkg/config/config_test.go`

Add test cases using the existing Ginkgo `Describe`/`It` patterns:

- `Defaults()` returns `TestCommand: "make test"` — confirm the default is set
- A `Config` with `TestCommand: ""` (explicit empty) is valid — `Validate()` returns nil
- A `Config` with `TestCommand: "make test"` is valid — `Validate()` returns nil

### Suffix tests (`pkg/report/report_test.go`)

Add test cases:
- `TestCommandSuffix("make test")` contains `"make test"`
- `TestCommandSuffix("make test")` contains `"Fast Feedback"`
- `TestCommandSuffix("make test")` contains `"repeatedly"` (or `"Repeatedly"`) — confirms iteration wording
- `ValidationSuffix("make precommit")` contains `"once at the end"` (or `"ONCE at the end"`) — confirms updated wording

</requirements>

<constraints>
- The `validationCommand` field, its default (`make precommit`), and its role as the authoritative success/failure signal must not change
- The completion report format (markers, JSON structure) must not change
- Existing `.dark-factory.yaml` files without `testCommand` get default `"make test"` — backward compatible
- Do NOT add validation logic for `testCommand` in `Validate()` — empty string is the intentional opt-out, no error needed
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Follow existing error wrapping: `errors.Errorf(ctx, ...)` — never `fmt.Errorf`
</constraints>

<verification>
```bash
# Confirm new field in config
grep -n "TestCommand\|testCommand" pkg/config/config.go

# Confirm default is set
grep -n "make test" pkg/config/config.go

# Confirm new suffix function exists
grep -n "TestCommandSuffix" pkg/report/suffix.go

# Confirm ValidationSuffix updated wording
grep -n "once at the end\|ONCE at the end" pkg/report/suffix.go

make precommit
```
Must pass with no errors.
</verification>
