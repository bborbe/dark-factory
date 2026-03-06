---
status: draft
---

# Native Spec Integration

## Problem

Dark-factory only knows about prompts. Specs live in `specs/` but the CLI has no awareness of them — no listing, no status, no approval workflow. Managing specs requires manual file editing and `grep` to understand progress. There is no link between specs and the prompts that implement them.

## Goal

Specs become first-class citizens alongside prompts. The CLI provides commands to list, inspect, and approve specs. Prompts link to their parent spec via a frontmatter field. Combined top-level views (`status`, `list`) show both specs and prompts together.

## Non-goals

- No automatic prompt generation from specs
- No enforcement that prompts must have a spec
- No spec execution (specs are documentation, prompts are executable)
- No backward compatibility for existing flat commands (`dark-factory approve` etc.)

## Desired Behavior

1. Two-level command structure: `dark-factory prompt <cmd>` and `dark-factory spec <cmd>` for targeted operations.
2. `dark-factory prompt list/status/approve/queue/requeue/retry` replace the current flat commands with identical behavior.
3. `dark-factory spec list` shows all specs with their status (draft/approved/prompted/completed).
4. `dark-factory spec status` shows a summary: counts by status.
5. `dark-factory spec approve <file>` sets `status: approved` in spec frontmatter.
6. `dark-factory status` shows combined output: prompt status followed by spec status.
7. `dark-factory list` shows combined output: prompt list followed by spec list.
8. Prompts link to their parent spec via a `spec` frontmatter field (e.g. `spec: 017`). This is optional — prompts without a spec field are valid.
9. When all prompts linked to a spec are completed, the spec is automatically marked `status: completed`.

## Constraints

- Spec directory defaults to `specs/` (configurable in `.dark-factory.yaml`)
- Spec frontmatter uses same YAML parsing as prompts
- Spec statuses follow the existing lifecycle: draft → approved → prompted → completed
- `dark-factory run` remains the default command and is unchanged
- Existing prompt behavior is unchanged — only the command routing changes

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| No `specs/` directory | `spec list` returns empty, no error | Create `specs/` when needed |
| Spec file without frontmatter | Shown with status "unknown" | Add frontmatter |
| Prompt references non-existent spec | Warning logged, prompt still executes | Fix `spec` field |
| `spec approve` on already-approved spec | No-op, no error | Expected |

## Acceptance Criteria

- [ ] `dark-factory prompt list` shows all prompts (replaces flat `list`)
- [ ] `dark-factory prompt status` shows prompt status summary (replaces flat `status`)
- [ ] `dark-factory prompt approve` moves prompt from inbox to queue (replaces flat `approve`)
- [ ] `dark-factory spec list` shows all specs with status
- [ ] `dark-factory spec status` shows spec count summary by status
- [ ] `dark-factory spec approve <file>` updates spec frontmatter to approved
- [ ] `dark-factory status` = combined prompt status + spec status
- [ ] `dark-factory list` = combined prompt list + spec list
- [ ] Prompts support optional `spec` frontmatter field linking to parent spec
- [ ] Spec auto-completes when all linked prompts are completed
- [ ] Old flat commands removed (no backward compat)

## Verification

```
make precommit
```

## Do-Nothing Option

Continue managing specs manually. Works for small projects but becomes painful as spec count grows and the link between specs and their prompts is only in the developer's head.
