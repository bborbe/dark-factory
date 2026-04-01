---
status: idea
tags:
    - dark-factory
    - spec
---

## Summary

- Eliminate `bborbe/claude-yolo-config` as a standalone repo
- Install `bborbe/coding` as a Claude Code plugin inside `~/.claude-yolo/plugins/marketplaces/coding/`
- Move coding docs, agents, commands from `~/.claude-yolo/` root dirs into the coding plugin
- Move dark-factory-specific commands (`create-prompt.md`, `generate-prompts-for-spec.md`, `run-prompt.md`) to `bborbe/dark-factory/commands/`
- `~/.claude-yolo/` becomes a minimal container home (CLAUDE.md, settings.json, plugins/)
- Update all prompt context paths from `/home/node/.claude/docs/` to `/home/node/.claude/plugins/marketplaces/coding/docs/`

## Problem

Three repos serve overlapping purposes for dark-factory container configuration:
- `bborbe/claude-yolo-config` — docs, agents, commands, settings (mounted as container home)
- `bborbe/coding` — coding plugin with same docs/agents/commands duplicated or split
- `bborbe/dark-factory` — dark-factory-specific commands/agents

This causes confusion about where content lives, duplication risk, and manual sync between repos.

## Goal

Single source of truth: coding content lives in `bborbe/coding` (plugin), dark-factory content lives in `bborbe/dark-factory`, and `~/.claude-yolo/` is assembled from these sources rather than being a standalone repo.

## Migration Steps (rough)

1. Ensure `bborbe/coding` has all docs from `~/.claude-yolo/docs/`
2. Ensure `bborbe/coding` has all coding agents from `~/.claude-yolo/agents/`
3. Move dark-factory commands to `bborbe/dark-factory/commands/`
4. Install coding plugin: `cd ~/.claude-yolo && git clone bborbe/coding plugins/marketplaces/coding`
5. Configure `~/.claude-yolo/settings.json` to load the coding plugin
6. Delete `~/.claude-yolo/docs/`, `~/.claude-yolo/agents/`, `~/.claude-yolo/commands/`
7. Update dark-factory prompt generator to use new doc paths
8. Update `agents/prompt-auditor.md` doc paths
9. Test container execution with new layout
10. Archive or delete `bborbe/claude-yolo-config` repo

## Open Questions

- Does claude-yolo container auto-discover plugins from `plugins/marketplaces/`?
- Should `~/.claude-yolo/` remain a git repo (minimal) or become untracked local files?
- How to handle the 200+ completed prompts that reference old paths (leave as-is or bulk update)?
- Should docs be baked into the claude-yolo Docker image instead of host-mounted?
- Impact on other users of `bborbe/claude-yolo-config` if any?
