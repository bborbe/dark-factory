---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-05-07T20:50:33Z"
generating: "2026-05-07T21:36:18Z"
prompted: "2026-05-07T21:46:45Z"
verifying: "2026-05-07T22:27:33Z"
completed: "2026-05-08T14:03:56Z"
branch: dark-factory/auto-approve-generated-prompts
---

## Summary

- Today the only manual gate left in the spec-to-execution pipeline is auditing and approving each prompt the daemon generates from an approved spec.
- This spec adds an opt-in setting that lets dark-factory audit and auto-approve those generated prompts on the user's behalf.
- The setting is off by default and is configured the same way `maxContainers` is configured (global, project, CLI), with the same effective-value reporting in the daemon log.
- A single audit pass per prompt — no fix loop, no retries.
- If a prompt's audit fails, dark-factory stops processing remaining prompts for that spec and surfaces the failure for human intervention.

## Problem

Once a spec is approved, dark-factory generates prompts automatically and executes them automatically — but the user must still manually audit and approve every generated prompt before execution can begin. For users who trust the spec-to-prompt pipeline (and have already invested effort in writing a high-quality spec), this manual step is the single remaining bottleneck preventing fully unattended overnight runs across many projects. The audit step is itself an existing slash command and the approve step is an existing CLI command — only the wiring between them is missing.

## Goal

Users who opt in can leave a project running unattended and have the daemon take an approved spec all the way through prompt generation, prompt audit, prompt approval, and execution without further input. Users who do not opt in see no behavior change: generated prompts continue to land in the inbox awaiting manual audit and approval.

## Non-goals

- Auto-approving specs themselves — specs remain a manual approve step.
- Auto-approving hand-written prompts that land directly in the prompts inbox without going through generate-prompts.
- Audit-fix iteration loops, retry logic, or prompt rewriting on audit failure — explicitly deferred.
- Alternative audit policies (e.g. partial pass, severity thresholds) — single binary pass/fail only.
- Cancelling or rolling back prompts that are already in the queue when this feature is toggled.

## Assumptions

- The existing `/dark-factory:audit-prompt` slash command is callable inside the YOLO container and produces a clear pass/fail signal that dark-factory can read.
- The existing in-YOLO slash-command invocation mechanism used by `generate-prompts` is reusable as-is for auditing — same launch path, same result-parsing path.
- The existing approve operation invoked by `dark-factory prompt approve <name>` is idempotent: calling it on an already-approved prompt is a no-op rather than an error (the manual-vs-auto race in Security / Abuse Cases relies on this).
- The `maxContainers` config-layering pattern (see `docs/config-layering.md`) — including the `*Source` reporting convention — is the established template and will not change for this spec's lifetime.

## Desired Behavior

1. **Opt-in setting.** A new setting `autoApprovePrompts` (boolean, default `false`) controls whether the daemon performs auto-approve for generated prompts. When `false`, the daemon's behavior is identical to today's behavior.

2. **Same precedence as `maxContainers`.** The setting is resolvable from global config, project-local config (`.dark-factory.yaml`), and a CLI argument, with the same precedence chain as `maxContainers` (CLI > project > global > default). See `docs/config-layering.md`.

3. **Effective-config reporting.** The daemon's startup effective-config log line emits the keys `autoApprovePrompts=<true|false>` and `autoApprovePromptsSource=<default|global|project|cli>` in the same shape as `maxContainers=N maxContainersSource=...`.

4. **Trigger only on generated prompts.** Auto-approve runs only on prompts produced by the existing generate-prompts flow for an approved spec. Prompts created by any other path (hand-written, dropped into the inbox, copied from another project) are never auto-approved.

5. **Audit before approve.** When enabled, after a prompt is generated for an approved spec, the daemon runs an audit on that prompt by invoking the existing `/dark-factory:audit-prompt` slash command inside the YOLO container, using the same mechanism `generate-prompts` already uses to invoke a slash command inside YOLO. No new invocation mechanism is introduced.

6. **Audit pass → auto-approve.** If the audit reports success, the daemon performs the same approve action that `dark-factory prompt approve <name>` performs today. The prompt is then eligible for execution by the existing executor.

7. **Audit fail → stop the spec.** If the audit reports failure for any prompt belonging to a spec, the daemon stops further audits and auto-approvals for that spec. Prompts already approved (whether by this feature or manually) continue to execute per existing executor behavior — this feature does not cancel the queue. The failure is surfaced via at least (a) a daemon log line naming the spec, the prompt, and the audit output, and (b) a status the user can see via `dark-factory status` / `dark-factory spec list`. Prompts from other specs are unaffected.

8. **Single audit pass per prompt.** No retries and no fix loop. A prompt either passes audit and is approved, or fails audit and blocks the spec.

## Constraints

- Default behavior (when `autoApprovePrompts` is unset) must be identical to today's behavior. No generated prompt may be auto-approved unless the user opts in.
- Auto-approve must reuse the existing slash-command-in-YOLO invocation mechanism used by generate-prompts. No second mechanism for invoking slash commands inside YOLO is acceptable.
- The approve action must be the same operation as `dark-factory prompt approve <name>` — same status transition, same file movement, same side effects. No alternate "auto-approved" status is introduced.
- Spec-level approval remains a manual step. This spec does not change the `dark-factory spec approve` flow.
- Hand-written prompts that did not come through generate-prompts must never trigger auto-approve, even when the setting is enabled.
- Config layering must follow the `maxContainers` pattern documented in `docs/config-layering.md` (precedence, source reporting, sentinel for unset booleans).
- Existing tests must continue to pass. `make precommit` must pass.
- The CLI flag name is `--auto-approve-prompts` (kebab-case derived from the YAML key, matching the `--max-containers` convention).

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `autoApprovePrompts` unset everywhere | Defaults to `false`; no auto-approve happens | Expected, no action |
| Audit slash command reports failure | Auto-approve is skipped for that prompt; daemon stops processing further generated prompts for the same spec; failure surfaced | User audits/fixes the prompt manually, then approves manually |
| Audit slash command crashes or times out inside YOLO | Treated as audit failure (fail-closed); same stop-the-spec behavior as a failed audit | User investigates and resumes manually |
| Setting enabled at global, disabled at project | Project value wins (precedence); auto-approve disabled in that project | Expected, no action |
| Daemon restarted mid-spec with the setting toggled | New value applies to generations started after restart; whatever was in-flight at restart is handled per the existing daemon-restart behavior (no new in-flight-config tracking introduced by this spec) | Expected, no action |
| Audit fails on prompt N of M for a spec | Prompts 1..N-1 already approved/executing continue; prompts N..M are not auto-approved; user intervenes | User fixes prompt N, approves manually, daemon resumes |
| `autoApprovePrompts` set to a non-boolean in YAML | Startup validation error with clear message | User fixes config |

## Security / Abuse Cases

- The setting is read from user-owned config files and CLI args — no untrusted input crosses a trust boundary.
- The audit slash command runs inside the same YOLO container the existing generate-prompts feature already uses; no new code path executes user-provided content outside that sandbox.
- A malformed prompt that somehow passes audit cannot escalate privileges beyond what manually-approved prompts already grant — auto-approve gates on audit success but does not bypass any existing execution-time guard.
- Auto-approve must not race with manual approve: if a prompt is already approved (manually) by the time auto-approve fires, the auto-approve is a no-op rather than an error.
- Audit must fail closed — any non-success result (failure, timeout, crash, malformed output) must be treated as a failed audit, never as a silent pass.

## Acceptance Criteria

- [ ] `autoApprovePrompts` is a boolean setting, default `false`.
- [ ] Setting is resolvable from global config, project config, and CLI argument with the same precedence as `maxContainers`.
- [ ] Daemon startup log includes the effective value and its source.
- [ ] When the setting is `false` (or unset), no generated prompt is auto-approved and behavior is unchanged from today.
- [ ] When the setting is `true`, generated prompts are audited via the existing `/dark-factory:audit-prompt` slash command using the same in-YOLO invocation mechanism as generate-prompts.
- [ ] Auto-approve uses the same approve operation as `dark-factory prompt approve <name>` (no new status, no new file lifecycle).
- [ ] Audit failure on a generated prompt blocks all remaining prompts for the same spec from auto-approve.
- [ ] Hand-written prompts (not produced by generate-prompts) are never auto-approved regardless of the setting.
- [ ] Audit timeout, crash, or non-success result is treated as a failure (fail-closed).
- [ ] If a prompt is manually approved before auto-approve fires, auto-approve is a no-op (no error, no double-approval).
- [ ] On audit failure for prompt N of a spec, prompts N+1..M from the same spec are not auto-approved; prompts 1..N-1 already approved continue executing.
- [ ] Existing tests pass.
- [ ] `make precommit` passes.

No new end-to-end scenario is justified: the setting is plumbed via the same layered-config path 200 other fields use (unit tests cover layering and source reporting), the audit invocation reuses the existing generate-prompts in-YOLO mechanism (already exercised by integration tests), and the approve operation is the same operation `dark-factory prompt approve` already calls (already covered). Integration tests at the daemon orchestration seam can verify the trigger/skip/stop-the-spec branches without standing up a full E2E run.

## Verification

```
make precommit
```

Manual verification:

1. With `autoApprovePrompts` unset, approve a spec and observe generated prompts land in the inbox awaiting manual approval (current behavior).
2. With `autoApprovePrompts: true` (project config), approve a spec and observe the daemon audit each generated prompt and auto-approve those that pass; check the daemon log for the audit invocation and approve action.
3. Force a generated prompt to fail audit (e.g. produce a deliberately broken prompt). Observe: that prompt is not approved, subsequent prompts for the same spec are not auto-approved, and the failure is visible to the user.
4. Set `autoApprovePrompts: true` globally and `autoApprovePrompts: false` in a project. Restart the daemon in that project. Observe the startup log reports `autoApprovePrompts=false autoApprovePromptsSource=project`.
5. Pass `--auto-approve-prompts=true` on the CLI and observe it overrides both global and project values, with the source reported as `cli`.

## Do-Nothing Option

Users continue to manually audit and approve each generated prompt. This is the current state and works correctly — the cost is purely the human attention required between prompt generation and prompt execution. For occasional users this cost is small; for users running many projects unattended (the same audience that motivated `maxContainers`), this cost is the dominant remaining friction in the pipeline. Doing nothing leaves the dark-factory pipeline a step short of fully autonomous execution from an approved spec.
