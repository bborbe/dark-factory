---
status: approved
spec: [099-correlation-ids-structured-logging]
created: "2026-06-26T05:42:49Z"
queued: "2026-06-26T06:13:15Z"
branch: dark-factory/correlation-ids-structured-logging
---

<summary>

- Migrates three hot-path packages to the context-bound logger: the Docker executor, the executing-prompt resumer, and the committing recoverer.
- Every bare package-level log call in these packages becomes a context-bound call, so the per-prompt correlation attributes attached upstream flow into their log lines.
- Normalizes attribute keys to snake_case (e.g. `containerName`, `maxPromptDuration`, `contentSize` become snake_case) and collapses prompt-identity keys to `prompt_id`.
- Leaves one documented exception untouched: the shared argv-builder code in the executor's `launch.go` (also used by healthcheck boot/probe paths) keeps its bare calls, since those functions have no per-prompt context and are explicitly excluded from the hot-path check.
- After this prompt the hot-path check (still warn mode) no longer reports these three packages.
- Leaves the tree green: `make precommit` passes.

</summary>

<objective>
Migrate `pkg/executor`, `pkg/promptresumer`, and `pkg/committingrecoverer` to the context-bound logger from prompt 1. Replace every bare `slog.Info/Warn/Error/Debug` (where a `ctx` is in scope) with `log.From(ctx).X`, normalize attr keys to snake_case, and collapse prompt-identity keys to `prompt_id`. The shared argv-builder functions in `pkg/executor/launch.go` are explicitly excluded (documented exception).
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
- `/workspace/docs/rules/logging-conventions.md` — canonical key set, snake_case rule, the `pkg/executor/launch.go` exclusion documented in prompt 1.
- `/workspace/scripts/hotpath-logcheck.sh` — confirm `pkg/executor/launch.go` is in its exclusion list (prompt 1 added it). If it is NOT excluded there, ADD it in this prompt's edits to the script (the launch.go builder functions have no ctx and are shared with healthcheck probes — they cannot use `log.From(ctx)`).

Read these source files END-TO-END before editing:
- `/workspace/pkg/executor/executor.go` — ALL `slog.*` calls. Verified each enclosing function HAS a `ctx context.Context` in scope:
  - `runWithFormatterPipeline(ctx,...)` -> `slog.Warn(fmtErrMsg, "error", fmtErr)` (~line 152)
  - `Execute(ctx,...)` -> `slog.Debug("prompt prepared...", "contentSize", ..., "tempFile", ...)` (~line 109), `slog.Debug("docker command prepared", "image", ..., "containerName", ..., "workspaceMount", ..., "configMount", ...)` (~line 118)
  - `Reattach(ctx,...)` -> `slog.Info("reattaching...", "containerName", ..., "maxPromptDuration", ...)` (~line 225)
  - `timeoutKiller(ctx,...)` -> `slog.Warn("container exceeded maxPromptDuration...", "containerName", ..., "duration", ...)` (~line 274), `slog.Warn("docker stop failed...", "containerName", ..., "error", ...)` (~line 280), `slog.Error("docker kill also failed...", "containerName", ..., "error", ...)` (~line 285)
  - `watchForCompletionReport(ctx,...)` -> `slog.Debug("watchForCompletionReport: failed to read log file", "error", err)` (~line 314), `slog.Info("stopping stuck container...", "containerName", ...)` (~line 318)
  - `StopAndRemoveContainer(ctx,...)` -> `slog.Debug("docker stop", "container", ..., "error", ...)` (~line 563)
  - `removeContainerIfExists(ctx,...)` -> `slog.Debug("docker rm -f", "containerName", ..., "error", ...)` (~line 575)
- `/workspace/pkg/executor/launch.go` — `slog.Error` (~line 121 in `buildClaudeDirMount`), `slog.Debug` (~lines 169, 199 in `appendExtraMounts`/`buildHideGitArgsForRoot`). THESE FUNCTIONS HAVE NO `ctx` PARAM and are shared with healthcheck probes via `BuildDockerRunArgs`. DO NOT migrate them. They stay as bare `slog.*`. They are EXCLUDED from the hot-path check (documented in prompt 1 / `scripts/hotpath-logcheck.sh`).
- `/workspace/pkg/promptresumer/resumer.go` — ALL `slog.*` calls. Enclosing functions: `resumePrompt(ctx,...)`, `prepareResume(ctx,...)`, `killTimedOutContainer(ctx,...)`, `computeReattachDuration(started)` — NOTE `computeReattachDuration` has NO ctx param but contains `slog.Warn`/`slog.Info` (~lines 250, 262). Thread `ctx` into `computeReattachDuration` (add `ctx context.Context` as the first param) and update its single caller in `resumePrompt` (`r.computeReattachDuration(pf.Frontmatter.Started)` -> `r.computeReattachDuration(ctx, pf.Frontmatter.Started)`).
- `/workspace/pkg/committingrecoverer/recoverer.go` — ALL `slog.*` calls. Enclosing functions: `RecoverAll(ctx)`, `Recover(ctx,...)` — both have ctx.

VERIFIED FACTS (do not re-derive):
- `pkg/promptresumer.ResumeAll` is invoked at daemon STARTUP, NOT via `ProcessPrompt`. So its `ctx` has NO logger bound from `ProcessPrompt` — `log.From(ctx)` returns the `slog.Default()` fallback (no correlation attrs). This is the spec's intended behavior (Failure Mode "pre-bind lines"). DO NOT add a `ProcessPrompt`-style bind here in this prompt — just mechanical `slog.X` -> `log.From(ctx).X` replacement. (Optional improvement: the resumer COULD bind its own per-prompt logger in `resumePrompt`; the spec does not require it. If you do it, note it in `## Improvements`. Default: do NOT — keep this prompt mechanical.)
- The `pkg/executor/launch.go` builder functions are the ONLY hot-path-package `slog.*` calls that remain bare after this prompt — by design (documented exclusion). The hot-path check excludes `launch.go`.

</context>

<requirements>

## 1. Migrate `pkg/executor/executor.go`

Replace EVERY bare `slog.Info/Warn/Error/Debug` call in `pkg/executor/executor.go` with `log.From(ctx).X`, using the `ctx` already in scope in each enclosing function (listed in `<context>`). Import `log "github.com/bborbe/dark-factory/pkg/log"` (the file already imports `log/slog` as `slog` — keep `slog` for the `slog.String`-free attr forms it does not use; you still need `slog` imported only if other references remain. After migration, if `slog` is unused, remove the `"log/slog"` import. Verify with `goimports`.)

Key normalization in `executor.go` (snake_case, spec AC 6):
- `"containerName"` -> `"container"` (this is the canonical container key; the value is the container name). Where a line already has both `"container"` and `"containerName"`, dedupe to one `"container"`.
- `"contentSize"` -> `"content_size"`
- `"tempFile"` -> `"temp_file"`
- `"workspaceMount"` -> `"workspace_mount"`
- `"configMount"` -> `"config_mount"`
- `"maxPromptDuration"` -> `"max_prompt_duration"`
- `"image"` -> `"image"` (already snake/single word — keep)
- `"duration"` -> `"duration"` (keep)
- `"error"` -> keep (canonical)
- `"container"` -> keep (canonical)

DO NOT migrate `pkg/executor/launch.go` — leave its `slog.Error`/`slog.Debug` calls bare (documented exclusion).

## 2. Migrate `pkg/promptresumer/resumer.go`

2.1. Thread `ctx` into `computeReattachDuration`: change its signature to `func (r *resumer) computeReattachDuration(ctx context.Context, started string) (time.Duration, time.Duration, bool)` and update the single caller in `resumePrompt`.

2.2. Replace EVERY bare `slog.Info/Warn` in `resumer.go` with `log.From(ctx).X`.

Key normalization in `resumer.go`:
- `"file"` (prompt basename identity) -> `"prompt_id"`.
- `"container"` -> keep (canonical).
- `"maxPromptDuration"` -> `"max_prompt_duration"`.
- `"started"` -> `"started"` (keep — single word, informational).
- `"elapsed"`, `"remaining"` -> keep (single word, informational).
- `"error"` -> keep.

Import `log "github.com/bborbe/dark-factory/pkg/log"`. Remove `"log/slog"` if it becomes unused.

## 3. Migrate `pkg/committingrecoverer/recoverer.go`

Replace EVERY bare `slog.Info/Warn/Error` with `log.From(ctx).X` (both `RecoverAll` and `Recover` have `ctx`).

Key normalization in `recoverer.go`:
- `"file"` (prompt basename identity) -> `"prompt_id"`.
- `"spec"` -> `"spec_id"` (canonical correlation key for spec identity).
- `"error"` -> keep.

Import `log "github.com/bborbe/dark-factory/pkg/log"`. Remove `"log/slog"` if unused.

## 4. snake_case sweep across the three packages

After steps 1-3, confirm no camelCase inline-kv attr key remains in the three packages' non-test files. Check:
```
grep -rEn '"[a-z]+[A-Z][a-zA-Z]*",' pkg/executor/executor.go pkg/promptresumer/resumer.go pkg/committingrecoverer/recoverer.go
```
Any remaining camelCase attr key is a miss — fix it. (`launch.go` is intentionally NOT in this list — its keys stay as-is under the exclusion.)

## 5. Tests

5.1. Update any existing tests in these three packages that assert on log keys (`grep -rn '"file"\|"containerName"\|"spec"\|prompt_id' pkg/executor/*_test.go pkg/promptresumer/*_test.go pkg/committingrecoverer/*_test.go`). Adjust assertions to the new keys.

5.2. For `computeReattachDuration` (now taking ctx) — update its existing unit test caller to pass `context.Background()`.

5.3. Add or extend a test that proves `log.From(ctx)` is used on at least one path in each package: e.g. in `pkg/committingrecoverer`, set `slog.SetDefault` to a captured-buffer handler in the test (restore after), bind a logger via `log.NewContext(ctx, slog.Default().With("prompt_id", "test-042"))`, call `RecoverAll(ctx)` against a fake that yields one committing prompt, and assert the emitted line carries `prompt_id=test-042` (proving the bound logger propagates). This is the boundary test (the bound logger flows through). Keep it minimal; reuse existing fakes/mocks.

## 6. CHANGELOG

Append to `## Unreleased` in `/workspace/CHANGELOG.md` ONE bullet:

```
- refactor: migrate pkg/executor, pkg/promptresumer, pkg/committingrecoverer to the context-bound logger (log.From(ctx)); normalize attr keys to snake_case; thread ctx into computeReattachDuration; pkg/executor/launch.go shared argv-builders excluded by design (spec 099 prompt 3)
```

</requirements>

<constraints>

- Each chunked migration prompt leaves the tree `make precommit`-green (spec Constraint). `make precommit` MUST exit 0 after this prompt.
- `pkg/executor/launch.go` is EXCLUDED — do NOT migrate its `slog.Error`/`slog.Debug` calls. Those functions have no per-prompt ctx and are shared with healthcheck probes (spec Non-goal "Does NOT migrate boot-time logs"). Confirm `scripts/hotpath-logcheck.sh` excludes `launch.go`; if not, add the exclusion in this prompt.
- After this prompt, `make hotpath-logcheck` (still warn mode) MUST no longer list `pkg/executor/executor.go`, `pkg/promptresumer/resumer.go`, or `pkg/committingrecoverer/recoverer.go`. It MAY still list `pkg/cancellationwatcher` and `pkg/queuescanner` (migrated in prompt 4) — that is fine in warn mode.
- Do NOT bind a new per-prompt logger in `pkg/promptresumer` in this prompt — mechanical replacement only (the resumer's ctx has no upstream bind; fallback is intended).
- No new dependencies (spec Constraint).
- Errors wrapped with `bborbe/errors` — never `fmt.Errorf`, never `context.Background()` in pkg/ non-test code. Do NOT change error-wrapping.
- The default slog handler choice is unchanged (spec Constraint) — no `slog.SetDefault` in production code.
- Counterfeiter mocks regenerate cleanly: `go generate ./...` exit 0 (the `computeReattachDuration` signature change is private — no mock impact, but regenerate to be safe).
- BSD-style license header on every modified file must survive.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# AC 4 (partial) — no bare slog.Info/Warn/Error in the three migrated files
grep -rnE 'slog\.(Info|Warn|Error)\(' pkg/executor/executor.go pkg/promptresumer/resumer.go pkg/committingrecoverer/recoverer.go
# expected: 0 lines

# launch.go intentionally still has bare slog (the exclusion)
grep -cE 'slog\.(Error|Debug)\(' pkg/executor/launch.go
# expected: >= 1 (unchanged)

# hotpath-logcheck (warn) no longer lists the three migrated files
make hotpath-logcheck 2>&1 | grep -E 'pkg/executor/executor.go|pkg/promptresumer/resumer.go|pkg/committingrecoverer/recoverer.go' || echo "three packages clean"
# expected: "three packages clean"

# AC 6 — no camelCase attr keys in the three migrated files
grep -rEn '"[a-z]+[A-Z][a-zA-Z]*",' pkg/executor/executor.go pkg/promptresumer/resumer.go pkg/committingrecoverer/recoverer.go || echo "no camelCase keys"
# expected: "no camelCase keys"

# build + tests
go generate ./... && go build ./...
go test ./pkg/executor/... ./pkg/promptresumer/... ./pkg/committingrecoverer/... ./pkg/log/...
# expected: PASS

# CHANGELOG
grep -n 'spec 099 prompt 3' CHANGELOG.md
# expected: >= 1 line

# full precommit
make precommit
# expected: exit 0
```

</verification>
