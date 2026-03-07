---
status: verifying
approved: "2026-03-07T20:55:40Z"
prompted: "2026-03-07T20:58:28Z"
verifying: "2026-03-07T21:45:31Z"
---

## Summary

- Every prompt currently needs its own verification command — repetitive and error-prone
- Projects can now configure a single validation command (default: `make precommit`) as the source of truth
- Dark-factory injects the command into every prompt before sending to the YOLO container
- The agent uses the command's exit code for success/failure, overriding prompt-level verification
- Empty config disables injection — prompts fall back to their own verification sections
- Existing projects get `make precommit` automatically with no config changes

## Problem

Every prompt must include a `<verification>` section telling the YOLO agent what command to run inside the container. This is repetitive — most projects use `make precommit` — and error-prone: prompt authors can specify commands that don't work in the container (e.g. `make build` requiring Docker). The completion report's success/failure depends entirely on what the prompt author remembered to include.

## Goal

Dark-factory injects a project-level validation command into every prompt before sending it to the YOLO container. The agent runs this command to determine success or failure. Prompts no longer need to specify their own validation command — the project config is the single source of truth.

## Non-goals

- No change to the host-side verification gate (separate feature)
- No change to the completion report format
- No change to how dark-factory interprets the completion report
- Prompt-level `<verification>` sections still allowed — they provide additional context to the agent

## Assumptions

- The YOLO container has the configured validation command available (e.g. `make precommit` target exists in the project's Makefile)
- Existing prompt content injection (report suffix, changelog suffix) continues to work alongside validation injection

## Desired Behavior

1. Projects can configure a single validation command in `.dark-factory.yaml` that applies to all prompts without each prompt specifying its own. Default: `make precommit`.
2. When a validation command is configured, dark-factory injects it into the prompt content before sending to the executor. The YOLO agent runs this command as the authoritative success/failure signal.
3. The injected instruction explicitly tells the agent to use the validation command's exit code for the completion report, overriding any prompt-level `<verification>` section.
4. Prompt-level `<verification>` sections remain visible to the agent as additional context but do not override the project-level command.
5. Setting `validationCommand` to empty string disables injection — prompts fall back to their own `<verification>` sections.

## Constraints

- Default value is `make precommit` — existing projects get this behavior without config changes
- Existing prompt content injection (report suffix, changelog suffix) must continue to work unchanged
- Prompts currently in the queue must work correctly after this ships — injection applies to all prompts regardless of when they were queued
- Existing executor and processor tests must keep passing
- `make precommit` must pass

## Security

No security impact — validation command is set by the project owner in `.dark-factory.yaml`, not by external input.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Project has no `make precommit` target | Agent reports failure in completion report | Set `validationCommand` to a valid command or empty string |
| Validation command exits non-zero | Agent reports `status: partial` or `status: failed` | Fix the code or adjust the validation command |
| Prompt has `<verification>` that contradicts injected command | Injected project-level command takes precedence; agent ignores prompt-level section | Remove or align the prompt-level section |

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
