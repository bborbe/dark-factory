---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-04-02T08:59:14Z"
prompted: "2026-04-02T09:07:49Z"
verifying: "2026-04-02T09:47:54Z"
completed: "2026-04-02T10:26:31Z"
branch: dark-factory/additional-instructions
---

## Summary

- Add `additionalInstructions` config field to `.dark-factory.yaml` — a multiline text block
- Prepended to every prompt before execution and to every spec generation command
- Allows per-project instructions: reference shared docs, enforce conventions, provide context
- Empty or missing field means no injection — existing behavior unchanged

## Problem

With `extraMounts`, users can mount shared docs into the container. But the agent doesn't know they exist unless each prompt explicitly references them. Users must add "read /docs/..." to every prompt's `<context>` section manually. This is repetitive and easy to forget.

More broadly, there's no way to give the agent project-wide instructions that apply to all prompts without editing CLAUDE.md in the claude-yolo config (which affects all projects, not just one).

## Goal

After this work, users configure `additionalInstructions` in `.dark-factory.yaml` that is automatically prepended to every prompt. The agent sees it as part of the prompt content — no per-prompt boilerplate needed.

## Non-goals

- Per-prompt overrides (instructions apply uniformly to all prompts)
- Templating or variable substitution in the instructions text
- Conditional instructions based on prompt type or spec
- Modifying CLAUDE.md or system prompt — this is prompt-level injection only

## Desired Behavior

1. **Config field**: `.dark-factory.yaml` supports an optional `additionalInstructions` field (multiline string). When present, its content is prepended to every prompt before execution.

   ```yaml
   additionalInstructions: |
     Read shared documentation at /docs for coding guidelines.
     Follow conventions in /docs/go-testing-guide.md for all test code.
   ```

2. **Prompt execution**: When the processor assembles prompt content for execution, `additionalInstructions` appears before the prompt body and before any system-appended sections.

3. **Spec generation**: When the generator builds the command for spec-to-prompt generation, `additionalInstructions` is prepended before the generate command content.

4. **No-op when empty**: When `additionalInstructions` is empty or missing, prompt content is unchanged. No empty lines or whitespace injected.

## Constraints

- Existing prompt format unchanged — instructions are prepended, not injected into specific sections
- `.dark-factory.yaml` backward compatible — missing field means no injection
- No changes to CLAUDE.md handling or system prompt
- `additionalInstructions` is a plain string, not a list or structured type
- All existing tests must pass, `make precommit` passes

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `additionalInstructions` missing from config | No injection, existing behavior | N/A |
| `additionalInstructions` is empty string | No injection, no empty lines added | N/A |
| Very long instructions (thousands of lines) | Prepended as-is, may consume token budget | User shortens instructions |
| Instructions contain YAML special characters | Handled by YAML multiline syntax (`\|` or `>`) | User fixes YAML |
| Instructions reference nonexistent mount path | Agent sees the instruction but path doesn't exist in container | User fixes mount config |

## Security / Abuse Cases

- Instructions come from `.dark-factory.yaml` which is user-owned and trusted
- No new attack surface — same trust model as existing config fields
- Instructions cannot override CLAUDE.md or system-level constraints

## Acceptance Criteria

- [ ] `additionalInstructions` config field parsed from `.dark-factory.yaml`
- [ ] Instructions prepended to prompt content during execution
- [ ] Instructions prepended to spec generation command content
- [ ] Missing or empty field = no change to existing behavior
- [ ] `docs/configuration.md` updated with `additionalInstructions` field documentation
- [ ] All existing tests pass, `make precommit` passes

## Verification

```bash
make precommit
```

Manual verification:

1. Add `additionalInstructions` to `.dark-factory.yaml` with a distinctive message
2. Run a prompt, verify the instructions appear in the container's prompt content
3. Generate prompts from a spec, verify the instructions appear in the generation command
4. Remove `additionalInstructions`, verify existing behavior unchanged

## Assumptions

- Instructions are plain text, no Markdown rendering or processing by dark-factory
- Prepend order: additionalInstructions appears before all prompt content
- No length validation — user is responsible for keeping instructions reasonable

## Do-Nothing Option

Users add doc references to every prompt manually, or put instructions in CLAUDE.md (which applies globally to all projects). Neither is per-project or automatic. For a project with 20+ prompts, that is 20+ identical `<context>` blocks to maintain. Forgetting one means the agent misses conventions and produces inconsistent output.
