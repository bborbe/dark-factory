---
status: completed
approved: "2026-05-06T09:07:19Z"
generating: "2026-05-06T09:08:11Z"
prompted: "2026-05-06T09:20:37Z"
verifying: "2026-05-06T10:39:25Z"
completed: "2026-05-06T17:52:09Z"
branch: dark-factory/bug-prompt-cancel-leaves-running-container-and-file
---

# `prompt cancel` does not kill the running container nor move the file out of `in-progress/`, so the daemon respawns it

## Summary

When an operator cancels an actively-executing prompt with `dark-factory prompt cancel <id>`:

1. The yolo container running that prompt **keeps running** until the prompt finishes naturally. The CLI returns `cancelled: <name>.md` immediately but does nothing to the container.
2. The prompt file **stays in `prompts/in-progress/`**. The CLI updates frontmatter (status), but the file location is unchanged.
3. After the operator manually `docker stop`s the container, the daemon (still running `dark-factory run`/`daemon`) **re-picks up the file** from `in-progress/` and spawns a new container with the same prompt name.

Net effect: cancel is a no-op for in-flight prompts. Operators must `docker stop` repeatedly, and even that just resets the loop.

## Reproduction

dark-factory version: HEAD at v0.148.4-3-gc45254a (also reproducible on v0.149.5).

1. Sandbox project with a long-running queued prompt (e.g. one whose `make precommit` takes >2 minutes).
2. Approve and start `dark-factory run` (or `daemon`).
3. Wait until `docker ps` shows the container, e.g. `bolt-001-bro-19959-deprecated-bucket-not-found-alias`.
4. From another terminal in the same project root: `dark-factory prompt cancel 001-bro-19959-deprecated-bucket-not-found-alias`.
   ```
   cancelled: 001-bro-19959-deprecated-bucket-not-found-alias.md
   ```
5. Check `docker ps`: the container is **still running**.
6. Check `prompts/in-progress/`: the `.md` file is **still there**.
7. Manually `docker stop bolt-001-bro-19959-deprecated-bucket-not-found-alias`.
8. Within seconds the daemon log shows it picked the file up again and spawned a new identical container:
   ```
   level=INFO msg="found queued prompt" file=001-bro-19959-deprecated-bucket-not-found-alias.md
   level=INFO msg="executing prompt" title=001-bro-19959-deprecated-bucket-not-found-alias
   ```
9. `docker ps` now shows a new container with the same name (after the previous instance had been stopped).

## Expected vs Actual

**Expected:**
- `prompt cancel <id>` immediately stops the running yolo container for that prompt (graceful or `docker kill`).
- The prompt file moves from `prompts/in-progress/` to `prompts/cancelled/` with frontmatter `status: cancelled` and a `cancelled: <UTC timestamp>` field.
- The daemon does not re-spawn a container for that prompt.

**Actual:**
- Container keeps running. Operator must `docker stop <name>` manually.
- File stays in `in-progress/`. Daemon re-picks it up after operator stops the container.
- Operator is in a loop of "cancel → docker stop → cancel → docker stop" with no termination unless the prompt eventually completes naturally or the daemon is killed.

## Code pointers

- `pkg/cmd/cancel.go:42-72` — `cancelCommand.Run`. Lines 60-62 only call `pf.MarkCancelled()` + `pf.Save(ctx)`. No `ContainerStopper` injection, no file move, no daemon notification. The CLI returns success after just rewriting the frontmatter.
- `pkg/executor/stopper.go:14-21` — `ContainerStopper.StopContainer(ctx, name) error` already exists, ready to wire into `cancelCommand`.
- `pkg/queuescanner/scanner.go` — the loop that picks up `in-progress/*.md`. Should filter by frontmatter `status` (skip `cancelled`) AND/OR cancellation should move the file out so the scanner never sees it. Verify on HEAD which surface to fix.
- `pkg/cmd/cancel_test.go` — existing tests cover the status transitions but don't assert container stop or file move (since neither happens today).

## Why this is a bug

The CLI returns `cancelled: <name>` (positive confirmation) but the system state is unchanged: container running, file unmoved, daemon ready to respawn. The operator is misled into thinking cancellation succeeded.

This compounds with leftover-prompt scenarios: an old `in-progress/<old-prompt>.md` from a previous workflow gets executed when the operator runs `dark-factory run` for a new fix, and `prompt cancel` is the documented way to suppress it — but it doesn't work.

## Workaround

Manual three-step:

```bash
# 1. Mark cancelled (status only)
dark-factory prompt cancel <id>

# 2. Stop the running container so the slot is free
docker stop <project>-<NNN>-<slug>

# 3. Move the file out of in-progress/ so the daemon doesn't re-pick it up
mv prompts/in-progress/<NNN>-<slug>.md prompts/cancelled/   # or just delete it
```

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| `prompt cancel` while container running | Container stopped, file moved to `cancelled/`, status `cancelled` | Container `docker stop`/`kill` from cancel command; move file atomically; daemon ignores missing file |
| `prompt cancel` while prompt approved (queued, not yet running) | File moved to `cancelled/`, status `cancelled` | Same logic, no container to stop |
| `prompt cancel` during commit phase (post-container, pre-MoveToCompleted) | Best-effort: skip remaining steps, move file to `cancelled/` | Honour the cancel even if container already exited; do not commit/push |
| `prompt cancel` on a prompt that doesn't exist | Clear error message | Existing behaviour |
| Multiple operators cancel concurrently | Idempotent: first wins, second is a no-op | File-locking on the move; check status before acting |

## Acceptance Criteria

- [ ] `dark-factory prompt cancel <id>` on an executing prompt stops the running yolo container within ~5s (graceful, then SIGKILL if needed).
- [ ] After cancel, the prompt file is moved to `prompts/cancelled/` — no longer present in `in-progress/`.
- [ ] Cancelled prompt's frontmatter has `status: cancelled` and `cancelled: <UTC ISO8601 timestamp>`.
- [ ] The daemon does **not** spawn a new container for a cancelled prompt, even if it was previously running.
- [ ] `dark-factory prompt cancel` on a queued (not-yet-running) prompt works as today (no container) but ALSO moves the file out of `in-progress/`.
- [ ] CLI exit code is `0` on successful cancel; non-zero with clear message if the prompt id is unknown.
- [ ] CHANGELOG.md `## Unreleased` entry added.

## Constraints

- Do NOT regress `prompt approve` / `prompt requeue` / `prompt retry`.
- Do NOT change the directory layout or filenames beyond moving cancelled prompts to a terminal directory.
- The CLI MUST return after issuing the container-stop signal — do not block on the container's actual exit. Container teardown is best-effort from the CLI's perspective.
- The cancel must be idempotent: running it twice on the same id must not error or duplicate state changes.
- Project layouts that don't have `cancelled/` yet must auto-create it (mirrors `completed/` / `in-progress/` auto-create).

## Verification

This is a runtime symptom — unit tests alone are not sufficient.

**Repro replay (must run after fix lands):**

```bash
cd ~/Documents/workspaces/dark-factory-sandbox
# Approve a long-running prompt
dark-factory prompt approve slow-test
dark-factory run &
RUN_PID=$!

# Wait for container
until docker ps --format '{{.Names}}' | grep -q slow-test; do sleep 1; done

# Cancel
dark-factory prompt cancel slow-test

# Within 5s:
#   - docker ps must NOT show the container
#   - prompts/in-progress/ must NOT contain the file
#   - prompts/cancelled/ must contain it with status: cancelled
#
# AND the daemon must not respawn:
sleep 30
docker ps --format '{{.Names}}' | grep slow-test    # expect: no match
ls prompts/in-progress/                              # expect: no match
ls prompts/cancelled/                                 # expect: 001-slow-test.md
grep '^status:' prompts/cancelled/001-slow-test.md    # expect: status: cancelled

kill $RUN_PID 2>/dev/null
```

**Edge cases to verify:**

- Cancel a prompt that's still queued (no container yet) — file moves, no container action attempted.
- Cancel twice — second call is idempotent, exit 0.
- Cancel a non-existent id — exit non-zero with clear error.
- Cancel during commit phase — commit is skipped, file moves.

## See also

- Spec 071 (autoreview / postmerge actions) — orthogonal.
- Spec 072 (autoMerge takes precedence) — orthogonal.
- Surfaces when leftover `in-progress/*.md` files (from previous workflows or aborted runs) collide with new `dark-factory run` invocations.
