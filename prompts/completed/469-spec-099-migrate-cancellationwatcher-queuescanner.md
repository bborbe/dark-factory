---
status: completed
spec: [099-correlation-ids-structured-logging]
summary: Migrated pkg/cancellationwatcher and pkg/queuescanner to context-bound logger (log.From(ctx)); normalized all attr keys to snake_case/canonical (prompt_id, spec_id, queued_count, previous_status); added workflow_step=cancel on cancellation path; updated scanner test for spec_id key; added boundary test proving bound-logger propagation through cancellation watcher; make precommit exited 0.
container: dark-factory-exec-469-spec-099-migrate-cancellationwatcher-queuescanner
dark-factory-version: v0.183.0
created: "2026-06-26T05:42:49Z"
queued: "2026-06-26T06:13:15Z"
started: "2026-06-26T06:49:52Z"
completed: "2026-06-26T06:56:37Z"
branch: dark-factory/correlation-ids-structured-logging
---

<summary>

- Migrates the last two hot-path packages — the cancellation watcher and the queue scanner — to the context-bound logger.
- Every bare package-level log call in these two packages becomes a context-bound call, so the per-prompt correlation attributes flow into their log lines.
- The cancellation watcher already receives the per-prompt context downstream of the binding site, so its lines automatically inherit the correlation attributes.
- Normalizes attribute keys to snake_case and the canonical set: prompt-identity keys collapse to `prompt_id`, spec keys to `spec_id`, and informational keys (reason, missing, status, queued count) become snake_case.
- After this prompt every hot-path package is migrated, so the hot-path check (still warn mode) reports no offenders.
- Leaves the tree green: `make precommit` passes.

</summary>

<objective>
Migrate `pkg/cancellationwatcher` and `pkg/queuescanner` to the context-bound logger from prompt 1. Replace every bare `slog.Info/Warn/Error/Debug/InfoContext` with `log.From(ctx).X`, normalize attr keys to the canonical/snake_case convention, and collapse prompt-identity keys to `prompt_id` and spec keys to `spec_id`. After this prompt all six hot-path packages are migrated.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-logging-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/099-correlation-ids-structured-logging.md` — Desired Behavior item 3; Acceptance Criteria 4, 5, 6.

Read prompt 1's deliverables (MUST be on the tree — if `pkg/log/context.go` absent, STOP and report `Status: failed` with "pkg/log helpers not yet deployed (prompt 1)"):
- `/workspace/pkg/log/context.go` — `NewContext`, `From`.
- `/workspace/docs/rules/logging-conventions.md` — canonical key set, snake_case rule.

Read these source files END-TO-END before editing:
- `/workspace/pkg/cancellationwatcher/watcher.go` — `slog.*` calls. Enclosing function `watch(ctx, promptPath, containerName, ch)` HAS `ctx`. Calls:
  - `slog.Warn("failed to create cancel watcher", "error", err)` (~line 68)
  - `slog.Warn("failed to watch prompt file", "path", promptPath, "error", err)` (~line 74)
  - `slog.Debug("cancel watcher error", "error", err)` (~line 86)
  - `slog.Info("prompt cancelled, stopping container", "file", ..., "container", containerName)` (~line 99)
  NOTE: `watch` is launched as a goroutine from `Watch(ctx, ...)` which receives `execCtx` from `pkg/processor`'s `runContainer` (downstream of the `ProcessPrompt` bind). So the bound logger propagates — these lines WILL carry the per-prompt correlation attrs.
- `/workspace/pkg/queuescanner/scanner.go` — `slog.*` calls (both bare `slog.Info/Debug/Warn` AND `slog.InfoContext(ctx,...)`). Enclosing functions all have `ctx`: `ScanAndProcess(ctx)`, `processSingleQueued(ctx)`, `readSpecID(ctx,...)`, `shouldSkipPrompt(ctx,...)`, `logBlockedOnce(ctx,...)`, `autoSetQueuedStatus(ctx,...)`. Calls include:
  - `slog.Info("queue blocked: prompt pending verification")` (~line 124)
  - `slog.Debug("queue scan complete", "queuedCount", ...)` (~lines 168, 172)
  - `slog.Warn("scanner: file lock release failed", "path", ..., "error", ...)` (~line 270)
  - `slog.Info("lock acquired", "file", ...)` (~line 279)
  - `slog.Info("scanner: candidate no longer at path...", "file", ...)` (~line 289)
  - `slog.Info("scanner: candidate no longer in pre-execution status...", "file", ..., "status", ...)` (~line 298)
  - `slog.Info("found queued prompt", "file", ...)` (~line 314)
  - `slog.Info("watching for queued prompts", "dir", ...)` (~line 327)
  - `slog.InfoContext(ctx, "prompt blocked", "file", ..., "reason", ..., "spec", ..., "missing", ...)` (~line 409)
  - `slog.Debug("skipping previously-failed prompt (unchanged)", "file", ...)` (~line 372)
  - `slog.Warn("skipping prompt", "file", ..., "reason", ...)` (~line 384)
  - `slog.Info("auto-setting status to approved", "file", ..., "previousStatus", ...)` (~line 453)

VERIFIED FACTS (do not re-derive):
- `pkg/queuescanner.ScanAndProcess` is called from `pkg/processor`'s `Process`/`runReadyTick`/`runQueueTick`. Those run on the daemon ctx, NOT inside `ProcessPrompt`'s bound ctx — so the scanner's log lines emit via the `slog.Default()` fallback (no per-prompt correlation attrs) UNTIL `ProcessPrompt` is entered for a specific prompt. This is intended (spec Failure Mode "pre-bind lines"). The scanner's job is selecting the next prompt — its lines legitimately precede the bind. Mechanical replacement only; do NOT add a bind in the scanner.
- After this prompt, ALL SIX hot-path packages are migrated. `make hotpath-logcheck` (warn mode) reports zero offenders. Prompt 5 flips it to strict.

</context>

<requirements>

## 1. Migrate `pkg/cancellationwatcher/watcher.go`

Replace EVERY bare `slog.Warn/Debug/Info` in `watcher.go` with `log.From(ctx).X` (the `watch` function has `ctx`). Import `log "github.com/bborbe/dark-factory/pkg/log"`. Remove `"log/slog"` if it becomes unused.

Key normalization:
- `"path", promptPath` (prompt identity) -> `"prompt_id", filepath.Base(promptPath)` (collapse to the canonical identity key; use the basename to match the bound `prompt_id` value). NOTE: the `prompt cancelled, stopping container` line currently uses `"file", filepath.Base(promptPath)` — rename to `"prompt_id"`. Since this line runs downstream of the bind, the bound logger ALREADY carries `prompt_id`; you MAY drop the redundant per-call `prompt_id`/`file` attr. PREFER dropping the redundant attr and add `"workflow_step", "cancel"` to the `prompt cancelled, stopping container` line (spec Desired Behavior item 5 — cancel decision point).
- `"container"` -> keep (canonical).
- `"error"` -> keep.

## 2. Migrate `pkg/queuescanner/scanner.go`

Replace EVERY bare `slog.Info/Warn/Debug` AND `slog.InfoContext(ctx, ...)` with `log.From(ctx).X`:
- `slog.Info(msg, kv...)` -> `log.From(ctx).Info(msg, kv...)`
- `slog.InfoContext(ctx, msg, kv...)` -> `log.From(ctx).Info(msg, kv...)` (drop the `Context` variant)
- `slog.Warn/Debug` -> `log.From(ctx).Warn/Debug`

Import `log "github.com/bborbe/dark-factory/pkg/log"`. Remove `"log/slog"` if unused.

Key normalization (snake_case + canonical, spec AC 5/6):
- `"file"` (prompt basename identity) -> `"prompt_id"`.
- `"path"` (prompt identity) -> `"prompt_id"`.
- `"spec"` -> `"spec_id"`.
- `"dir"` -> keep (canonical).
- `"queuedCount"` -> `"queued_count"`.
- `"previousStatus"` -> `"previous_status"`.
- `"status"` -> `"status"` (keep — single word, informational).
- `"reason"` -> `"reason"` (keep — single word, informational).
- `"missing"` -> `"missing"` (keep — single word, informational).
- `"error"` -> keep.

These informational keys (`status`, `reason`, `missing`, `queued_count`, `previous_status`) are permitted because they carry non-correlation context AND are snake_case (per the `docs/rules/logging-conventions.md` clarification from prompt 2: "informational keys permitted if snake_case"). Confirm that clarification exists in the doc; if prompt 2 did not add it, add it now.

## 3. snake_case sweep across both packages

After steps 1-2, confirm no camelCase inline-kv attr key remains:
```
grep -rEn '"[a-z]+[A-Z][a-zA-Z]*",' pkg/cancellationwatcher/watcher.go pkg/queuescanner/scanner.go
```
Any remaining camelCase attr key is a miss — fix it.

## 4. Tests

4.1. Update any existing tests in these packages asserting on log keys (`grep -rn '"file"\|"path"\|"spec"\|"queuedCount"\|"previousStatus"' pkg/cancellationwatcher/*_test.go pkg/queuescanner/*_test.go`). Adjust to new keys.

4.2. Add or extend a boundary test that proves the bound logger propagates through the cancellation watcher: in `pkg/cancellationwatcher`, set `slog.SetDefault` to a captured-buffer handler (restore after), bind `log.NewContext(ctx, slog.Default().With("prompt_id", "test-042"))`, trigger the cancellation path (write `status: cancelled` to a temp prompt file the watcher is watching, or drive `watch` directly with a fake `PromptLoader` returning a cancelled status), and assert the emitted `prompt cancelled, stopping container` line carries `prompt_id=test-042` and `workflow_step=cancel`. Reuse existing fakes/mocks (`mocks.Executor`, the local `PromptLoader`). Keep it minimal.

## 5. CHANGELOG

Append to `## Unreleased` in `/workspace/CHANGELOG.md` ONE bullet:

```
- refactor: migrate pkg/cancellationwatcher and pkg/queuescanner to the context-bound logger (log.From(ctx)); normalize attr keys to snake_case/canonical (prompt_id, spec_id) and add workflow_step=cancel on cancellation; all six hot-path packages now migrated (spec 099 prompt 4)
```

</requirements>

<constraints>

- Each chunked migration prompt leaves the tree `make precommit`-green (spec Constraint). `make precommit` MUST exit 0 after this prompt.
- After this prompt `make hotpath-logcheck` (still warn mode) MUST report ZERO offenders across all six hot-path packages. Prompt 5 flips it to strict + wires into precommit — do NOT do that here.
- Do NOT add a per-prompt logger bind in `pkg/queuescanner` (its lines precede the `ProcessPrompt` bind — fallback is intended; spec Failure Mode "pre-bind lines"). The cancellation watcher needs no bind either — it inherits the bind via the downstream `ctx`.
- No new dependencies (spec Constraint).
- Errors wrapped with `bborbe/errors` — never `fmt.Errorf`, never `context.Background()` in pkg/ non-test code. Do NOT change error-wrapping.
- The default slog handler choice is unchanged (spec Constraint) — no `slog.SetDefault` in production code.
- Counterfeiter mocks regenerate cleanly: `go generate ./...` exit 0.
- BSD-style license header on every modified file must survive.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# AC 4 (partial) — no bare slog.Info/Warn/Error in the two migrated files
grep -rnE 'slog\.(Info|Warn|Error)\(' pkg/cancellationwatcher/watcher.go pkg/queuescanner/scanner.go
# expected: 0 lines

# no leftover InfoContext either
grep -nE 'slog\.(Info|Warn|Debug)Context\(' pkg/cancellationwatcher/watcher.go pkg/queuescanner/scanner.go
# expected: 0 lines

# hotpath-logcheck (warn) reports ZERO offenders across all six packages now
make hotpath-logcheck 2>&1 | grep -E 'pkg/(processor|executor|promptresumer|committingrecoverer|cancellationwatcher|queuescanner)/' || echo "all six clean"
# expected: "all six clean" (launch.go is excluded by the script and must not appear)

# AC 6 — no camelCase attr keys
grep -rEn '"[a-z]+[A-Z][a-zA-Z]*",' pkg/cancellationwatcher/watcher.go pkg/queuescanner/scanner.go || echo "no camelCase keys"
# expected: "no camelCase keys"

# build + tests
go generate ./... && go build ./...
go test ./pkg/cancellationwatcher/... ./pkg/queuescanner/... ./pkg/log/...
# expected: PASS

# CHANGELOG
grep -n 'spec 099 prompt 4' CHANGELOG.md
# expected: >= 1 line

# full precommit
make precommit
# expected: exit 0
```

</verification>
