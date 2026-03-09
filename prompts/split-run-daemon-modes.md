<summary>
- `dark-factory run` becomes one-shot: processes all queued prompts then exits cleanly
- New `dark-factory daemon` command keeps the current long-running watcher behavior
- One-shot mode is useful for CI, scripted scenarios, and when you want dark-factory to process and stop
- Daemon mode is the existing behavior — watches for file changes, runs indefinitely until killed
- Both modes share the same initialization (lock, reset, normalize) and processor logic
- CLI help text updated to describe both modes
</summary>

<objective>
Split `dark-factory run` into two modes: `run` (one-shot: drain queue and exit) and `daemon` (long-running watcher, current behavior). This makes scenario testing reliable (no need to kill the process) and enables scripted/CI usage.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `main.go` — focus on the `case "run"` switch and `ParseArgs` function.
Read `pkg/runner/runner.go` — the current `Run` method starts watcher + processor in parallel via `run.CancelOnFirstError`.
Read `pkg/processor/processor.go` — `Process` method has `processExistingQueued` (drain) followed by a `for/select` loop listening on `ready` channel and ticker.
Read `pkg/factory/factory.go` — `CreateRunner` wires everything together.
</context>

<requirements>
1. In `pkg/processor/processor.go`, extract the queue-draining logic into a separate exported method (e.g., `ProcessQueue(ctx) error`) that:
   - Calls `promptManager.ResetFailed`, the private `checkPromptedSpecs`, and `processExistingQueued` (the existing startup sequence from `Process` lines 119-131)
   - Returns after processing all queued prompts (does NOT enter the `for/select` loop)
   - Add `ProcessQueue` to the `Processor` interface

2. In `pkg/runner/runner.go`, add a `OneShotRunner` (interface → constructor → struct → method):
   - Acquire lock, reset executing, normalize filenames (same init as current `Run`)
   - Call `processor.ProcessQueue(ctx)` — process all queued prompts sequentially
   - After queue is drained, release lock and return nil
   - No watcher, no ticker, no `select` loop, no server, no review poller

3. In `pkg/factory/factory.go`, add `CreateOneShotRunner` that creates a one-shot runner with processor and locker only — no watcher, server, or review poller.

4. In `main.go`, add a new `case "daemon"` that calls the current `factory.CreateRunner(cfg, version.Version).Run(ctx)` — this preserves the existing long-running behavior under a new command name.

5. Modify `case "run"` in `main.go` to call the new one-shot runner: `factory.CreateOneShotRunner(cfg, version.Version).Run(ctx)`.

6. Update `ParseArgs` in `main.go` to recognize `"daemon"` as a valid command. Also update the default no-args behavior: when no command is given, `ParseArgs` currently returns `"run"` (line 167). Change the default to `"daemon"` so that running `dark-factory` with no arguments preserves the existing long-running behavior (avoids a silent breaking change).

7. Update `printHelp` in `main.go` to document both commands:
   - `run` — Process all queued prompts and exit
   - `daemon` — Watch for prompts and process continuously (long-running, default)

8. Add/update tests:
   - Test that `ParseArgs` recognizes `"daemon"` command
   - Test that `ParseArgs` with no args defaults to `"daemon"` (not `"run"`)
   - Test that `OneShotRunner` processes queued prompts and returns (doesn't block)
   - Existing runner tests should still pass
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `daemon` must behave exactly like current `run` (no behavior change for daemon mode)
- `run` must exit with code 0 after draining queue (not hang)
- `run` must exit with code 0 even if queue is empty (nothing to process)
- Follow existing code patterns (interface → constructor → struct → method)
- Use `github.com/bborbe/errors` for error wrapping
</constraints>

<verification>
Run `make precommit` -- must pass.
Run `make test` -- all tests must pass.
Verify `dark-factory help` shows both `run` and `daemon` descriptions.
</verification>
