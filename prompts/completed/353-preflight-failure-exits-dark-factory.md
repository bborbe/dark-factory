---
status: completed
summary: Renamed ErrPreflightSkip to ErrPreflightFailed throughout codebase, updated scanner to propagate error terminating dark-factory instead of looping, added slog.Error messages in main.go for both daemon and run commands, updated all tests and docs to reflect new terminal contract
container: dark-factory-353-preflight-failure-exits-dark-factory
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T15:34:21Z"
queued: "2026-04-28T15:34:21Z"
started: "2026-04-28T16:21:13Z"
completed: "2026-04-28T16:25:27Z"
---

# Preflight failure exits dark-factory

<summary>
- When the preflight baseline check fails, dark-factory exits non-zero instead of looping or skipping.
- Applies to both daemon mode and one-shot `run` mode.
- The human fixes the tree, then explicitly restarts dark-factory.
- Removes the transient-green race where the daemon resumes execution mid-edit while a human is still fixing the baseline.
- Preserves the sole-writer invariant: while dark-factory runs, no other writer touches the tree.
</summary>

<objective>
Make preflight failure terminal. Today the daemon returns `ErrPreflightSkip` and waits for the next tick, which lets a transiently-clean tree (mid-fix) trigger prompt execution against files a human is still editing. After this change, a failed preflight stops dark-factory immediately with a clear cause message and a non-zero exit code.
</objective>

<context>
Read `CLAUDE.md` and `docs/architecture-flow.md` (section "Preflight Failure Policy") for the rationale.

Current behavior:

- `pkg/preflightconditions/conditions.go` — `ShouldSkip` returns `(false, ErrPreflightSkip)` when the baseline is broken. Sentinel: `var ErrPreflightSkip = stderrors.New("preflight baseline broken — skip cycle")`.
- `pkg/processor/processor.go` — `ProcessPrompt` propagates `ErrPreflightSkip` unwrapped to the caller. Re-exports the sentinel as `processor.ErrPreflightSkip`.
- `pkg/queuescanner/scanner.go` — caller catches the sentinel (around the `stderrors.Is(err, preflightconditions.ErrPreflightSkip)` branch) and returns `(true, nil)` to wait for the next 5s tick. This is the loop that must change.
- `pkg/factory/factory.go` — `CreateRunner` (daemon) and `CreateOneShotRunner` (one-shot `run`) both wire the same `preflightConditions` into the processor. Both runners must surface the failure as a non-zero process exit.
- `main.go` — invokes `factory.CreateRunner(...).Run(ctx)` and `factory.CreateOneShotRunner(...).Run(ctx)`. The returned error becomes the process exit code via the existing CLI error path.
</context>

<requirements>
1. Rename `ErrPreflightSkip` to `ErrPreflightFailed` in `pkg/preflightconditions/conditions.go`. The new contract is "preflight failed → dark-factory terminates", not "skip a cycle". Update all `stderrors.Is(...)` callers. Update the re-export in `pkg/processor/processor.go` (around line 37) so the public alias is `processor.ErrPreflightFailed` — there are no external dark-factory consumers, so no backwards-compatible alias is needed. Update the sentinel message to: `"preflight baseline broken — dark-factory exiting"`.

2. In `pkg/queuescanner/scanner.go`, where the scan loop currently does:

   ```go
   if stderrors.Is(err, preflightconditions.ErrPreflightSkip) {
       // Baseline is broken — exit scan loop and wait for next 5s tick.
       return true, nil
   }
   ```

   Replace with: propagate the error up so the runner stops. Do NOT swallow it. Keep the existing transient-skip path for git index lock and dirty files — those still return `(true, nil)` and the loop continues.

3. The runner returned by `CreateRunner` (daemon) and `CreateOneShotRunner` (one-shot `run`) in `pkg/factory/factory.go` must return a non-nil error when preflight fails so the CLI exits non-zero. The error must be `errors.Is`-comparable with `preflightconditions.ErrPreflightFailed` so callers and tests can recognize the cause.

4. In `main.go`, at the two sites where `factory.CreateRunner(...).Run(ctx)` and `factory.CreateOneShotRunner(...).Run(ctx)` are called (the existing `runDaemonCommand` function and the one-shot `run` command function), ensure that when the returned error matches `preflightconditions.ErrPreflightFailed`, a clear `slog.Error` is logged before returning the error. Suggested message:

   `preflight baseline broken — dark-factory exiting. Fix the tree (e.g. run the failing command manually), then restart dark-factory.`

   Use `slog.Error` consistent with the rest of the package. Return the error so the existing CLI error path produces the non-zero exit code.

5. Do NOT add any retry, backoff, or auto-restart. Do NOT add a "wait for tree to be clean" detector. Exit is terminal; the human restarts.

6. Update doc comments to reflect the new contract:
   - `ErrPreflightFailed` in `pkg/preflightconditions/conditions.go` — describe that this error causes dark-factory to exit, it does not skip a cycle.
   - The package-level comment in `pkg/preflightconditions/conditions.go` (the line near the top mentioning "Returns ErrPreflightSkip for the baseline-broken case") — update text and identifier.
   - `ShouldSkip`'s doc comment on the `Conditions` interface and on the implementation — update the `(false, ErrPreflightSkip)` reference to the new name and clarify the caller is expected to terminate, not skip.
   - The re-export comment in `pkg/processor/processor.go` — update to the new name.

7. Tests:
   - Update `pkg/preflightconditions/conditions_test.go` to assert the new sentinel and contract.
   - Update `pkg/queuescanner/scanner_test.go` (or equivalent test file in that package) to assert that a preflight-baseline-broken result causes the scanner to surface the error rather than returning `(true, nil)`.
   - Add a test in `pkg/factory/` (matching the existing factory test file pattern) that wires a preflight checker which always fails and asserts the runner returned by `CreateRunner` and the runner returned by `CreateOneShotRunner` both return an error matching `preflightconditions.ErrPreflightFailed` from `Run(ctx)`.
   - Existing tests for the transient-skip path (git index lock, dirty files) must still pass: those return `(true, nil)` and let the loop continue.
   - All tests use Ginkgo/Gomega in external test packages (`package <name>_test`), consistent with existing `*_test.go` files in each package.

8. Counterfeiter mocks under `mocks/` (in particular `mocks/preflight-conditions.go`) must regenerate cleanly via the existing `make` target. Do not edit generated mock files by hand.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Use `github.com/bborbe/errors` for error wrapping (`errors.Wrap(ctx, err, "...")`), consistent with the rest of the codebase.
- Preserve the existing transient-skip path semantics for git lock and dirty files — only the preflight-baseline case becomes terminal.
- Exit code must be non-zero — use the existing CLI error-return path; do not invent a special code or call `os.Exit` directly.
- Do not introduce new config flags. The behavior is unconditional: preflight failure → exit.
- Keep the change scoped — do not refactor unrelated code in `factory.go`, `processor.go`, `main.go`, or the scanner.
- Tests use Ginkgo/Gomega in external `_test` packages, consistent with existing files in each touched package.
</constraints>

<verification>
Run `make precommit` — must pass.

Manual smoke (optional, for the implementer):
1. Set `preflightCommand: false` (always-failing command) in a test project's `.dark-factory.yaml`.
2. Run `dark-factory daemon` (or `dark-factory run`).
3. Expected: process logs the "preflight baseline broken" message and exits non-zero within one preflight cycle. No prompt execution, no looping.
</verification>
