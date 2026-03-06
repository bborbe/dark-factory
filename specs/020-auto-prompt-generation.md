---
status: draft
---

# Auto-Prompt Generation from Spec

## Problem

Writing prompts for an approved spec is manual and slow. The human must context-switch into prompt-engineering mode, re-read the spec, decompose it, and write each prompt file. Dark-factory already has everything it needs to do this: the spec content, the Dark Factory Guide, and Claude Code. The `prompted` lifecycle state exists but nothing ever sets it.

## Goal

When the human approves a spec, dark-factory detects it automatically, runs Claude Code to generate prompt files in the inbox, and transitions the spec to `prompted`. The human then reviews and queues the prompts as before.

## Non-goals

- No automatic queuing of generated prompts — human gates that step
- No modification of the spec content
- No multi-turn conversation or interactive refinement during generation

## Desired Behavior

1. Dark-factory watches the `specs/` directory using the same fsnotify mechanism as the queue watcher.
2. When a spec file changes and its status is `approved`, the spec watcher signals the generator.
3. Generation runs `claude /generate-prompts-for-spec <spec-file>` via the existing YOLO mechanism. The YOLO container mounts `~/.claude-yolo` as `/home/node/.claude`, so slash commands are resolved from `~/.claude-yolo/commands/` — not from the host's `~/.claude/commands/`.
4. The `/generate-prompts-for-spec` command (in `~/.claude-yolo/commands/`) reads the spec, reads the Dark Factory Guide for conventions, and writes one or more prompt files to `prompts/` (inbox).
5. Each generated prompt has `status: created` and `spec: ["NNN"]` in its frontmatter.
6. After successful generation the spec transitions from `approved` to `prompted`.
7. If generation fails or produces no prompt files, the spec stays `approved`, an error is logged, and the next file change retriggers.
8. Only one spec is generated at a time.

## Constraints

- The `/generate-prompts-for-spec` command must be non-interactive — no `AskUserQuestion`, no user prompts
- Generated prompts land in `prompts/` (inbox) only — never directly in `prompts/queue/`
- After writing the command file, it must be committed to both repos:
  - `~/.claude-yolo` (git remote: `claude-yolo-config`) — where the YOLO container reads commands from
  - `~/Documents/workspaces/claude-yolo` — the canonical source repo
- Existing `dark-factory spec approve` behavior is unchanged
- No new required config fields

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Claude Code produces no prompt files | Spec stays `approved`, retried next cycle | Improve spec quality |
| Generation crashes | Spec stays `approved`, error logged, auto-retried | — |

## Acceptance Criteria

- [ ] `~/.claude-yolo/commands/generate-prompts-for-spec.md` exists, is non-interactive, and committed to both repos
- [ ] Approving a spec triggers automatic prompt generation
- [ ] Generated prompts appear in `prompts/` with `spec: ["NNN"]` and `status: created`
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
