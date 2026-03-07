---
status: verifying
approved: "2026-03-07T21:37:13Z"
prompted: "2026-03-07T21:44:13Z"
verifying: "2026-03-07T22:54:09Z"
---

## Summary

- Adds an optional verification gate for prompts — successful execution pauses before completing
- Human runs host-side checks (e.g. `make build`, Docker tests) that can't run in the YOLO container
- New CLI command to mark a prompt as verified, triggering the normal commit/tag/push flow
- Queue blocks on pending-verification prompts — next prompt waits until human verifies
- Disabled by default — existing projects unaffected

## Problem

When dark-factory executes a prompt inside a YOLO container, some verifications cannot run in the container. For example, projects that produce Docker images need `make build` or `docker run` to verify correctness, but Docker is not available inside the YOLO sandbox. Today, prompts either skip host-level verification (risking undetected failures) or include impossible verification commands (causing the prompt to fail with `status: partial`).

## Goal

After successful execution, prompts optionally pause in a `verifying` state before completing. The human runs host-side verification (e.g. `make build`), then explicitly marks the prompt as verified. This mirrors the existing spec verification gate (`dark-factory spec verify`).

## Non-goals

- No automated execution of host verification commands — human runs them manually
- No change to spec lifecycle
- No change to in-container verification (completion report still works as today)
- No new container capabilities (no Docker-in-Docker)

## Assumptions

- A single human operator owns the verification gate (no multi-operator handoff needed)
- The YOLO container lacks host-level tooling (Docker, Kubernetes) by design — this is permanent, not temporary
- "Successful execution" means the agent's in-container checks passed (completion report `status: success`)

## Desired Behavior

1. The verification gate is opt-in and disabled by default — existing projects require no configuration change.
2. When disabled (default): behavior unchanged — successful prompts go directly to `completed`.
3. When enabled: successful prompts transition to a pending-verification state instead of `completed`. Dark-factory does NOT commit/tag/push yet.
4. `dark-factory prompt list` shows `pending_verification` prompts — they require human attention.
5. `dark-factory prompt verify <file>` transitions the prompt from `pending_verification` to `completed`, then dark-factory commits/tags/pushes (same post-completion flow as today).
6. Running `prompt verify` on a prompt that is not `pending_verification` returns a clear error.
7. Queue blocks on `pending_verification` — next prompt does not start until the current one is verified (same blocking behavior as `failed`).
8. When a prompt enters `pending_verification`, dark-factory logs the prompt's `<verification>` section content as a hint to the human on what to check.

## Constraints

- Default behavior unchanged — gate is off unless explicitly enabled
- Existing `prompt approve`, `prompt requeue`, `prompt retry` commands unaffected
- The prompt status model must be extended to represent a pending-verification state without breaking existing status transitions
- The prompt pending-verification state must be clearly distinct from the existing spec `verifying` state — different status string to avoid confusion (e.g. `pending_verification` vs spec's `verifying`)
- `make precommit` must pass

## Security

No external input surface — the verification gate is controlled by `.dark-factory.yaml` (project-owner only). The `prompt verify <file>` argument is resolved against known prompt directories only (same pattern as `prompt approve`), preventing path traversal. Concurrent `prompt verify` calls are safe — the first one transitions the state, the second gets an error.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Human runs `prompt verify` on non-verifying prompt | Error: "prompt is not in pending verification state" | Check `prompt list` for correct status |
| Host verification command exits non-zero (e.g. `make build` fails) | Prompt stays in pending-verification; human fixes code and retries | Fix code, then `prompt verify` or `prompt requeue` |
| Human decides prompt is fundamentally broken | Leave in pending-verification, then requeue | `prompt requeue` to re-execute |
| Dark-factory restarts while prompt is pending verification | Prompt stays in pending-verification, queue remains blocked | Human runs `prompt verify` or `prompt requeue` |

## Acceptance Criteria

- [ ] Verification gate config field exists, defaults to disabled
- [ ] When disabled: no behavior change, prompts complete as today
- [ ] When enabled: successful execution → pending-verification (not `completed`)
- [ ] When enabled: dark-factory does not commit/tag/push until `prompt verify`
- [ ] `dark-factory prompt verify <file>` transitions pending-verification → `completed` and triggers commit/tag/push
- [ ] `prompt verify` on wrong state returns clear error
- [ ] Queue blocks on pending-verification prompt (next prompt waits)
- [ ] `dark-factory prompt list` shows pending-verification status
- [ ] Dark-factory logs the prompt's `<verification>` section when entering pending-verification state
- [ ] Prompt pending-verification state uses a distinct status string from spec `verifying`
- [ ] `make precommit` passes

## Verification

```
make precommit
```

## Do-Nothing Option

Keep current behavior. Prompts for projects needing host-level verification must use weaker in-container checks (like `grep`). Human must remember to run `make build` manually after dark-factory completes. No structured gate — easy to forget, no status tracking.
