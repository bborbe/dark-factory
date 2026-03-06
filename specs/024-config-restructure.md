---
status: draft
---

# Config Restructure: Nested Prompts and Specs Sections

## Problem

The current config is flat, mixing prompt and spec paths with no clear ownership. `queueDir` is a poor name for what the factory actively monitors. Specs have no directory lifecycle — they stay in `specs/` regardless of status, making it hard to see at a glance what is in flight.

## Goal

Restructure `.dark-factory.yaml` into `prompts` and `specs` subsections. Both follow the same three-directory lifecycle: `inbox` → `in-progress` → `completed`. Rename `queueDir` to `inProgressDir`. Move spec files between directories as their status changes.

## Non-goals

- No change to prompt or spec frontmatter fields
- No change to the status values themselves
- No change to YOLO container or slash commands

## Desired Behavior

### New config format

```yaml
workflow: direct
prompts:
    inboxDir: prompts
    inProgressDir: prompts/in-progress
    completedDir: prompts/completed
    logDir: prompts/log
specs:
    inboxDir: specs
    inProgressDir: specs/in-progress
    completedDir: specs/completed
    logDir: specs/log
debounceMs: 500
serverPort: 0
```

### Directory lifecycle

**Prompts** (unchanged behavior, new directory name):
| Status | Directory |
|--------|-----------|
| `created` | `prompts/` (inboxDir) |
| `queued`, `executing`, `failed`, `in_review` | `prompts/in-progress/` (inProgressDir) |
| `completed` | `prompts/completed/` (completedDir) |

**Specs** (new — specs now move between directories):
| Status | Directory |
|--------|-----------|
| `draft`, `approved` | `specs/` (inboxDir) |
| `prompted`, `verifying` | `specs/in-progress/` (inProgressDir) |
| `completed` | `specs/completed/` (completedDir) |

### Migration

- `prompts/queue/` → `prompts/in-progress/` (rename on startup if old path exists)
- All other directories created on startup if missing
- Old flat config keys (`inboxDir`, `queueDir`, `completedDir`) supported as fallback with deprecation warning

## Constraints

- `make precommit` must pass
- Existing prompts in `prompts/queue/` must be picked up after migration
- Default values must match the new paths above

## Failure Modes

| Trigger | Expected behavior |
|---------|------------------|
| Old `queueDir` config key present | Log deprecation warning, use value as `prompts.inProgressDir` |
| `in-progress/` dir missing | Create on startup |

## Acceptance Criteria

- [ ] Config struct has nested `PromptsConfig` and `SpecsConfig` with `InboxDir`, `InProgressDir`, `CompletedDir`, `LogDir`
- [ ] Factory, watcher, processor, specwatcher all use new config fields
- [ ] Spec files move to `in-progress/` on `prompted` transition and `completed/` on `completed` transition
- [ ] `prompts/queue/` → `prompts/in-progress/` migration runs on startup
- [ ] All directories created on startup if missing
- [ ] Old flat config keys log deprecation warning and still work
- [ ] `make precommit` passes

## Do-Nothing Option

Keep flat config and `queueDir`. Specs stay in `specs/` regardless of status. No visual lifecycle for specs.
