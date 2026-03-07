---
status: completed
spec: ["025"]
summary: Added ValidationCommand config field with 'make precommit' default, ValidationSuffix function, and processor injection that appends the validation command override to every prompt before execution
container: dark-factory-134-spec-025-prompt-validation-command
dark-factory-version: v0.26.0
created: "2026-03-07T21:00:00Z"
queued: "2026-03-07T21:35:07Z"
started: "2026-03-07T21:35:58Z"
completed: "2026-03-07T21:45:31Z"
---
<summary>
- Projects can configure a single validation command that applies to all prompts (default: `make precommit`)
- The configured command is injected into every prompt before execution
- The agent uses the command's exit code as the authoritative success/failure signal
- Injected validation explicitly overrides any prompt-level `<verification>` section
- Empty config disables injection — prompts fall back to their own verification
- Existing projects get `make precommit` automatically with no config changes
- Tests cover the config default, injection logic, and empty-string opt-out
</summary>

<objective>
Inject a project-level validation command into every prompt before it is sent to the YOLO container, making `make precommit` (or any configured command) the single source of truth for success/failure. Prompt authors no longer need to remember which verification commands work inside the container.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/config/config.go` — `Config` struct and `Defaults()` function where the new field and default live.
Read `pkg/report/suffix.go` — existing `Suffix()` and `ChangelogSuffix()` functions; `ValidationSuffix` follows the same pattern.
Read `pkg/processor/processor.go` — `processor` struct (line 38), `NewProcessor` constructor (line 62), and `processPrompt` (line 314). Injection happens at lines 346-351 alongside `report.Suffix()` and `report.ChangelogSuffix()`.
Read `pkg/factory/factory.go` — `CreateProcessor` function signature (line 163) and its call inside `CreateRunner` (line 105). Both need a new `validationCommand string` parameter.
Read `pkg/factory/factory_test.go` — existing `CreateProcessor` test at line 43; update it to pass the new parameter.
Read `pkg/config/config_test.go` — existing config tests; add a test asserting the default value.
Read `pkg/report/suffix_test.go` if it exists, or `pkg/report/report_test.go` — add tests for `ValidationSuffix`.
Read `pkg/processor/processor_test.go` — existing `NewProcessor` calls; update them and add a test verifying that a non-empty `validationCommand` causes the suffix to appear in the content passed to the executor.
</context>

<requirements>
1. **`pkg/config/config.go`** — Add field and default:
   - Add `ValidationCommand string \`yaml:"validationCommand"\`` to the `Config` struct, after `Model string`.
   - In `Defaults()`, set `ValidationCommand: "make precommit"`.
   - Do NOT add a validation rule for this field — empty string is valid (disables injection).

2. **`pkg/report/suffix.go`** — Add `ValidationSuffix`:
   ```go
   // ValidationSuffix returns the markdown text injected when a project-level validation command is configured.
   // It instructs the agent to treat the command's exit code as the authoritative success/failure signal,
   // overriding any <verification> section in the prompt.
   func ValidationSuffix(cmd string) string {
       return "\n\n---\n\n## Project Validation Command (REQUIRED — overrides <verification> section)\n\nRun the following command as the authoritative validation step and use its exit code in the completion report:\n\n```\n" + cmd + "\n```\n\nThis overrides any `<verification>` section in this prompt. Report `\"status\":\"success\"` if and only if this command exits 0.\n"
   }
   ```

3. **`pkg/processor/processor.go`** — Wire in the field and injection:
   - Add `validationCommand string` field to the `processor` struct (after `specLister spec.Lister`, before `skippedPrompts`).
   - Add `validationCommand string` parameter to `NewProcessor` (after `specLister spec.Lister` — the current last parameter).
   - Wire the field in the constructor body: `validationCommand: validationCommand`.
   - In `processPrompt`, after the changelog suffix block (lines 349-351):
     ```go
     // Inject project-level validation command (overrides prompt-level <verification>)
     if p.validationCommand != "" {
         content = content + report.ValidationSuffix(p.validationCommand)
     }
     ```

4. **`pkg/factory/factory.go`** — Thread through factory:
   - Add `validationCommand string` parameter to `CreateProcessor` (after `autoReview bool`, before `specsInboxDir string` — or at the end of the existing parameters; keep consistent with what minimises churn).
   - Pass `validationCommand` to `processor.NewProcessor(...)` at the correct position.
   - In `CreateRunner`, pass `cfg.ValidationCommand` to `CreateProcessor` after `cfg.AutoReview`.

5. **Tests** — Add/update tests:

   a. `pkg/config/config_test.go`: Add a test asserting `Defaults().ValidationCommand == "make precommit"`.

   b. `pkg/report/suffix_test.go` (create if absent) or add to existing report test file:
      - Test that `ValidationSuffix("make precommit")` contains the command string and the override instruction.
      - Test that the returned string contains `"make precommit"`.

   c. `pkg/processor/processor_test.go`:
      - Update all existing `NewProcessor(...)` calls to include the new parameter. Pass `""` (empty string) to preserve current behaviour — keeps existing tests unchanged.
      - Add a new test: construct a processor with `validationCommand: "make precommit"`, invoke `processPrompt` (or capture the content passed to the executor mock), and assert that the content contains the `ValidationSuffix("make precommit")` text.

   d. `pkg/factory/factory_test.go`:
      - Update the `CreateProcessor` call at line 46 to pass the new parameter (e.g. `"make precommit"`).

6. Run `go generate ./pkg/...` only if counterfeiter annotations changed. No new interfaces are added by this prompt, so generation is likely unnecessary.

7. Remove any imports that become unused.
</requirements>

<constraints>
- Default value is `"make precommit"` — existing projects get this behaviour without config changes
- Existing prompt content injection (report suffix, changelog suffix) must continue to work unchanged; validation suffix is appended after them
- Empty string disables injection — do NOT inject anything when `validationCommand == ""`
- Prompt-level `<verification>` sections remain visible to the agent but are explicitly overridden by the injected text
- Do NOT modify the completion report format or how dark-factory interprets completion reports
- Do NOT modify the executor interface — injection happens in the processor before calling `executor.Execute`
- Existing tests must keep passing; only add the new parameter to `NewProcessor` calls (pass `""` in existing tests)
- Do NOT commit — dark-factory handles git
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
