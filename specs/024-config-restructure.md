---
status: prompted
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

Feature flags (`autoMerge`, `autoRelease`, `autoReview`) default to `false` and are omitted from the default config — only set them explicitly when enabling.

### Directory lifecycle

**Prompts** (unchanged behavior, new directory name):
| Status | Directory |
|--------|-----------|
| `created` | `prompts/` (inboxDir) |
| `queued`, `executing`, `failed`, `in_review` | `prompts/in-progress/` (inProgressDir) |
| `completed` | `prompts/completed/` (completedDir) |

**Specs** (new — file move is the signal, not frontmatter polling):
| Status | Directory |
|--------|-----------|
| `draft` | `specs/` (inboxDir) |
| `approved`, `prompted`, `verifying` | `specs/in-progress/` (inProgressDir) |
| `completed` | `specs/completed/` (completedDir) |

`dark-factory spec approve` sets `status: approved` **and** moves the file to `specs/in-progress/`. The SpecWatcher watches `specs/in-progress/` for new file Create events — no frontmatter polling needed. A file appearing in `in-progress/` is the unambiguous signal to generate prompts.

The prompt generator logs its output (the YOLO container run for spec prompt generation) to `specs.logDir`, mirroring how prompt execution logs to `prompts.logDir`.

### Number assignment

When dark-factory assigns the next prompt or spec number, it scans **all three directories** (`inboxDir`, `inProgressDir`, `completedDir`) to find the current highest number, then uses `+1`. Scanning only the inbox would miss files that have already been processed and moved — leading to number reuse.

### Command scope

| Command | Searches |
|---------|----------|
| `spec approve` | `specs.inboxDir` only — only draft specs live there |
| `spec verify` | all three dirs (inbox + in-progress + completed) |
| `spec list` | all three dirs — same as `prompt list` across all prompt dirs |

The flat `specDir` config field is removed; all spec commands switch to `cfg.Specs.InboxDir` / `cfg.Specs.InProgressDir` / `cfg.Specs.CompletedDir` as appropriate.

### Migration

- `prompts/queue/` → `prompts/in-progress/` (rename on startup if old path exists)
- All other directories created on startup if missing

## Constraints

- `make precommit` must pass
- Existing prompts in `prompts/queue/` must be picked up after migration
- Default values must match the new paths above

## Failure Modes

| Trigger | Expected behavior |
|---------|------------------|
| `in-progress/` dir missing | Create on startup |

## Acceptance Criteria

- [ ] Config struct has nested `PromptsConfig` and `SpecsConfig` with `InboxDir`, `InProgressDir`, `CompletedDir`, `LogDir`
- [ ] Factory, watcher, processor, specwatcher all use new config fields
- [ ] `spec approve` moves spec file from `inboxDir` to `inProgressDir` (file move = signal to generate)
- [ ] SpecWatcher watches `specs/inProgressDir` for Create events — no frontmatter polling
- [ ] Prompt generator logs to `specs.logDir`
- [ ] Spec files move to `completedDir` on `completed` transition
- [ ] `prompts/queue/` → `prompts/in-progress/` migration runs on startup
- [ ] All directories created on startup if missing
- [ ] Number assignment scans all three dirs (inbox + in-progress + completed) to find highest existing number
- [ ] `spec approve` searches only `specs.inboxDir`
- [ ] `spec verify` searches all three spec dirs
- [ ] `spec list` lists specs from all three dirs
- [ ] Flat `specDir` config field removed; all spec commands use nested fields
- [ ] `make precommit` passes

## Post-Implementation

Do these steps after `make install` succeeds with the new config format.

### 1. Update all `.dark-factory.yaml` configs

All projects slim down to a single line (defaults cover everything else):

```yaml
workflow: direct
```

Trading projects (`trading`, `trading-dev`, `trading-prod`):
```yaml
workflow: pr
```

Projects to update:
- `go-skeleton`, `pr-reviewer`, `dark-factory`, `claude-yolo`, `updater`, `time`, `vault-cli` → `workflow: direct`
- `trading`, `trading-dev`, `trading-prod` → `workflow: pr`

### 2. Move files to new directory structure

For each project:
- `prompts/queue/` → `prompts/in-progress/` (rename directory)
- Spec files: move based on current status
  - `draft` → stays in `specs/`
  - `approved`, `prompted`, `verifying` → move to `specs/in-progress/`
  - `completed` → move to `specs/completed/`

### 3. Fix frontmatter for all existing prompts and specs

- Prompts: ensure `status` field is present and correct for their directory
- Specs: ensure `status` field matches their directory location

## Do-Nothing Option

Keep flat config and `queueDir`. Specs stay in `specs/` regardless of status. No visual lifecycle for specs.
