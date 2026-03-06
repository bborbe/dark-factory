---
status: completed
---

# Configuration via YAML File

## Problem

All paths, image names, and settings are hardcoded. Different projects need different container images, directory layouts, and behavior. Changing settings requires recompiling.

## Goal

A `.dark-factory.yaml` file in the project root configures all runtime settings. Sane defaults mean the file is optional — dark-factory works out of the box.

## Non-goals

- No environment variable overrides
- No CLI flag overrides (config file only)
- No per-prompt config (all prompts share project config)
- No hot-reload (restart required for config changes)

## Desired Behavior

1. On startup, dark-factory looks for `.dark-factory.yaml` in the current directory
2. If found, values override defaults
3. If not found, all defaults apply (zero-config works)
4. Config fields:
   - `workflow`: execution mode (`direct` or `pr`)
   - `inboxDir`: where humans drop draft prompts
   - `queueDir`: where the factory watches and processes
   - `completedDir`: where finished prompts are archived
   - `logDir`: where execution logs are written
   - `containerImage`: Docker image to use
   - `debounceMs`: file watcher debounce interval
   - `serverPort`: HTTP API port (0 = disabled)
5. Validation runs on startup — invalid config prevents startup with clear error message

## Constraints

- Config format is YAML (not JSON, not TOML)
- File must be in project root (not configurable)
- All directory paths are relative to project root

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Config file missing | Use all defaults, no error | None needed |
| Invalid YAML syntax | Startup failure with parse error | Fix YAML |
| Invalid field value | Startup failure with validation error | Fix value |
| Unknown fields | Silently ignored (forward compatibility) | None needed |

## Acceptance Criteria

- [ ] Dark-factory starts without config file (defaults work)
- [ ] `.dark-factory.yaml` overrides individual fields
- [ ] `workflow` validated as "direct" or "pr"
- [ ] `debounceMs` validated as > 0
- [ ] `serverPort` validated as 0 or 1-65535
- [ ] Invalid config produces clear error message and non-zero exit

## Verification

Run `make precommit` — must pass.

## Do-Nothing Option

Keep hardcoded defaults. Works for one project but breaks when a second project needs different settings.
