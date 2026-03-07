---
status: ""
created: "2026-03-07T21:25:00Z"
---
<summary>
- Adds startup scan for specs stuck in `prompted` status with all linked prompts completed
- Transitions matching specs to `verifying` on daemon startup, same as the event-driven path
- Reuses existing `AutoCompleter.CheckAndComplete` logic — no duplication
- Fixes specs that missed the auto-transition because their prompts completed before the feature existed
</summary>

<objective>
The `prompted → verifying` auto-transition only fires when a prompt completes (event-driven). If the daemon restarts after all linked prompts already completed, specs stay stuck in `prompted` forever. Add a startup scan to catch these, matching the existing pattern where `ResetFailed` runs on startup.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/processor/processor.go` — `Process()` method at line 106: on startup it calls `ResetFailed` (line 110) then `processExistingQueued` (line 115). The new startup scan should go between these two, following the same pattern.
Read `pkg/spec/spec.go` — `AutoCompleter` interface with `CheckAndComplete(ctx, specID)` at line 178. This already handles the `prompted → verifying` transition. Reuse it.
Read `pkg/spec/lister.go` — `Lister` interface with `List(ctx)` returning `[]*SpecFile`. Use this to find all specs in `prompted` status.
Read `pkg/spec/spec.go` — `SpecFile` struct: `Name` field holds the spec name (without `.md`), `Frontmatter.Status` holds the status string. `StatusPrompted` constant.
</context>

<requirements>
1. Add a method to the `processor` struct (in `pkg/processor/processor.go`):
   - Name: `checkPromptedSpecs(ctx context.Context) error`
   - Uses the spec `Lister` (already available on the processor struct as `specLister`) to list all specs.
   - Filters for specs with `status == "prompted"`.
   - For each, calls `autoCompleter.CheckAndComplete(ctx, specName)` where `specName` is the spec's `Name` field.
   - Logs each transition: `slog.Info("startup: checking prompted spec", "spec", specName)`.

2. If the processor struct does not already have a `specLister` field, add one:
   - Add `specLister spec.Lister` to the processor struct.
   - Add it as a parameter to the constructor (`NewProcessor` or equivalent).
   - Wire it in `pkg/factory/factory.go`.

3. Call `checkPromptedSpecs` in `Process()` on startup, between `ResetFailed` (line 112) and `processExistingQueued` (line 115). Add a comment: `// Check prompted specs that may need verifying transition`.

4. Add a test in `pkg/processor/processor_test.go`:
   - Create a spec in `prompted` status with all linked prompts completed.
   - Start the processor.
   - Assert the spec transitions to `verifying`.

5. Remove any imports that become unused.
</requirements>

<constraints>
- Reuse `AutoCompleter.CheckAndComplete` — do NOT duplicate transition logic
- Do NOT change the event-driven path (post-prompt-completion check stays as-is)
- Do NOT modify spec status types or the AutoCompleter interface
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
