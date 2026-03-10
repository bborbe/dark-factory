---
status: created
---
<summary>
- `dark-factory run` (one-shot) now generates prompts from approved specs before draining the queue
- It loops until no approved specs and no queued prompts remain
- Makes `run` behave like `daemon` except it exits when idle instead of watching
</summary>

<objective>
Make the one-shot runner (`dark-factory run`) process both approved specs and queued prompts in a loop, so a single `run` invocation handles the full pipeline: generate prompts from specs, then execute them, repeating until nothing remains.
</objective>

<context>
Read CLAUDE.md for project conventions.

Current one-shot flow in `pkg/runner/oneshot.go`:
- `Run()` (line ~70): acquires lock, creates dirs, resets executing, normalizes filenames, calls `processor.ProcessQueue()`
- `ProcessQueue()` in `pkg/processor/processor.go` (line ~165): resets failed, checks prompted specs, calls `processExistingQueued()`, logs if empty, returns

Current spec generation in `pkg/specwatcher/watcher.go`:
- `scanExistingInProgress()` (line ~165): scans `specsInProgressDir` for `.md` files, calls `handleFileEvent()` for each
- `handleFileEvent()` (line ~132): loads spec, checks `status == approved`, calls `generator.Generate()` which creates prompts in inbox
- Generation only happens in daemon mode (`specWatcher.Watch()`), never in one-shot mode

The one-shot runner (`oneShotRunner`) does NOT have a `SpecGenerator` or `SpecWatcher` — it only has a `processor`. The daemon runner has the `specWatcher` which runs `scanExistingInProgress` on startup.

Factory wiring in `pkg/factory/factory.go`:
- `CreateOneShotRunner()` (line ~135): creates the one-shot runner without any spec generator
- `CreateSpecGenerator()` (line ~188): creates a `generator.SpecGenerator` — currently only used by the daemon runner
</context>

<requirements>
1. Add a `generator.SpecGenerator` parameter to `NewOneShotRunner()` and store it on the `oneShotRunner` struct.

2. Add a method `generateFromApprovedSpecs()` on `oneShotRunner` that scans `specsInProgressDir` for `.md` files with `status: approved`, and calls `generator.Generate()` for each. This is the same logic as `specwatcher.scanExistingInProgress()` but without the fsnotify watcher. After generation, call `promptManager.NormalizeFilenames()` on the inbox dir to ensure generated prompts are properly named, then use `promptManager.AutoApprove()` or the existing approve flow to move them to in-progress. Note: check how the daemon's spec watcher handles the approve step — the generator may already create prompts directly in the in-progress dir.

3. Change `oneShotRunner.Run()` to loop:
   ```
   loop:
     generateFromApprovedSpecs()
     processQueue()  // existing ProcessQueue logic
     if no specs were generated AND no prompts were processed:
       break
   ```
   The loop ensures that prompts generated from specs are picked up and executed in the same run.

4. Update `factory.CreateOneShotRunner()` to pass the `SpecGenerator` (reuse `CreateSpecGenerator()`).

5. Update existing tests to pass a `SpecGenerator` (nil or mock) to `NewOneShotRunner()`.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- The daemon (`dark-factory daemon`) behavior is unchanged
- When no spec generator is provided (nil), skip spec generation gracefully — do not panic
- The loop must terminate: if generation or processing produces no new work, exit
- `make precommit` must pass
</constraints>

<verification>
```bash
# No regressions
make precommit

# Verify one-shot generates from specs
grep -n "SpecGenerator\|specGenerator\|generateFrom" pkg/runner/oneshot.go
# Expected: new field and method present

# Verify factory passes generator
grep -n "SpecGenerator\|specGen" pkg/factory/factory.go | grep -i oneshot
# Expected: CreateOneShotRunner passes a SpecGenerator
```
Must pass with no errors.
</verification>

<success_criteria>
- `dark-factory run` generates prompts from approved specs
- `dark-factory run` executes generated prompts in the same invocation
- Loop terminates when no approved specs and no queued prompts remain
- Daemon behavior unchanged
- All existing tests pass
- `make precommit` passes
</success_criteria>
