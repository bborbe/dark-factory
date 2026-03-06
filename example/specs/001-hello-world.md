---
status: completed
---

# Hello World: Verify the Pipeline Works

## Problem

A new dark-factory project has no prompts and no verified setup. There is no way to know if the pipeline is working until something actually runs.

## Goal

A simple prompt executes successfully end-to-end: picked up from the queue, run inside the YOLO container, committed, and moved to completed.

## Non-goals

- No real code changes — this is a smoke test only
- No PR creation

## Desired Behavior

1. A prompt in the queue is picked up and executed.
2. The container runs and produces output.
3. The prompt is moved to `completed/` with `status: completed`.

## Constraints

- Workflow: `direct` (no PR needed for a smoke test)
- Prompt must be trivially simple so failure means pipeline failure, not task failure

## Acceptance Criteria

- [ ] Prompt appears in `completed/` after execution
- [ ] Log file created in `prompts/log/`

## Verification

```
ls prompts/completed/001-*.md
ls prompts/log/001-*.log
```

## Do-Nothing Option

Skip the smoke test and go straight to real work. Risk: first real failure is hard to diagnose without knowing if the pipeline itself works.
