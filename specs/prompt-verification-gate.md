---
status: draft
---

# Prompt Verification Gate

## Problem

When dark-factory executes a prompt inside a YOLO container, some verifications cannot run in the container. For example, projects that produce Docker images need `make build` or `docker run` to verify correctness, but Docker is not available inside the YOLO sandbox. Today, prompts either skip host-level verification (risking undetected failures) or include impossible verification commands (causing the prompt to fail with `status: partial`).

## Goal

After successful execution, prompts optionally pause in a `verifying` state before completing. The human runs host-side verification (e.g. `make build`), then explicitly marks the prompt as verified. This mirrors the existing spec verification gate (`dark-factory spec verify`).

## Non-goals

- No automated execution of host verification commands — human runs them manually
- No change to spec lifecycle
- No change to in-container verification (completion report still works as today)
- No new container capabilities (no Docker-in-Docker)

## Desired Behavior

1. New config field `prompts.verification` (boolean, default `false`).
2. When `prompts.verification: false` (default): behavior unchanged — successful prompts go directly to `completed`.
3. When `prompts.verification: true`: successful prompts transition to `verifying` instead of `completed`. Dark-factory does NOT commit/tag/push yet.
4. `dark-factory prompt list` shows `verifying` prompts — they require human attention.
5. `dark-factory prompt verify <file>` transitions the prompt from `verifying` to `completed`, then dark-factory commits/tags/pushes (same post-completion flow as today).
6. Running `prompt verify` on a prompt that is not `verifying` returns a clear error.
7. Queue blocks on `verifying` — next prompt does not start until the current one is verified (same blocking behavior as `failed`).
8. When a prompt enters `verifying`, dark-factory logs the prompt's `<verification>` section content as a hint to the human on what to check.

## Constraints

- Default behavior unchanged (`prompts.verification: false`)
- Existing `prompt approve`, `prompt requeue`, `prompt retry` commands unaffected
- `make precommit` must pass
- `StatusVerifying` added to the existing Status type in `pkg/prompt/`

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Human runs `prompt verify` on non-verifying prompt | Error: "prompt is not in verifying state" | Check `prompt list` for correct status |
| Human decides prompt failed verification | Leave in `verifying`, fix code manually, then `prompt verify` or `prompt requeue` | Manual |
| Dark-factory restarts while prompt is `verifying` | Prompt stays `verifying`, queue remains blocked | Human runs `prompt verify` or `prompt requeue` |

## Acceptance Criteria

- [ ] `prompts.verification` config field exists, defaults to `false`
- [ ] When `false`: no behavior change, prompts complete as today
- [ ] When `true`: successful execution → `verifying` (not `completed`)
- [ ] When `true`: dark-factory does not commit/tag/push until `prompt verify`
- [ ] `dark-factory prompt verify <file>` transitions `verifying` → `completed` and triggers commit/tag/push
- [ ] `prompt verify` on wrong state returns clear error
- [ ] Queue blocks on `verifying` prompt (next prompt waits)
- [ ] `dark-factory prompt list` shows `verifying` status
- [ ] Dark-factory logs the prompt's `<verification>` section when entering `verifying` state
- [ ] `make precommit` passes

## Verification

```
make precommit
```

## Do-Nothing Option

Keep current behavior. Prompts for projects needing host-level verification must use weaker in-container checks (like `grep`). Human must remember to run `make build` manually after dark-factory completes. No structured gate — easy to forget, no status tracking.
