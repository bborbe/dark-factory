---
status: completed
---

# Directory Separation: Inbox, Queue, Completed, Logs

## Problem

All prompts live in one directory. No separation between drafts (human writing), active queue (factory processing), finished work (archive), and execution logs. This causes the factory to modify user drafts and makes it unclear which files are active.

## Goal

Four distinct directories for the prompt lifecycle. Each has a clear role and the factory respects boundaries — it never touches files outside its designated directories.

## Non-goals

- No automatic promotion from inbox to queue (human moves files manually)
- No automatic cleanup of completed directory
- No log rotation or size limits

## Desired Behavior

1. **Inbox** (`inboxDir`): Passive drop zone. Factory never reads, modifies, watches, or normalizes files here. Users write drafts at their own pace.
2. **Queue** (`queueDir`): Active processing zone. Factory watches for changes, normalizes filenames, and processes queued prompts.
3. **Completed** (`completedDir`): Archive. Finished prompts are moved here with updated frontmatter. Factory reads this directory to check predecessor completion and avoid number reuse.
4. **Logs** (`logDir`): Execution output. Each prompt execution writes stdout/stderr to `NNN-slug.log`.
5. All four paths are configurable in `.dark-factory.yaml`
6. Factory creates missing directories on startup
7. Factory refuses to start if any two directories overlap (same path)

## Constraints

- Existing prompt file format (frontmatter + markdown body) must not change
- `dark-factory queue` command moves files from inbox to queue
- File moves use `git mv` where possible for clean git history

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Directory missing | Created on startup automatically | None needed |
| Two directories configured to same path | Startup failure with clear error | Fix config |
| completedDir is subdirectory of queueDir | Allowed (common layout: `prompts/queue/` and `prompts/completed/`) | None needed |
| Permissions error creating directory | Startup failure with OS error | Fix permissions |

## Acceptance Criteria

- [ ] File placed in inbox is never modified by factory
- [ ] File moved to queue is normalized, processed, moved to completed
- [ ] Execution logs written to logDir as `NNN-slug.log`
- [ ] Config validation rejects overlapping directories
- [ ] Missing directories created on startup
- [ ] `dark-factory status` shows all four directories

## Verification

Run `make precommit` — must pass.

## Do-Nothing Option

Keep single directory. Users must be careful not to edit files while factory processes them. No separation between drafts and active queue. Confusing for any project with more than 5 prompts.
