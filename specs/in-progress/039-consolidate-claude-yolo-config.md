---
status: verifying
tags:
    - dark-factory
    - spec
---

## Summary

- Eliminate `bborbe/claude-yolo-config` as a standalone repo
- Install `bborbe/coding` as a Claude Code plugin inside `~/.claude-yolo/plugins/marketplaces/coding/`
- Install `bborbe/dark-factory` as a Claude Code plugin inside `~/.claude-yolo/plugins/marketplaces/dark-factory/`
- Move coding docs, agents, commands from `~/.claude-yolo/` root dirs into the coding plugin
- Move dark-factory-specific commands (`create-prompt.md`, `generate-prompts-for-spec.md`, `run-prompt.md`) to `bborbe/dark-factory/commands/`
- `~/.claude-yolo/` becomes a minimal container home (CLAUDE.md, plugins/)
- Update all prompt context paths from `/home/node/.claude/docs/` to `/home/node/.claude/plugins/marketplaces/coding/docs/`

## Problem

Three repos serve overlapping purposes for dark-factory container configuration:
- `bborbe/claude-yolo-config` — docs, agents, commands, settings (mounted as container home)
- `bborbe/coding` — coding plugin with same docs/agents/commands duplicated or split
- `bborbe/dark-factory` — dark-factory-specific commands/agents

This causes confusion about where content lives, duplication risk, and manual sync between repos.

## Goal

Single source of truth: coding content lives in `bborbe/coding` (plugin), dark-factory content lives in `bborbe/dark-factory`, and `~/.claude-yolo/` is assembled from these sources rather than being a standalone repo.

## Progress (2026-04-01)

### Completed

- [x] Move dark-factory commands (`generate-prompts-for-spec`, `run-prompt`) to `bborbe/dark-factory/commands/` (v0.85.0)
- [x] Install coding plugin: `git clone bborbe/coding ~/.claude-yolo/plugins/marketplaces/coding`
- [x] Install dark-factory plugin: `git clone bborbe/dark-factory ~/.claude-yolo/plugins/marketplaces/dark-factory`
- [x] Delete `~/.claude-yolo/commands/` (5 commands migrated to plugins)
- [x] Delete `~/.claude-yolo/docs/` (30 docs migrated to coding plugin)
- [x] Update CLAUDE.md: doc paths → `/home/node/.claude/plugins/marketplaces/coding/docs/`
- [x] Update CLAUDE.md: command namespaces → `/coding:*` and `/dark-factory:*`
- [x] Make generate command configurable via `generateCommand` config field (default `/dark-factory:generate-prompts-for-spec`)

### Remaining

- [x] Migrate `~/.claude-yolo/agents/` to coding plugin (deleted, all 16 agents exist in coding plugin)
- [x] Test container execution end-to-end with new layout (Max function prompt in dark-factory-sandbox, v1.1.0 released)
- [x] Verify plugin auto-discovery works in YOLO container (confirmed `/plugin` shows both plugins)
- [x] Decide: keep claude-yolo-config as minimal repo (CLAUDE.md, settings.json, plugins/)
- [x] Update `agents/prompt-auditor.md` doc paths in dark-factory (yolo docs → coding plugin docs)

## Decisions

- `~/.claude-yolo/` stays as minimal git repo (`bborbe/claude-yolo-config`) with CLAUDE.md, settings.json, plugins/
- 200+ completed prompts referencing old paths: leave as-is (historical)
