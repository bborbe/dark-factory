---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-04-06T16:24:27Z"
generating: "2026-04-06T16:24:27Z"
prompted: "2026-04-06T16:32:51Z"
branch: dark-factory/test-command
---

## Summary

- Add a `testCommand` config field so YOLO agents can iterate fast with `make test` instead of running the full `make precommit` every time.
- Add a new test-command suffix that tells the agent to use the fast command during development and the full validation command only once at the end.
- Update validation-command suffix wording to clarify it is a final gate, not an iteration tool.

## Problem

YOLO agents currently run `make precommit` (build + test + lint + security) 5-10 times during a single prompt execution as they iterate on code changes. This wastes significant time because most iterations only need build + test feedback. There is no way to configure a lightweight feedback command separately from the authoritative validation gate.

## Goal

After this work:

- Projects can configure a fast feedback command (`testCommand`) alongside the existing full validation command (`validationCommand`).
- YOLO agents receive clear instructions distinguishing fast iteration feedback from final validation.

## Non-goals

- Changing the completion report format or markers.
- Modifying how `validationPrompt` (AI-judged criteria) works.
- Adding per-prompt command overrides.
- Changing the default value of `validationCommand`.
- Refactoring suffix strings to embedded markdown files (handled by separate prompt `embed-suffix-templates`).

## Desired Behavior

1. A new `testCommand` config field is available in `.dark-factory.yaml`. When omitted, it defaults to `make test`.
2. The YOLO agent prompt includes a new section instructing it to use `testCommand` for fast feedback after each code edit, running it frequently during development.
3. The YOLO agent prompt includes updated wording for the validation command section, clarifying it should be run exactly once at the end as the authoritative final check.
4. The ordering of appended suffixes is: completion report, changelog (if applicable), test command, validation command, validation prompt (if applicable).
5. Setting `testCommand` to empty string disables the test-command suffix entirely (same pattern as `validationCommand`).

## Dependencies

- The `embed-suffix-templates` prompt should be executed first, so new suffix text is added as embedded markdown files rather than Go string concatenation.

## Constraints

- The `validationCommand` field, its default (`make precommit`), and its role as the authoritative success/failure signal must not change.
- The completion report format (markers, JSON structure) must not change.
- Existing `.dark-factory.yaml` files without `testCommand` must work without modification (backward compatible via default).
- See `docs/configuration.md` for existing config field documentation patterns. The new field must be documented there.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `testCommand` set to empty string | No test-command suffix appended to prompt | Intentional opt-out, no recovery needed |
| Both `testCommand` and `validationCommand` empty | Agent gets no build/test instructions in suffix | User misconfiguration; agent still gets completion report suffix |
| `testCommand` exits non-zero during iteration | Agent sees failure output and iterates on fix — this is the intended fast-feedback loop | No recovery needed; agent treats it as build/test feedback |
| `testCommand` is a typo or missing binary | Agent sees "command not found" error on first run and may report it | User fixes config; agent still runs `validationCommand` at end |
| `testCommand` passes but `validationCommand` fails at end | Agent treats `validationCommand` result as authoritative — reports failure | Agent may iterate further or report the mismatch in completion report |

## Acceptance Criteria

- [ ] `testCommand` field exists in config with default `make test`
- [ ] YOLO prompts contain test-command instructions referencing the configured command
- [ ] YOLO prompts contain updated validation-command wording emphasizing "run once at end"
- [ ] Test-command suffix appears before validation-command suffix in prompt output
- [ ] Setting `testCommand: ""` suppresses the test-command suffix
- [ ] Existing configs without `testCommand` work unchanged (default applies)
- [ ] All existing tests pass

## Verification

```
make precommit
```

## Do-Nothing Option

Agents continue running `make precommit` 5-10 times per prompt, wasting build time on lint and security checks during iteration. Acceptable short-term but increasingly costly as prompt volume grows.
