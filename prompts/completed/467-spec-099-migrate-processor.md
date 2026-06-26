---
status: completed
spec: [099-correlation-ids-structured-logging]
summary: 'Migrated pkg/processor to context-bound logger: bound per-prompt correlation attrs (prompt_id/spec_id/container/workflow_type) at ProcessPrompt entry, re-bind container on assignment, added workflow_step at key decision points, replaced all bare slog calls with log.From(ctx), normalized keys to snake_case, added bindPromptLogger helper with test, updated all NewProcessor call sites with workflowType param.'
container: dark-factory-exec-467-spec-099-migrate-processor
dark-factory-version: v0.183.0
created: "2026-06-26T05:42:49Z"
queued: "2026-06-26T06:13:15Z"
started: "2026-06-26T06:18:20Z"
completed: "2026-06-26T06:34:57Z"
branch: dark-factory/correlation-ids-structured-logging
---

<summary>

- Binds a per-prompt structured logger at the single entry point where a prompt starts being processed, carrying four stable attributes: the prompt's id, its spec id, its container name, and its workflow type.
- Every downstream log line emitted while that prompt is processed inherits those four attributes automatically — no log call has to restate them.
- When the container name is assigned, re-binds the logger so all later lines carry the real container name, and emits one "container assigned" line recording the transition.
- Replaces every bare package-level log call in `pkg/processor` with the context-bound logger, so a single `grep prompt_id=<id>` returns that prompt's whole lifecycle.
- Normalizes the prompt-identity attribute key to `prompt_id` everywhere in `pkg/processor` (the old `file`/`path` keys for prompt identity are removed in favor of `prompt_id`).
- Adds a `workflow_step` attribute at the key decision points (acquire container, run claude, commit, push) so the timeline is reconstructable from one grep.
- Leaves the tree green: `make precommit` passes and `make hotpath-logcheck` (still warn mode) no longer reports `pkg/processor`.

</summary>

<objective>
Migrate `pkg/processor` to the context-bound logger from prompt 1. Bind the per-prompt logger with the four correlation attrs (`prompt_id`, `spec_id`, `container`, `workflow_type`) at the `ProcessPrompt` entry point, re-bind on container assignment, and replace every bare `slog.Info/Warn/Error` in the package with `log.From(ctx).Info/Warn/Error`. Normalize prompt-identity keys to `prompt_id` and add `workflow_step` at decision points.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-logging-guide.md` — slog conventions
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors` wrapping stays unchanged
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega, coverage

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/099-correlation-ids-structured-logging.md` — especially Desired Behavior items 2, 3, 4; Constraints; Failure Modes rows 2, 5, 6; Acceptance Criteria 3, 5, 6, 8.

Read prompt 1's deliverables (they MUST already be on the tree — if `pkg/log/context.go` is absent, STOP and report `Status: failed` with message "pkg/log helpers not yet deployed (prompt 1)"):
- `/workspace/pkg/log/context.go` — `NewContext(ctx, logger) ctx` and `From(ctx) *slog.Logger`.
- `/workspace/docs/rules/logging-conventions.md` — the canonical key set and threading rule.

Read these source files END-TO-END before editing (full reads, not skims):
- `/workspace/pkg/processor/processor.go` — `ProcessPrompt` (the binding site), `runContainer`, `enterPendingVerification`, `handleEmptyPrompt`, `moveCancelledPrompt`, `computePromptMetadata`, `Process`, `runReadyTick`, `runQueueTick`, `runSweepTick`. ALL `slog.*` calls here must migrate.
- `/workspace/pkg/processor/workflow_executor_branch.go`, `workflow_executor_clone.go`, `workflow_executor_worktree.go`, `workflow_executor_direct.go`, `workflow_helpers.go` — these ALSO have `slog.*` calls and are part of `pkg/processor`. They must migrate too.
- `/workspace/pkg/prompt/prompt.go` — `Frontmatter.Specs` (type `SpecList []string`), `(*PromptFile).Specs() []string` (line 596), `(*PromptFile).Title()`, `(*PromptFile).PrepareForExecution(container, version string)`. These supply `spec_id`.
- `/workspace/pkg/config/workflow.go` — the `config.Workflow` enum (`WorkflowDirect`/`WorkflowBranch`/`WorkflowWorktree`/`WorkflowClone`) and `(Workflow).String()`. Supplies `workflow_type`.
- `/workspace/pkg/factory/factory.go` around line 942 (`workflowExecutor := workflowExecutorProvider.Get(ctx, cfg.Workflow)`) and line 994 (`proc := processor.NewProcessor(...)`). This is the ONLY `NewProcessor` call site (verified: `grep -rn 'NewProcessor(' --include='*.go' | grep -v _test` returns only `pkg/processor/processor.go` and `pkg/factory/factory.go`). `main.go` is the only binary entry point. There is NO `cmd/run-once` sibling — do not search for one.

VERIFIED FACTS (do not re-derive):
- `ProcessPrompt(ctx context.Context, pr prompt.Prompt)` is called from `pkg/queuescanner/scanner.go` line 316 (`s.promptProcessor.ProcessPrompt(ctx, pr)`). It is the single per-prompt entry point. Binding the logger here (spec Desired Behavior item 2) means all downstream calls within `ProcessPrompt` — including `runContainer` -> `cancellationWatcher.Watch(execCtx, ...)` and `workflowExecutor.Setup/Complete` — inherit the bound logger via the threaded `ctx`/`execCtx`.
- The processor struct does NOT currently know its workflow type. `cfg.Workflow` is resolved in the factory (line 942). To supply the `workflow_type` attr, add a `workflowType config.Workflow` field to the processor (see requirement 1).
- `spec_id` derivation: `pf.Frontmatter.Specs` is a `SpecList` (`[]string`). Use the first element when present, else empty string (spec Failure Mode row 2: spec-less prompt binds `spec_id=""`). Do NOT error on multiple specs here — pick the first (the scanner already fails closed on multi-spec elsewhere; the logger just needs a representative id).
- `prompt_id` derivation: basename without `.md` extension — i.e. the `baseName` already computed by `computePromptMetadata(pr.Path, p.projectName)` (returns `prompt.BaseName`). Use `baseName.String()`.
- `container` at bind time is empty string (container not yet assigned). It is re-bound after `pf.PrepareForExecution(containerName.String(), ...)` (spec Desired Behavior item 4).

Current `ProcessPrompt` log calls to migrate (verbatim references from `pkg/processor/processor.go`):
- `slog.Info("executing prompt", "title", title)` (~line 310)
- `slog.Info("prompt cancelled", "file", filepath.Base(promptPath))` (~lines 404, 413) in `runContainer`
- `slog.Info("daemon shutting down, leaving container running")` (~lines 417, 424)
- `slog.Info("docker container exited with error", "error", execErr)` (~line 419)
- `slog.Info("docker container exited", "exitCode", 0)` (~line 427)
- `slog.Info("prompt pending verification ...", "file", ..., "verification", hint)` (~lines 443, 451) in `enterPendingVerification`
- `slog.Debug("skipping empty prompt", "file", ..., "reason", ...)` (~line 467) in `handleEmptyPrompt`
- `slog.Warn("failed to move cancelled prompt", "file", ..., "error", ...)` (~line 488) in `moveCancelledPrompt`
- `slog.Info("processor started")`, `slog.Warn("prompt failed on startup scan...")`, `slog.Info("processor shutting down")`, etc. in `Process`/`runReadyTick`/`runQueueTick`/`runSweepTick`.

</context>

<requirements>

## 1. Give the processor its workflow type (for the `workflow_type` attr)

In `/workspace/pkg/processor/processor.go`:

1.1. Add a field `workflowType config.Workflow` to the `processor` struct (after `verificationGate` or anywhere consistent with the struct's grouping). Import `"github.com/bborbe/dark-factory/pkg/config"` (verify it is not already imported; the processor package may already import config indirectly — add the direct import).

1.2. Add a `workflowType config.Workflow` parameter to `NewProcessor` and assign it into the struct. Place it adjacent to `verificationGate bool` in the param list (document it with a short comment: `// workflowType is the configured workflow (direct/branch/clone/worktree); used as the workflow_type log attr.`). Because Go has no default parameters, you MUST update the single call site.

1.3. Update the ONLY `NewProcessor` call site in `/workspace/pkg/factory/factory.go` (~line 994) to pass `cfg.Workflow` for the new parameter. `cfg.Workflow` is already in scope there (it is used at line 942: `workflowExecutorProvider.Get(ctx, cfg.Workflow)`). Pass it in the matching positional slot.

1.4. Update any `NewProcessor(...)` call in `pkg/processor`'s OWN tests that construct a processor directly (search: `grep -rn 'NewProcessor(' pkg/processor/`). If tests use a helper constructor, update that. Pass `config.WorkflowDirect` (or the workflow the test exercises) as the new arg. Do NOT break existing tests.

## 2. Bind the per-prompt logger at `ProcessPrompt` entry — via a `bindPromptLogger` helper

Add this private helper to `/workspace/pkg/processor/processor.go`. The single-line `return log.NewContext(ctx, slog.Default().With(` is what satisfies the spec AC-3 grep `log\.NewContext\(.*With\(`; keep `log.NewContext(` and `slog.Default().With(` on ONE line (golines keeps this line under 100 chars):

```go
// bindPromptLogger returns ctx carrying a logger with the four correlation attrs
// (spec 099). All downstream log.From(ctx) calls inherit these attrs.
func bindPromptLogger(ctx context.Context, promptID, specID, container, workflowType string) context.Context {
	return log.NewContext(ctx, slog.Default().With(
		"prompt_id", promptID,
		"spec_id", specID,
		"container", container,
		"workflow_type", workflowType,
	))
}
```

In `ProcessPrompt`, after `baseName, containerName := computePromptMetadata(...)` and after `pf` is loaded (so `pf.Frontmatter.Specs` is available), but BEFORE any hot-path log call or downstream call (`workflowExecutor.Setup`, `runContainer`, etc.), bind the logger into `ctx`:

```go
specID := ""
if specs := pf.Frontmatter.Specs; len(specs) > 0 {
	specID = specs[0]
}
ctx = bindPromptLogger(ctx, baseName.String(), specID, "", p.workflowType.String())
```

Import `log "github.com/bborbe/dark-factory/pkg/log"` (the file already imports `log/slog` as `slog`; use the alias `log` for the new package). Verify the module path is `github.com/bborbe/dark-factory/pkg/log`.

After this, the spec AC-3 grep `grep -nE 'log\.NewContext\(.*With\(' pkg/processor/processor.go` returns the helper's `return log.NewContext(ctx, slog.Default().With(` line, and the four keys appear within that `With(...)` call. Verify with the AC command after `make format`.

## 3. Re-bind the logger when the container is assigned

In `ProcessPrompt`, after `pf.PrepareForExecution(containerName.String(), ...)` succeeds (the container name is now known), re-bind the logger to carry the real container name and emit ONE transition line (spec Desired Behavior item 4, Failure Mode row 5):

```go
log.From(ctx).Info("container assigned",
	"container_old", "",
	"container", containerName.String(),
	"workflow_step", "acquire_container",
)
ctx = log.NewContext(ctx, log.From(ctx).With("container", containerName.String()))
```

This emits the `container assigned` line with both `container_old=` and `container=` present (spec Failure Mode row 5 detection), then re-binds so all subsequent lines carry the real container name.

Add `container_old` to the `## Canonical Keys` section of `/workspace/docs/rules/logging-conventions.md` in THIS prompt (transition-only key). The `hotpath-logcheck` script inspects `slog.X(` call FORMS only (not keys), so no allow-list edit is needed — but the doc MUST record the new key per the spec's "new key requires doc update" rule (Failure Mode row 4).

Place the re-bind so that all later log lines (`runContainer`, `Complete`, etc.) inherit `container=<name>`.

## 4. Replace all bare `slog.Info/Warn/Error/Debug` in `pkg/processor` with `log.From(ctx)`

For EVERY `slog.Info(...)`, `slog.Warn(...)`, `slog.Error(...)`, `slog.Debug(...)`, `slog.InfoContext(ctx,...)`, `slog.WarnContext(ctx,...)`, `slog.DebugContext(ctx,...)` call in ALL `pkg/processor/*.go` NON-test files (`processor.go`, `workflow_executor_branch.go`, `workflow_executor_clone.go`, `workflow_executor_worktree.go`, `workflow_executor_direct.go`, `workflow_helpers.go`), rewrite to use the context-bound logger:

- `slog.Info(msg, kv...)` -> `log.From(ctx).Info(msg, kv...)`
- `slog.Warn(msg, kv...)` -> `log.From(ctx).Warn(msg, kv...)`
- `slog.Error(msg, kv...)` -> `log.From(ctx).Error(msg, kv...)`
- `slog.Debug(msg, kv...)` -> `log.From(ctx).Debug(msg, kv...)`
- `slog.InfoContext(ctx, msg, kv...)` -> `log.From(ctx).Info(msg, kv...)` (drop the redundant `Context` variant; the ctx now carries the logger)
- `slog.WarnContext(ctx, msg, kv...)` -> `log.From(ctx).Warn(msg, kv...)`
- `slog.DebugContext(ctx, msg, kv...)` -> `log.From(ctx).Debug(msg, kv...)`

EVERY enclosing function MUST have a `ctx context.Context` in scope. Verified: all `slog.*` sites in `pkg/processor` are inside functions that already take `ctx` (e.g. `Process(ctx)`, `runReadyTick(ctx,...)`, `runContainer(ctx,...)`, `syncWithRemoteViaDeps(ctx, deps)`, `Complete(gitCtx, ctx, ...)` — use `ctx`, NOT `gitCtx`, for log calls). If any helper lacks ctx, thread it from its caller. After this, `grep -cE 'slog\.(Info|Warn|Error)\b' pkg/processor/processor.go` MUST return 0 (spec AC 3).

## 5. Normalize prompt-identity keys to `prompt_id`; add `workflow_step` at decision points

5.1. Prompt-identity key normalization (spec AC 5/6, Removed Synonyms): in `pkg/processor`, every log attr that names the PROMPT identity by basename currently uses `"file"` or `"path"`. For lines emitted AFTER the bind, the bound logger already carries `prompt_id`, so DROP the redundant per-call `"file"`/`"path"` identity attr. For lines emitted BEFORE the bind (in `Process`/`runReadyTick`/`runQueueTick`/`runSweepTick`, which run on the daemon ctx with no per-prompt bind), keep an identity attr but name it `"prompt_id"` (not `"file"`/`"path"`). Keep `"branch"`, `"error"`, `"dir"` as-is (already canonical).

5.2. snake_case the kept informational keys (spec AC 6): rename camelCase attr keys to snake_case — `completedPath` -> `completed_path`, `promptPath` -> drop (covered by bound `prompt_id`), `remoteBranch` -> `remote_branch`, `exitCode` -> `exit_code`. Keep genuinely-useful informational keys (`url`, `version`, `title`, `verification`, `reason`, `hint`) but ensure they are snake_case (these are already single words or snake_case). Add to `docs/rules/logging-conventions.md` `## Canonical Keys` this clarifying sentence: "Correlation keys are fixed; informational keys are permitted but MUST be snake_case." (Prompts 3-4 rely on this clarification.)

5.3. Add `workflow_step` at decision points (spec Desired Behavior item 5). Add `"workflow_step", "<step>"` to the relevant INFO line using the closest label from the spec's set `{acquire_container, run_claude, commit, push, cancel, resume}`:
- container acquisition: the `container assigned` line already carries `"workflow_step", "acquire_container"` (requirement 3).
- container run: in `runContainer`, add `"workflow_step", "run_claude"` to the `docker container exited` / `docker container exited with error` lines.
- commit/push: in `workflow_helpers.go` / `workflow_executor_*.go`, add `"workflow_step", "commit"` to commit lines (`committed changes`, `committed and tagged`), `"workflow_step", "push"` to push/release lines AND to merge lines (`merged PR and updated default branch`) — merge is the same lifecycle phase as push per the spec's canonical set `{acquire_container, run_claude, commit, push, cancel, resume}`. Do NOT introduce a `merge` value.
- cancel: add `"workflow_step", "cancel"` to the `prompt cancelled` lines.

Add `workflow_step` only at genuine decision points — do NOT spray it on every line.

## 6. camelCase -> snake_case sweep

Confirm no camelCase inline-kv attr key remains in `pkg/processor` non-test files. Known offenders: `completedPath`, `promptPath`, `remoteBranch`, `exitCode`. After the renames in 5.2, verify by inspection that no inline kv key in `pkg/processor` non-test code is camelCase. (The spec's AC-6 `slog.String`-form grep is vacuous in this codebase since it uses inline kv; the real check is the inline-kv camelCase sweep below.)

## 7. Update / add tests

7.1. Existing `pkg/processor` tests that assert on log output (if any) — update for the new keys. Search `grep -rn 'prompt_id\|"file"\|"path"' pkg/processor/*_test.go` and adjust assertions that depended on old keys.

7.2. Add a focused test in `pkg/processor` (internal or external test pkg, matching the existing test style) that exercises the binding: test `bindPromptLogger` directly — set `slog.SetDefault` to a `slog.New(slog.NewTextHandler(&buf, nil))` (restore after), call `ctx := bindPromptLogger(context.Background(), "042-foo", "099", "", "direct")`, then `log.From(ctx).Info("x")`, and assert the output contains `prompt_id=042-foo`, `spec_id=099`, `container=`, and `workflow_type=direct`. This is the boundary test exercising the binding the production path uses.

## 8. CHANGELOG

Append to the `## Unreleased` section of `/workspace/CHANGELOG.md` ONE bullet:

```
- refactor: migrate pkg/processor to the context-bound logger; bind per-prompt correlation attrs (prompt_id/spec_id/container/workflow_type) at ProcessPrompt entry, re-bind container on assignment, add workflow_step at decision points (spec 099 prompt 2)
```

</requirements>

<constraints>

- Each chunked migration prompt leaves the tree in a `make precommit`-green state (spec Constraint). After this prompt `make precommit` MUST exit 0.
- `make hotpath-logcheck` is still in WARN mode (prompt 5 flips it). After this prompt the warn output MUST no longer list any `pkg/processor` file (all migrated).
- The single binding site is `ProcessPrompt` (spec Desired Behavior item 2). Do NOT add a second per-prompt binding site in `pkg/processor`. (The pre-bind identity attrs in `Process`/`runReadyTick` are NOT a logger bind — they are per-call attrs on the fallback logger.)
- `spec_id=""` for spec-less prompts is CORRECT (spec Failure Mode row 2) — do NOT error or skip the bind when `Specs` is empty.
- Use `ctx` (not `gitCtx`) for log calls in `Complete`/`workflow_helpers.go` where both are in scope. `context.WithoutCancel(ctx)` preserves context values, so `gitCtx` does carry the bound logger, but prefer `ctx` for clarity.
- Counterfeiter mocks regenerate cleanly: run `go generate ./...` (exit 0) after the `NewProcessor` signature change.
- No new dependencies (spec Constraint).
- Errors wrapped with `bborbe/errors` — never `fmt.Errorf`, never `context.Background()` in pkg/ non-test code. Do NOT change any error-wrapping call.
- The default slog handler choice is unchanged (spec Constraint) — you only attach attributes via `slog.Default().With(...)` and re-bind; you do NOT call `slog.SetDefault` in production code.
- BSD-style license header on every modified file must survive the edit.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# AC 3 — binding references all four keys; no bare slog.Info/Warn/Error in processor.go
grep -nE 'log\.NewContext\(.*With\(' pkg/processor/processor.go
# expected: >= 1 line (the bindPromptLogger helper's return)
grep -nE 'prompt_id|spec_id|container|workflow_type' pkg/processor/processor.go
# expected: all four keys present
grep -cE 'slog\.(Info|Warn|Error)\b' pkg/processor/processor.go
# expected: 0

# AC 4 (partial) — no bare slog.Info/Warn/Error anywhere in pkg/processor non-test files
grep -rnE 'slog\.(Info|Warn|Error)\(' pkg/processor --include='*.go' | grep -v _test.go
# expected: 0 lines

# hotpath-logcheck (warn) no longer lists pkg/processor
make hotpath-logcheck 2>&1 | grep 'pkg/processor' || echo "processor clean"
# expected: "processor clean"

# AC 6 — no camelCase attr keys in pkg/processor non-test files (inline-kv form)
grep -rEn '"[a-z]+[A-Z][a-zA-Z]*",' pkg/processor/*.go | grep -v _test.go || echo "no camelCase keys"
# expected: "no camelCase keys" (manually confirm any hit is NOT an attr key)

# build + tests
go generate ./... && go build ./...
# expected: exit 0
go test ./pkg/processor/... ./pkg/factory/... ./pkg/log/...
# expected: PASS

# coverage for processor stays healthy
go test -coverprofile=/tmp/cover.out ./pkg/processor/... && go tool cover -func=/tmp/cover.out | tail -1

# CHANGELOG entry
grep -n 'spec 099 prompt 2' CHANGELOG.md
# expected: >= 1 line

# full precommit
make precommit
# expected: exit 0
```

</verification>
