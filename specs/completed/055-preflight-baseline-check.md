---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-04-19T11:36:25Z"
generating: "2026-04-19T11:36:37Z"
prompted: "2026-04-19T11:46:17Z"
verifying: "2026-04-19T17:37:34Z"
completed: "2026-04-19T17:37:43Z"
branch: dark-factory/preflight-baseline-check
---

## Summary

- Run the project's baseline check on a clean tree before each prompt executes, so prompts never start on an already-broken baseline.
- If the baseline is green, proceed as today. If red, abort before Claude starts, leave the prompt queued, and emit a clear "baseline broken" report.
- Result is cached per main-branch commit SHA so the check runs at most once per baseline state, not once per queued prompt.
- Configurable via `preflightCommand` (default: `make precommit`) and `preflightInterval` (default: 8h) in `.dark-factory.yaml`.
- Applies to all workflow modes. Does not attempt to auto-fix — user creates a follow-up prompt.

## Problem

Prompts regularly fail their end-of-run validation not because the code change is bad, but because the baseline was already broken — a transient CVE in an indirect dependency, a flaky test, or lint drift from an unrelated merge. Operators cannot tell from the failure report whether the prompt itself regressed something or inherited a broken tree, so they re-run prompts, investigate false positives, and waste container time on unstable baselines. A recent concrete example: vault-cli prompt 105 reported `status: partial` solely because `go-git v5.17.2` triggered `osv-scanner` on an unrelated CVE (GHSA-3xc5-wrhm-f963) — the actual code change was correct. The prompt should never have started on that baseline.

## Goal

Before a prompt is executed, the daemon has verified that the project's baseline validation passes on the clean tree. Prompts only start on a known-green baseline. When the baseline is broken, queued prompts remain queued, no container is spent, and the operator is told the infrastructure needs fixing first — not that their feature prompt failed.

## Assumptions

- The configured baseline command is meaningful to run on a clean main-branch tree (no pending prompt changes applied).
- Baseline state correlates strongly with the main-branch commit SHA — i.e., if the SHA hasn't changed and the check passed recently, it will still pass. Time-based dependencies (CVE databases refreshed externally, dated tokens) are handled by `preflightInterval`.
- The existing container image used for prompt execution is appropriate for running the baseline command (same toolchain, same dependencies).
- The `make precommit` equivalent default exists for most projects; those where it does not can disable preflight via empty string.

## Non-goals

- Auto-generating a baseline-fix prompt when preflight fails. Out of scope; possible future enhancement.
- Replacing per-prompt end-of-run validation. Prompts still run their own validation command — preflight is additive, not a substitute.
- Detecting and diagnosing the specific cause of baseline failure (e.g., CVE vs flaky test). The report surfaces the command output; humans triage.
- Per-prompt opt-out of preflight.
- Running preflight on every daemon tick regardless of SHA or queue state.
- Scheduling a baseline check proactively when the queue is empty.

## Desired Behavior

1. Before picking up a queued prompt for execution, the daemon runs the configured preflight command on a clean tree inside the same container environment used for prompt execution.

2. If the preflight command exits zero, the prompt proceeds through the normal execution flow unchanged.

3. If the preflight command exits non-zero, the daemon aborts before starting the prompt's Claude container. The prompt remains in its current queued state (no status change, no retry count increment, not counted as a prompt failure).

4. When preflight fails, the daemon emits a report and human-facing notification that clearly identify the failure as a baseline/infrastructure problem (not a prompt failure), include the command run and its captured output, and reference the main-branch commit SHA checked.

5. Preflight results are cached per project, keyed by main-branch commit SHA. Within `preflightInterval`, prompts against the same SHA reuse the cached green result and skip re-running. When the SHA advances (fetch/merge) or the interval expires, preflight re-runs. This ensures preflight runs at most once per baseline state per interval, not once per prompt.

6. Preflight is configurable via `.dark-factory.yaml`: `preflightCommand` (string; default `make precommit`; empty string disables) and `preflightInterval` (duration; default `8h`).

## Constraints

- Existing `.dark-factory.yaml` files without the new fields must continue working with defaults applied. Backward compatible.
- The per-prompt end-of-run `validationCommand` behavior, default, and semantics must not change.
- Preflight failure must NOT increment `retryCount` on any prompt, must NOT move prompts between directories, and must NOT mark prompts as `failed` or `permanently_failed`.
- Preflight runs on the clean main-branch tree before any workflow setup (branch switch, clone, worktree) has been started for the prompt.
- The preflight command runs in the same container image configured for prompt execution (`containerImage`), so toolchain and dependencies match what end-of-run validation will see. Projects without `containerImage` configured follow the same host/container fallback used today for `validationCommand`.
- Preflight applies to both PR and direct workflows (and worktree/branch variants) uniformly.
- See `docs/configuration.md` for existing config field conventions (duration strings, empty-string-disables pattern, defaults documented in table form). The new fields must be documented there.
- See `docs/architecture-flow.md` for the existing execution flow; preflight slots in before Step 4 ("Setup workflow") in Phase 2.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `preflightCommand` exits non-zero | Abort before prompt start; prompt stays queued; emit baseline-broken report with output; notify | Operator creates a fix prompt for the baseline issue |
| `preflightCommand` empty string | Preflight disabled; daemon behaves as today | Intentional opt-out |
| `preflightCommand` typo or missing binary | Treated as preflight failure; report surfaces the command-not-found output | Operator fixes config or environment |
| Preflight command hangs | Subject to the same container timeout handling as prompt execution; killed and treated as preflight failure | Operator investigates; may shorten command or disable preflight |
| Main-branch SHA advances between queued prompts | Cache entry for old SHA is ignored; preflight runs again for the new SHA | Automatic |
| `preflightInterval` elapses while SHA unchanged | Next queued prompt re-runs preflight for the same SHA | Automatic |
| Container image missing or pull fails during preflight | Treated as preflight failure; reported with clear error | Operator fixes image config |
| Daemon restarts | Cache is in-memory; next run re-checks baseline. Acceptable — baseline check is cheap relative to prompt execution and the restart case is rare | Automatic |
| `preflightInterval` invalid duration string | Rejected at daemon startup with a clear error (same pattern as `maxPromptDuration`) | Operator fixes config |

## Security / Abuse Cases

- `preflightCommand` is a shell command read from the project's own `.dark-factory.yaml`, which is already trusted input for this project (same trust boundary as `validationCommand`). No new trust boundary introduced.
- Preflight runs inside the existing YOLO container with the existing mount rules — no additional host exposure.
- Captured command output included in reports and notifications may contain sensitive strings (e.g., env var values printed by the build). Same exposure surface as existing validation output; no new mitigation required beyond what exists for `validationCommand` output.

## Acceptance Criteria

- [ ] `preflightCommand` and `preflightInterval` fields exist in `.dark-factory.yaml` with documented defaults.
- [ ] Configs without the new fields load unchanged and apply defaults.
- [ ] `preflightCommand: ""` disables preflight entirely.
- [ ] When preflight passes, the prompt proceeds through the normal flow.
- [ ] When preflight fails, no Claude container is started for the prompt.
- [ ] When preflight fails, the prompt's status, retry count, and directory are unchanged.
- [ ] When preflight fails, a baseline-failure report is emitted containing the command, captured output, and the main-branch SHA.
- [ ] Notification is emitted on preflight failure.
- [ ] Preflight runs at most once per main-branch commit SHA within `preflightInterval`, regardless of how many prompts are queued.
- [ ] When main-branch SHA changes, preflight re-runs.
- [ ] Invalid `preflightInterval` duration string is rejected at daemon startup with a clear error.
- [ ] Preflight applies uniformly to direct, worktree, branch, and PR workflows.
- [ ] `docs/configuration.md` documents the new fields.
- [ ] `docs/architecture-flow.md` describes where preflight fits in the execution flow.
- [ ] `make precommit` passes.

## Verification

```
make precommit
```

Scenario-level verification: run the daemon against a project whose baseline command is set to fail (e.g., `false`), queue a prompt, confirm the prompt remains queued and a baseline-broken report is produced without starting a Claude container. Then flip the command to succeed (e.g., `true`), confirm the prompt executes normally and preflight is not re-run for subsequent prompts within the configured `preflightInterval`.

## Do-Nothing Option

Operators continue to see prompt failures caused by pre-existing baseline issues, spend container time and Claude tokens on those failures, and must manually diagnose whether each failure is a real prompt regression or inherited breakage. Acceptable when prompt volume is low, increasingly costly as the daemon runs unattended across many projects. The vault-cli prompt 105 case (CVE in indirect dep caused a `partial` report) is a concrete recent example of the waste this spec prevents.
