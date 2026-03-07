---
status: executing
container: dark-factory-132-fix-spec-verifying-on-startup
dark-factory-version: v0.25.0
created: "2026-03-07T21:25:00Z"
queued: "2026-03-07T20:28:43Z"
started: "2026-03-07T20:28:54Z"
---
<summary>
- Adds startup scan for specs stuck in `prompted` with all linked prompts already completed
- Transitions matching specs to `verifying` on daemon startup, catching missed auto-transitions
- Follows existing startup pattern: runs alongside `ResetFailed` and `processExistingQueued`
- Reuses existing spec-completion logic — no duplication of transition rules
- Adds a spec directory scanner to the processor for finding specs by status
</summary>

<objective>
The `prompted → verifying` auto-transition only fires when a prompt completes (event-driven). If the daemon restarts after all linked prompts already completed, specs stay stuck in `prompted` forever. Add a startup scan to catch these, matching the existing pattern where `ResetFailed` runs on startup.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/processor/processor.go` — `Process()` method at line 106: on startup it calls `ResetFailed` (line 110) then `processExistingQueued` (line 115). The new scan goes between these. The `processor` struct (line 38) already has `autoCompleter spec.AutoCompleter` (line 56) but no spec lister.
Read `pkg/spec/spec.go` — `AutoCompleter` interface at line 178, `CheckAndComplete(ctx, specID)` implementation at line 251. Handles `prompted → verifying` transition. Reuse this.
Read `pkg/spec/lister.go` — `Lister` interface with `List(ctx)` returning `[]*SpecFile`. Constructor: `spec.NewLister(dirs ...string)`.
Read `pkg/spec/spec.go` — `SpecFile.Name` field (spec name without `.md`), `Frontmatter.Status`, `StatusPrompted` constant.
Read `pkg/factory/factory.go` — `CreateProcessor` at line 162 already receives `specsInboxDir`, `specsInProgressDir`, `specsCompletedDir` (lines 179-181). Pass `spec.NewLister(specsInboxDir, specsInProgressDir, specsCompletedDir)` as the new `specLister` argument.
Read `pkg/processor/processor.go` — `NewProcessor` at line 61, constructor params end at line 79. Add `specLister spec.Lister` parameter here.
</context>

<requirements>
1. Add `specLister spec.Lister` field to the `processor` struct in `pkg/processor/processor.go` (after `autoCompleter` at line 56).

2. Add `specLister spec.Lister` parameter to `NewProcessor` (after `autoCompleter` at line 79). Wire it in the constructor body.

3. Add a private method to the processor:
   ```
   func (p *processor) checkPromptedSpecs(ctx context.Context) error
   ```
   - Calls `p.specLister.List(ctx)` to get all specs.
   - Filters for specs where `sf.Frontmatter.Status == string(spec.StatusPrompted)`.
   - For each, calls `p.autoCompleter.CheckAndComplete(ctx, sf.Name)`.
   - Logs: `slog.Info("startup: checking prompted spec", "spec", sf.Name)`.
   - Returns first error encountered, or nil.

4. Call `p.checkPromptedSpecs(ctx)` in `Process()`, after the `ResetFailed` block (lines 110-112) and before `processExistingQueued` (line 115). Add comment: `// Transition prompted specs with all prompts completed to verifying`.

5. Update `CreateProcessor` in `pkg/factory/factory.go`:
   - Add `spec.NewLister(specsInboxDir, specsInProgressDir, specsCompletedDir)` as the new argument in the `processor.NewProcessor(...)` call (after the `autoCompleter` argument at line 207).

6. Update test in `pkg/processor/processor_test.go`:
   - Add the new `specLister` argument to existing `NewProcessor` calls (use the counterfeiter mock `mocks.Lister` or a real `spec.NewLister` with test dirs).
   - Add a test case: create a spec file in `prompted` status, create a completed prompt linked to that spec, start the processor, assert the spec transitions to `verifying`.

7. Remove any imports that become unused.
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
