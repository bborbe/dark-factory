---
status: created
spec: ["032"]
created: "2026-03-16T12:00:00Z"
---
<summary>
- AI-judged quality criteria are injected into every prompt at execution time when configured
- The criteria value is resolved: if it points to an existing file, the file content is loaded; otherwise the value is used directly as inline text
- A missing file logs a warning and skips evaluation — execution continues normally
- An empty config field means no injection — zero overhead
- Criteria evaluation is instructed to run after the validation command passes (build failure skips it)
- Unmet criteria produce partial status with each unmet criterion listed as a blocker
- Met criteria have no effect on the completion report status
- The new field is wired through the processor constructor and factory call sites
- Works alongside the validation command — both can be active simultaneously
- Tests cover file resolution, inline text, missing file warning, empty value, and processor injection
</summary>

<objective>
Wire `validationPrompt` into the prompt execution pipeline. After the `validationCommand` suffix
is injected, resolve the `validationPrompt` value and append a criteria-evaluation suffix so the
agent evaluates project-specific quality criteria and reports unmet ones as blockers.
This is the second of two prompts for spec 032. Prerequisite: spec-032 prompt 1 must be completed
(the `ValidationPrompt` config field and its path-safety validation must already exist).
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/report/suffix.go` — `Suffix()`, `ValidationSuffix()`, and `ChangelogSuffix()` are the
existing suffix functions. The new `ValidationPromptSuffix(criteria string) string` follows the
same pattern.
Read `pkg/processor/processor.go` — `processor` struct (line 93), `NewProcessor` constructor
(line 39), and `processPrompt` (line 382). The injection block at lines 420-423 appends
`report.ValidationSuffix` when `p.validationCommand != ""`. The new injection goes right after it.
Read `pkg/factory/factory.go` — `CreateProcessor` function (line 383) and its call sites in
`CreateRunner` (line 240) and `CreateOneShotRunner` (line 291). The new `validationPrompt string`
parameter must be added and threaded through both call sites using `cfg.ValidationPrompt`.
Read `pkg/config/config.go` — verify the `ValidationPrompt` field exists (added by prompt 1).
Read `pkg/processor/processor_test.go` — existing `NewProcessor(...)` calls; add the new parameter.
Read `pkg/factory/factory_test.go` — existing `CreateProcessor` calls; add the new parameter.
Read `pkg/report/suffix_test.go` (if it exists) or the report test file — add `ValidationPromptSuffix` tests.
</context>

<requirements>
1. **`pkg/report/suffix.go`** — Add `ValidationPromptSuffix`:
   ```go
   // ValidationPromptSuffix returns the markdown text injected when a project-level validation prompt is configured.
   // It instructs the agent to evaluate each criterion against its changes and report unmet criteria as blockers
   // with "partial" status. Evaluation runs only after validationCommand passes (if one is configured).
   func ValidationPromptSuffix(criteria string) string {
       return "\n\n---\n\n## Project Quality Criteria (AI-Judged)\n\nAfter all code changes are complete and `make precommit` (or the configured validation command) has passed, evaluate each of the following criteria against your changes:\n\n" + criteria + "\n\nFor each criterion:\n- If met: note it as passing.\n- If NOT met: add it to the `blockers` array in the completion report.\n\nIf any criterion is not met, set `\"status\":\"partial\"` in the completion report and list each unmet criterion as a separate entry in `blockers`. If all criteria are met, this section has no effect on the status — `\"success\"` stays `\"success\"`.\n"
   }
   ```

2. **`pkg/processor/processor.go`** — Add resolver helper function (unexported, package-level):
   ```go
   // resolveValidationPrompt resolves the validationPrompt config value.
   // If value is a relative path to an existing file, the file contents are returned.
   // If value is non-empty but the file does not exist, ("", false) is returned (caller logs warning).
   // If value is empty, ("", false) is returned silently.
   // The resolved result is the criteria text to inject, or empty string to skip injection.
   func resolveValidationPrompt(ctx context.Context, value string) (string, bool) {
       if value == "" {
           return "", false
       }
       // Check if value is a path to an existing file
       if _, err := os.Stat(value); err == nil {
           data, readErr := os.ReadFile(value)
           if readErr != nil {
               slog.WarnContext(ctx, "failed to read validationPrompt file", "path", value, "error", readErr)
               return "", false
           }
           return string(data), true
       }
       // Check if value looks like a file path (contains path separator or .md extension)
       // and the file doesn't exist — log a warning
       if strings.Contains(value, string(filepath.Separator)) || strings.HasSuffix(value, ".md") {
           slog.WarnContext(ctx, "validationPrompt file not found, skipping criteria evaluation", "path", value)
           return "", false
       }
       // Value is inline criteria text
       return value, true
   }
   ```
   Add `"os"` to imports if not already present (it is — check line 13).
   Note: `slog`, `os`, `strings`, `filepath` are already imported in `processor.go`.

3. **`pkg/processor/processor.go`** — Add field and parameter:
   - Add `validationPrompt string` field to the `processor` struct after `validationCommand string` (line 114).
   - Add `validationPrompt string` parameter to `NewProcessor` after `validationCommand string` (line 60).
   - Wire the field in the constructor body: `validationPrompt: validationPrompt`.
   - In `processPrompt`, after the `validationCommand` injection block (lines 420-423), add:
     ```go
     // Inject project-level validation prompt criteria (AI-judged, runs after validationCommand)
     if criteria, ok := resolveValidationPrompt(ctx, p.validationPrompt); ok {
         content = content + report.ValidationPromptSuffix(criteria)
     }
     ```

4. **`pkg/factory/factory.go`** — Thread through factory:
   - Add `validationPrompt string` parameter to `CreateProcessor` after `validationCommand string` (line 404).
   - Pass `validationPrompt` to `processor.NewProcessor(...)` at the correct position (after `validationCommand`).
   - In `CreateRunner` (line 240), pass `cfg.ValidationPrompt` after `cfg.ValidationCommand` in the `CreateProcessor(...)` call.
   - In `CreateOneShotRunner` (line 291), pass `cfg.ValidationPrompt` after `cfg.ValidationCommand` in the `CreateProcessor(...)` call.

5. **Tests** — Add/update tests:

   a. **`pkg/report/suffix_test.go`** (create if absent, otherwise add to existing):
      - Test that `ValidationPromptSuffix("readme.md is updated")` contains `"readme.md is updated"`.
      - Test that the returned string contains `"partial"` (instructs the agent to use partial status).
      - Test that the returned string contains `"blockers"`.

   b. **`pkg/processor/processor_test.go`**:
      - Update all existing `NewProcessor(...)` calls: add `""` (empty string) as the `validationPrompt` parameter immediately after the `validationCommand` parameter.
      - Add a new test: construct a processor with `validationPrompt: "readme.md is updated"` (inline),
        invoke `processPrompt` (or capture content passed to executor mock), and assert the content
        contains `"readme.md is updated"`.
      - Add a new test: construct a processor with `validationPrompt: "nonexistent-file.md"`. Since
        it ends in `.md` and doesn't exist, `resolveValidationPrompt` should log a warning and return
        `("", false)`. Assert the content does NOT contain `ValidationPromptSuffix` output.

   c. **`pkg/factory/factory_test.go`**:
      - Update existing `CreateProcessor(...)` calls to add the new `validationPrompt` parameter
        (pass `""` to preserve existing behavior).

6. Run `go generate ./pkg/...` only if counterfeiter annotations changed. No new interfaces are added,
   so generation is likely unnecessary.

7. Remove any imports that become unused.
</requirements>

<constraints>
- Config field name is `validationPrompt` — parallel to `validationCommand`
- Criteria evaluation suffix is appended AFTER `validationCommand` suffix (if both are set)
- If the value resolves to an existing file (relative to project root), load its content; otherwise use as inline text
- File detection heuristic: if the file doesn't exist AND the value contains a path separator or ends in `.md`, log a warning and skip injection (treat as a missing file, not inline text). If it doesn't match those patterns, treat as inline text
- Missing file → warning logged, execution continues without criteria injection (not an error)
- Empty config field → no injection, no log output, zero overhead
- `partial` status (not `failed`) when criteria are not met
- Do NOT attempt to stat or validate the file path at config validation time (that is already handled by prompt 1)
- Do NOT modify the completion report format or how dark-factory interprets completion reports
- Do NOT modify the executor interface
- Existing tests must keep passing; only add the new parameter to `NewProcessor` and `CreateProcessor` calls (pass `""` in existing tests)
- Do NOT commit — dark-factory handles git
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
