---
status: draft
---

## Problem

Every prompt must include a `<verification>` section telling the YOLO agent what command to run inside the container. This is repetitive — most projects use `make precommit` — and error-prone: prompt authors can specify commands that don't work in the container (e.g. `make build` requiring Docker). The completion report's success/failure depends entirely on what the prompt author remembered to include.

## Goal

Dark-factory injects a project-level validation command into every prompt before sending it to the YOLO container. The agent runs this command to determine success or failure. Prompts no longer need to specify their own validation command — the project config is the single source of truth.

## Non-goals

- No change to the host-side verification gate (separate feature)
- No change to the completion report format
- No change to how dark-factory interprets the completion report
- Prompt-level `<verification>` sections still allowed — they provide additional context to the agent

## Desired Behavior

1. New config field `validationCommand` (string, default: `make precommit`).
2. Dark-factory appends the validation command to the prompt content before passing it to the executor, following the same pattern as `report.Suffix()` and `report.ChangelogSuffix()`.
3. The injected text instructs the YOLO agent: "Run `<command>` as your final verification. Use its exit code to determine the completion report status. This overrides any `<verification>` section in the prompt."
4. If `validationCommand` is empty string, no validation command is injected (opt-out).

## Constraints

- Default value is `make precommit` — existing projects get this behavior without config changes
- Existing completion report suffix (`report.Suffix()`) format unchanged
- `make precommit` must pass

## Security

No security impact — validation command is set by the project owner in `.dark-factory.yaml`, not by external input.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Project has no `make precommit` target | Agent reports failure in completion report | Set `validationCommand` to a valid command or empty string |
| Validation command exits non-zero | Agent reports `status: partial` or `status: failed` | Fix the code or adjust the validation command |

## Acceptance Criteria

- [ ] `validationCommand` config field exists, defaults to `make precommit`
- [ ] Validation command is injected into prompt content before executor call
- [ ] Agent is instructed to run the command and base success/failure on its exit code
- [ ] Injected text explicitly overrides prompt-level `<verification>` sections
- [ ] Empty string disables injection
- [ ] Existing prompts with `<verification>` sections still work
- [ ] `make precommit` passes

## Verification

```
make precommit
```

## Do-Nothing Option

Keep current behavior. Every prompt must specify its own verification command. Prompt authors must remember what works inside the container. Risk of prompts failing due to impossible verification commands (e.g. `make build` needing Docker).
