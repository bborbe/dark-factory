---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-04-30T19:21:28Z"
generating: "2026-04-30T19:21:29Z"
prompted: "2026-04-30T19:27:13Z"
branch: dark-factory/skip-preflight-cli-flag
---

## Summary

- Add a per-invocation CLI flag that bypasses the preflight baseline check for `dark-factory run` and `dark-factory daemon`.
- When set, queued prompts proceed directly to execution without running the preflight command, consulting the cache, or producing baseline-failure reports.
- Intended for development iteration, urgent prompt execution against a knowingly broken baseline, and end-to-end testing of the runner.
- Does not change `.dark-factory.yaml` semantics (`preflightCommand` / `preflightInterval` continue to work as today). The flag is a one-shot override scoped to a single process lifetime.
- Default behavior is unchanged: preflight still runs unless the operator explicitly opts out for that invocation.

## Problem

Operators currently have no way to skip the preflight baseline check for a single invocation. When the baseline is known broken (e.g., transient CVE, upstream flake, mid-refactor) and the operator urgently needs a prompt to execute, the only options are: edit `.dark-factory.yaml` to set `preflightCommand: ""` and remember to revert, or fix the baseline first. Both are friction during development and incident response. Tests and demos that need to run prompts in a controlled, baseline-agnostic environment have the same problem.

## Goal

When the operator passes an explicit per-invocation skip flag to `run` or `daemon`, the daemon never executes preflight during that process, never reads or writes the preflight cache, and never produces baseline-failure reports. Prompts proceed straight to normal execution. When the flag is not passed, behavior is identical to today.

## Non-goals

- Persistent or per-prompt preflight opt-out. The flag affects only the current process; the next invocation without the flag runs preflight as configured.
- Changing `.dark-factory.yaml` schema. `preflightCommand` and `preflightInterval` keep their existing semantics.
- Skipping any other validation step (per-prompt `validationCommand`, scenario verification, etc.).
- Auto-detecting a broken baseline and skipping preflight automatically.
- A "skip once" mode that re-enables preflight on the next prompt within the same process.

## Desired Behavior

1. `dark-factory run --skip-preflight` and `dark-factory daemon --skip-preflight` are accepted by the CLI. The flag may appear in any position relative to other args, like the existing global flags.

2. When the skip flag is set, the runner does not invoke the preflight command for any prompt during the process lifetime. Prompts move directly into normal execution.

3. When the skip flag is set, no preflight cache reads or writes occur, no baseline-failure reports are produced, and no preflight-related notifications are emitted.

4. When the skip flag is not set, preflight behavior is identical to today: configured command runs, cache applies, failures abort prompts and produce reports.

5. The fact that preflight was skipped is visible in the daemon's log output at startup so operators reviewing logs can tell that this run bypassed baseline checking.

6. `--help` for both `run` and `daemon` documents the flag, when to use it, and the safety implications (prompts may run on a broken baseline).

7. The flag works regardless of the `preflightCommand` config value: it is a no-op when preflight is already disabled (`preflightCommand: ""`), and it overrides any non-empty configured command.

## Constraints

- Existing CLI behavior must not change for invocations that do not pass the flag.
- `.dark-factory.yaml` parsing, defaults, and validation are unchanged.
- The flag must be parsed in the same position-agnostic style as existing global flags (`--auto-approve`, `-debug`), and must not be persisted into `config.Config` since it is per-invocation.
- Preflight cache state from previous non-skip runs must not be corrupted by a skip run (the simplest correct behavior is "do not touch the cache at all when skipping").
- The flag applies uniformly to direct, worktree, branch, and PR workflows.
- Documentation lives where existing CLI flags are documented (`docs/configuration.md` mentions config-file fields today; CLI flag docs may belong there or in a CLI-flags section — follow existing convention).
- See `specs/completed/055-preflight-baseline-check.md` for the existing preflight contract this flag bypasses.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `--skip-preflight` passed with `preflightCommand: ""` already set | Flag is a no-op; daemon proceeds normally and logs that preflight is disabled by config | None needed |
| `--skip-preflight` passed and a prompt's own validation later fails because the baseline was broken | Prompt fails through normal end-of-run validation path; no preflight-specific report; failure is attributed to the prompt as today | Operator inspects validation output; this is the documented trade-off of the flag |
| Operator forgets they used `--skip-preflight` and a prompt regression slips through | Startup log line records that preflight was skipped, providing an audit trail | Operator reviews logs |
| Flag passed to a subcommand that does not support it (e.g., `dark-factory status --skip-preflight`) | Rejected with the same "unknown flag" error path used for any other unrecognized flag on `status` / `list` / `prompt` / `spec` / `scenario` | Operator removes the flag |
| Flag passed twice (`--skip-preflight --skip-preflight`) | Idempotent; same as passing once | None needed |

## Security / Abuse Cases

- The flag broadens the operator's ability to run prompts against a broken baseline. It does not introduce new code execution paths, network access, or trust boundaries — preflight already runs trusted commands from `.dark-factory.yaml`.
- An operator who can invoke `dark-factory` already has the ability to edit `.dark-factory.yaml` or run prompts directly, so the flag does not grant new privilege; it only reduces friction for an existing capability.
- The startup log line recording skipped preflight is the audit signal. No further mitigation required.

## Acceptance Criteria

- [ ] `dark-factory run --skip-preflight` is accepted and proceeds to execute queued prompts without invoking the preflight command.
- [ ] `dark-factory daemon --skip-preflight` is accepted and runs the daemon loop without invoking preflight for any prompt.
- [ ] Without the flag, preflight runs exactly as today (verified by an existing scenario or unit test for the preflight path still passing).
- [ ] When the flag is set, no preflight cache reads or writes occur, and no baseline-failure reports are emitted even if the configured command would have failed.
- [ ] Both `dark-factory --skip-preflight run` and `dark-factory run --skip-preflight` are accepted.
- [ ] A scenario in `scenarios/` exercises `dark-factory run --skip-preflight` against a project whose `preflightCommand` would fail (e.g., `false`), and asserts the prompt executes without producing a baseline-failure report.
- [ ] `--help` for `run` and `daemon` lists the flag with a one-line description and a note about the safety trade-off.
- [ ] CLI flag documentation in the `docs/` tree mentions `--skip-preflight` for `run` and `daemon`.
- [ ] Daemon/run startup log clearly indicates when preflight is skipped due to the flag.
- [ ] `make precommit` passes.

## Verification

```
make precommit
```

Manual scenario: configure a project with `preflightCommand: "false"` (guaranteed failure), queue a prompt. Without the flag: prompt remains queued and a baseline-failure report is produced (existing behavior). With `--skip-preflight`: the prompt executes through the normal flow and the daemon log records that preflight was skipped.

## Do-Nothing Option

Operators continue to edit `.dark-factory.yaml` (set `preflightCommand: ""` and revert) when they need to bypass preflight, or fix the baseline first. Acceptable when bypass is rare; friction grows the more often baseline issues are unrelated to the prompts queued. The cost of adding the flag is small and isolated to argument parsing plus a boolean plumbed to the runner factory, so the do-nothing option is dominated by the small implementation cost as soon as the bypass is needed more than occasionally.
