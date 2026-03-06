---
status: draft
---

# Auto-Prompt Generation from Spec

## Problem

Writing prompts for an approved spec is manual and slow. The human must context-switch into prompt-engineering mode, re-read the spec, decompose it, and write each prompt file. Dark-factory already has everything it needs to do this: the spec content, the Dark Factory Guide, and Claude Code. The `prompted` lifecycle state exists but nothing ever sets it.

## Goal

When a spec reaches `approved`, dark-factory automatically generates prompt files in the inbox and transitions the spec to `prompted`. The human then reviews and queues the prompts as before.

## Non-goals

- No automatic queuing of generated prompts — human gates that step
- No modification of the spec content
- No prompt generation for specs that are not `approved`

## Desired Behavior

1. Dark-factory detects specs with `status: approved` and generates prompts for them automatically.
2. Generation runs Claude Code in the project directory so it can read the spec, existing code, and the Dark Factory Guide for context.
3. Generated prompt files are written to `prompts/` (inbox) with `status: created` and `spec: ["NNN"]` in frontmatter.
4. After successful generation the spec transitions to `prompted`.
5. If generation fails or produces no files, the spec stays `approved`, an error is logged, and the cycle retries.
6. Only one spec is generated at a time.

## Constraints

- Generated prompts land in `prompts/` (inbox) only — never directly in `prompts/queue/`
- Existing `dark-factory spec approve` behavior is unchanged
- No new required config fields

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Claude Code produces no prompt files | Spec stays `approved`, retried next cycle | Improve spec quality |
| Generation crashes | Spec stays `approved`, error logged | Auto-retried |

## Acceptance Criteria

- [ ] Approving a spec triggers automatic prompt generation
- [ ] Generated prompts appear in `prompts/` with correct frontmatter
- [ ] Spec transitions to `prompted` after successful generation
- [ ] Failed generation leaves spec `approved` and retries
- [ ] `make precommit` passes

## Verification

```
dark-factory spec approve specs/020-auto-prompt-generation.md
# wait one poll cycle
dark-factory spec list   # 020 shows prompted
ls prompts/              # generated prompt files present
```

## Do-Nothing Option

Humans keep writing prompts manually. Works today. The cost is 10–30 minutes per spec of context-switching into prompt-engineering mode.
