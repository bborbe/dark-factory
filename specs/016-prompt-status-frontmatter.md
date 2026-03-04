---
tags:
  - dark-factory
  - spec
status: draft
---
Tags: [[Dark Factory Guide]]

---

## Problem

Prompt lifecycle tracking relies solely on folder location (inbox/queue/completed). This is too coarse — a prompt in `queue/` could be waiting for execution, already executed, or reviewed. There's no way to know without checking external state (PRs, logs).

## Goal

After completion, dark-factory reads and writes a `status` frontmatter field on each prompt, reflecting the last completed action. Folder moves still happen, but status provides fine-grained tracking within each folder.

## Non-goals

- Changing folder structure (inbox/queue/completed stays)
- Automated PR review status detection (human updates `reviewed`)
- Automated merge detection (human updates `merged` and moves to completed)
- Blocking execution based on previous prompt's status

## Desired Behavior

1. When dark-factory moves a prompt from inbox to queue, it sets `status: queued`
2. When dark-factory finishes executing a prompt, it sets `status: executed`
3. When dark-factory creates a PR for a prompt, it sets `status: executed` (PR creation is part of execution)
4. Dark-factory reads existing `status` field and only transitions forward (never overwrites a later status with an earlier one)
5. Prompts without a `status` field get one added on first transition
6. Human manually sets `status: reviewed` after PR review and `status: merged` after merge (then moves to completed)

## Constraints

- Existing frontmatter fields (`spec`, `tags`, etc.) must be preserved
- Status values are exactly: `created`, `queued`, `executed`, `reviewed`, `merged`
- Status transitions are forward-only: created → queued → executed → reviewed → merged
- Prompt files that already have other frontmatter must not lose it
- YAML frontmatter format must be preserved (not converted to JSON or other format)

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Prompt has no frontmatter | Add frontmatter with status | Automatic |
| Prompt has frontmatter but no status | Add status field | Automatic |
| Status is already ahead of transition | Skip update, log warning | Automatic |
| Malformed YAML frontmatter | Log error, skip status update, continue execution | Manual fix |

## Acceptance Criteria

- [ ] Prompt gets `status: queued` when moved to queue
- [ ] Prompt gets `status: executed` after dark-factory execution completes
- [ ] Existing frontmatter fields preserved during status update
- [ ] Forward-only transitions enforced
- [ ] Prompts without frontmatter get frontmatter added
- [ ] `make precommit` passes

## Verification

```
make precommit
```

## Do-Nothing Option

Keep folder-only tracking. Works but requires checking PR state externally to know prompt progress. Acceptable for small projects, annoying when multiple prompts are in queue.
