---
status: generating
approved: "2026-05-06T16:19:54Z"
generating: "2026-05-06T16:25:52Z"
branch: dark-factory/bug-cancelled-prompt-classified-as-failed-due-to-stop-then-close-race
---

# Cancelled prompt is classified as `failed` because watcher blocks on `StopAndRemoveContainer` before closing the cancel channel

## Summary

Spec 075 wired up the cancel flow for executing prompts: CLI writes `status: cancelled` → `cancellationwatcher` detects via fsnotify → stops the container → `processor.runContainer` returns `cancelled=true` → `ProcessPrompt` calls `MoveToCancelled`.

In practice the executing-cancel path classifies the prompt as `failed` instead of `cancelled`. The container DOES stop within 1 second (the watcher fires correctly), but the file ends up in `prompts/in-progress/` with `status: failed` and a `lastFailReason: 'execute prompt: docker run failed: wait command: exit status 143'`. `MoveToCancelled` is never called.

The cause: `cancellationwatcher` calls `StopAndRemoveContainer` BEFORE `close(ch)` — and `StopAndRemoveContainer` blocks until the container has actually exited. By the time the cancel channel closes, `executor.Execute` has already returned with the SIGTERM error, and `runContainer` reads `cancelledByUser=false`.

## Reproduction

dark-factory version: HEAD `d01b167` (built at `/tmp/new-dark-factory dark-factory dev`).

1. Sandbox project (`~/Documents/workspaces/dark-factory-sandbox`) with `workflow: direct, autoRelease: false`.
2. Drop a long-running prompt:
   ```yaml
   ---
   status: draft
   ---

   <summary>
   long-running prompt
   </summary>

   # Test
   Run `python3 -c "import time; time.sleep(180)"`.
   ```
3. `dark-factory prompt approve <name>`
4. `dark-factory daemon --skip-preflight` (background)
5. Wait until `docker ps` shows the container.
6. `dark-factory prompt cancel <id>` — CLI prints `cancelled: <name>.md`.
7. Container stops within 1 second (verified twice — 0.34s, 1.0s).
8. **Expected:** file in `prompts/cancelled/`, frontmatter `status: cancelled`.
9. **Actual:** file in `prompts/in-progress/`, frontmatter:
   ```yaml
   status: failed
   container: dark-factory-sandbox-006-spec-075-cancel-3
   started: "2026-05-06T14:10:17Z"
   completed: "2026-05-06T14:10:44Z"
   lastFailReason: 'execute prompt: docker run failed: wait command: exit status 143'
   cancelled: "2026-05-06T14:10:44Z"
   ```
10. The `cancelled:` timestamp survives (CLI's `MarkCancelled` did write it), but `status:` was overwritten by the processor's `MarkFailed` because `MarkCancelled` happened first then `MarkFailed` second when `runContainer` returned `cancelled=false, err=docker-exit-143`.

Daemon log shows the order:

```
16:10:44.542 prompt cancelled, stopping container
16:10:44.882 docker container exited with error  exit status 143
16:10:44.882 prompt failed
```

Reproduction is **deterministic** — observed on two consecutive cancel attempts, both with status=failed. Not a timing fluke.

## Expected vs Actual

**Expected** (per spec 075 AC2 + AC3 in `specs/in-progress/075-bug-prompt-cancel-leaves-running-container-and-file.md`):
- AC2: prompt file moved to `prompts/cancelled/`.
- AC3: frontmatter `status: cancelled`.

**Actual:**
- File stays in `prompts/in-progress/`.
- Frontmatter `status: failed` with `lastFailReason` quoting the SIGTERM exit code.
- `MoveToCancelled` is never invoked because `runContainer` returns `cancelled=false`.

## Why this is a bug

The contract spec 075 promised was: cancel an executing prompt, daemon classifies it as cancelled (not failed), file ends up in `cancelled/`. That contract is broken on every executing-cancel attempt. Spec 075 verification cannot pass without this fix.

Operators see a misleading `lastFailReason` ("docker run failed: wait command: exit status 143") which suggests their prompt's `make precommit` failed when actually the operator manually cancelled it. Triage of failed prompts becomes noisy.

## Code pointers

- `pkg/cancellationwatcher/watcher.go:91-107` — the watcher's fsnotify branch. Order today:
  1. Log `prompt cancelled, stopping container`
  2. `w.executor.StopAndRemoveContainer(ctx, containerName)` — **blocks until container exits** (line 104)
  3. `close(ch)` — **only runs after the container is already dead** (line 105)
  4. `return`

- `pkg/processor/processor.go:376-420` — `runContainer`. The cancel goroutine reads `<-cancelledCh` and sets `cancelledByUser = true`. But by the time the channel closes (post-`StopAndRemoveContainer`), `executor.Execute` has already returned with `err=exit-143`. The main goroutine reads `cancelledByUser` — still `false` because the goroutine hasn't been scheduled yet.

- `pkg/processor/processor.go:402-408` — the post-Execute branch:
  ```go
  if cancelledByUser {
      return true, nil  // never reached in practice
  }
  if execErr != nil {
      return false, errors.Wrap(ctx, execErr, "execute prompt")  // taken every time
  }
  ```

## Workaround

After `dark-factory prompt cancel`, the operator must manually `mv prompts/in-progress/<file>.md prompts/cancelled/` and edit the frontmatter to set `status: cancelled` (replacing `failed`). The CLI's "successful" `cancelled:` output is misleading.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| Cancel an executing prompt | File moves to `cancelled/`, status `cancelled`, container stopped | Watcher closes channel BEFORE blocking on container stop, OR processor re-reads file after Execute returns and detects status=cancelled |
| Cancel an approved (queued, not yet running) prompt | Existing behavior — file moves to cancelled/ via CLI's direct path | No change needed (this path works already) |
| Cancel during commit phase | Best-effort (out of scope; covered by spec 075) | No change |
| Container hangs ignoring SIGTERM | Watcher's `StopAndRemoveContainer` should still send SIGKILL after grace period | Existing executor behavior preserved |
| Daemon shutdown via Ctrl-C while container running | `daemon shutting down, leaving container running` log line — existing path preserved | Out of scope |

## Acceptance Criteria

- [ ] After cancelling an executing prompt, the daemon log shows `prompt cancelled` (not `prompt failed`).
- [ ] After cancelling an executing prompt, the prompt file is in `prompts/cancelled/` (not `prompts/in-progress/`).
- [ ] After cancelling an executing prompt, frontmatter `status: cancelled` (not `status: failed`).
- [ ] After cancelling an executing prompt, no `lastFailReason` is set.
- [ ] Cancel-while-approved (not running) path is not regressed — existing behavior preserved.
- [ ] Container still stops within ~5 seconds of the cancel signal (existing AC1 from spec 075).
- [ ] CHANGELOG.md `## Unreleased` entry added.

## Verification

Per `docs/bug-workflow.md` §Verification, this is a runtime symptom — unit tests alone are not sufficient. The current implementation has unit tests passing (`processor_cancel_test.go`) yet the runtime behavior is wrong, which is exactly the case the doc warns about.

**Repro replay (must run after fix lands):**

```bash
cd ~/Documents/workspaces/dark-factory-sandbox
git checkout -- .dark-factory.yaml
echo "workflow: direct
autoRelease: false" > .dark-factory.yaml

cat > prompts/test-cancel-race.md << 'EOF'
---
status: draft
---

<summary>
test-cancel-race
</summary>

# Test
Run `python3 -c "import time; time.sleep(180)"`.
EOF

dark-factory prompt approve test-cancel-race
dark-factory daemon --skip-preflight &
DAEMON_PID=$!

# Wait for container
until docker ps --format '{{.Names}}' | grep -q test-cancel-race; do sleep 2; done

# Let it run for 10 seconds
sleep 10

# Cancel
PROMPT_ID=$(ls prompts/in-progress/ | grep test-cancel-race | sed 's/\.md$//')
dark-factory prompt cancel "$PROMPT_ID"

# Wait for container exit
while docker ps --format '{{.Names}}' | grep -q test-cancel-race; do sleep 1; done

# Wait briefly for processor to settle
sleep 5

# Expected: file in cancelled/, status: cancelled
ls prompts/cancelled/                                     # must contain the file
ls prompts/in-progress/                                   # must NOT contain it
grep '^status:' prompts/cancelled/*test-cancel-race*.md   # must say "status: cancelled"
grep -L 'lastFailReason' prompts/cancelled/*test-cancel-race*.md  # must have NO lastFailReason

# Daemon log must show "prompt cancelled" not "prompt failed"
grep -E 'prompt cancelled|prompt failed' .dark-factory.log

kill $DAEMON_PID 2>/dev/null
```

**Acceptable evidence for `verifying → completed`:**

| Evidence | Acceptable? |
|---|---|
| Daemon log shows `prompt cancelled` after the cancel signal | Yes |
| File in `cancelled/` with `status: cancelled` after runtime replay | Yes |
| Unit test asserting `runContainer` returns `cancelled=true` when watcher fires before Execute returns | Necessary but not sufficient |
| "All tests pass" without runtime replay | No |

## See also

- Spec 075 (`bug-prompt-cancel-leaves-running-container-and-file`) — established the cancel flow but missed this race in the watcher → processor handoff. This spec fixes the gap.
- `pkg/cancellationwatcher/watcher.go:91-107` — the watcher loop that needs reordering (`close(ch)` before `StopAndRemoveContainer`, OR `StopAndRemoveContainer` in a goroutine).
- `pkg/processor/processor.go:376-420` — the `runContainer` race surface; an alternative fix is to have the processor re-read the prompt file from disk after Execute returns and detect `status: cancelled` regardless of the channel timing.
- `docs/bug-workflow.md` §Verification — establishes the runtime-replay requirement that exposed this race in the first place (unit tests alone hid it).
- `github.com/bborbe/run` `CancelOnFirstFinish` / `CancelOnFirstError` (https://github.com/bborbe/run/blob/master/run_runner.go) — instead of the bespoke goroutine + `cancelledByUser` bool pattern, `runContainer` could express "wait for Execute to return" and "wait for cancel signal" as two peer functions. Whoever finishes first wins; the other gets a cancelled context. Eliminates the read-after-write race on the bool entirely.
- `github.com/bborbe/collection` `ChannelFnMap` (https://github.com/bborbe/collection/blob/master/collection_channel-fn-map.go) — producer/consumer pattern built on `run.CancelOnFirstErrorWait`. Useful if the cancellationwatcher should be modeled as a value-emitting producer (emits cancel events to a channel) and the processor consumes them — keeps the producer/consumer halves cleanly separated and lets `run` handle the cancellation propagation.
