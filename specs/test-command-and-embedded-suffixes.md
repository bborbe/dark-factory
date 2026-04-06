---
tags:
  - dark-factory
  - spec
status: draft
---

## Summary

- Add a `testCommand` config field so YOLO agents can iterate fast with `make test` instead of running the full `make precommit` every time.
- Suffix text becomes standalone markdown files instead of inline code, improving readability and maintainability.
- Add a new test-command suffix that tells the agent to use the fast command during development and the full validation command only once at the end.
- Update validation-command suffix wording to clarify it is a final gate, not an iteration tool.

## Problem

YOLO agents currently run `make precommit` (build + test + lint + security) 5-10 times during a single prompt execution as they iterate on code changes. This wastes significant time because most iterations only need build + test feedback. There is no way to configure a lightweight feedback command separately from the authoritative validation gate. Additionally, the suffix strings that instruct agents are built via Go string concatenation, making them hard to read, review, and maintain.

## Goal

After this work:

- Projects can configure a fast feedback command (`testCommand`) alongside the existing full validation command (`validationCommand`).
- YOLO agents receive clear instructions distinguishing fast iteration feedback from final validation.
- All agent-facing suffix text lives in standalone markdown files loaded via Go embed, making them readable and editable without touching Go code.

## Non-goals

- Changing the completion report format or markers.
- Modifying how `validationPrompt` (AI-judged criteria) works.
- Adding per-prompt command overrides.
- Changing the default value of `validationCommand`.

## Desired Behavior

1. A new `testCommand` config field is available in `.dark-factory.yaml`. When omitted, it defaults to `make test`.
2. The YOLO agent prompt includes a new section instructing it to use `testCommand` for fast feedback after each code edit, running it frequently during development.
3. The YOLO agent prompt includes updated wording for the validation command section, clarifying it should be run exactly once at the end as the authoritative final check.
4. The ordering of appended suffixes is: completion report, changelog (if applicable), test command, validation command, validation prompt (if applicable).
5. All suffix text (completion report, test command, validation command, validation prompt, changelog) is stored as individual standalone files — no multi-line string concatenation in code.
6. Dynamic values (command names, criteria text) are injected into templates at runtime using placeholder substitution.
7. Setting `testCommand` to empty string disables the test-command suffix entirely (same pattern as `validationCommand`).

## Constraints

- The `validationCommand` field, its default (`make precommit`), and its role as the authoritative success/failure signal must not change.
- The completion report format (markers, JSON structure) must not change.
- Existing `.dark-factory.yaml` files without `testCommand` must work without modification (backward compatible via default).
- Suffix text must be compiled into the binary — no runtime file reads from disk.
- See `docs/configuration.md` for existing config field documentation patterns. The new field must be documented there.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `testCommand` set to empty string | No test-command suffix appended to prompt | Intentional opt-out, no recovery needed |
| Both `testCommand` and `validationCommand` empty | Agent gets no build/test instructions in suffix | User misconfiguration; agent still gets completion report suffix |
| Template placeholder not found in markdown file | Build-time or test failure (template renders with raw placeholder) | Tests catch missing placeholders |
| Embedded file missing or empty | Compilation error (go:embed directive fails) | Fix file path in embed directive |

## Acceptance Criteria

- [ ] `testCommand` field exists in config with default `make test`
- [ ] YOLO prompts contain test-command instructions referencing the configured command
- [ ] YOLO prompts contain updated validation-command wording emphasizing "run once at end"
- [ ] Test-command suffix appears before validation-command suffix in prompt output
- [ ] All suffix text lives in standalone files, not in code string literals
- [ ] Setting `testCommand: ""` suppresses the test-command suffix
- [ ] Existing configs without `testCommand` work unchanged (default applies)
- [ ] All existing tests pass

## Verification

```
make precommit
```

## Do-Nothing Option

Agents continue running `make precommit` 5-10 times per prompt, wasting build time on lint and security checks during iteration. Suffix strings remain as Go string concatenation — functional but hard to review. Acceptable short-term but increasingly costly as prompt volume grows.
