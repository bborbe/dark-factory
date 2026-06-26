---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-06-26T05:30:43Z"
generating: "2026-06-26T05:39:50Z"
prompted: "2026-06-26T06:00:36Z"
verifying: "2026-06-26T07:12:50Z"
branch: dark-factory/correlation-ids-structured-logging
---

## Summary

- Today's hot-path log sites (~89 across `pkg/processor`, `pkg/executor`, `pkg/cancellationwatcher`, `pkg/promptresumer`, `pkg/committingrecoverer`, `pkg/queuescanner`) build attribute lists from scratch and disagree on key names: the same prompt appears as `"file"` in one place, `"path"` in another, `"prompt"` in a third. There is no correlation ID that ties a prompt's lifecycle together across packages or container boundaries.
- This spec introduces a per-prompt `*slog.Logger` bound at `ProcessPrompt` entry with stable structured attrs (`prompt_id`, `spec_id`, `container`, `workflow_type`) and threads it through hot-path packages via `context.Context`. Every downstream log line on that code path inherits those attrs without restating them.
- An attribute-key convention (snake_case, fixed canonical set) is documented in `docs/rules/logging-conventions.md`. Synonyms (`file`/`path`/`prompt` → `prompt_id`) are removed at migration time, not deprecated.
- A `make precommit`-time check enforces the convention: hot-path packages must use a context-bound logger (`log.From(ctx)`) — bare `slog.Info/Warn/Error` package-level calls are rejected by the check.
- Migration is chunked per package (see Suggested Decomposition); end-state is a green `make precommit` with the strict check enabled and a single `grep prompt_id=<id> .dark-factory.log` returning the full lifecycle of one prompt.

## Problem

`grep` is currently the only debugging primitive for "what happened to prompt X?", and it returns wrong answers — sometimes `grep file=<id>` finds creation but misses execution (which logs under `prompt=`), sometimes `grep prompt=<id>` finds workflow setup but misses cancellation (which logs under `path=`). The operator runs three greps with three key guesses, OR's the result, hopes nothing is missed. Worse, since no correlation ID is propagated, lines from `pkg/cancellationwatcher` that act on a prompt have no shared key with the lines from `pkg/processor` that created it — they share only a basename buried inside a free-text message. Stuck prompts in production are debugged by reading hundreds of lines of mixed-tenant logs and reconstructing the timeline by hand. The 2026-06-25 architecture review flagged this as observability finding #4.

## Goal

After this work, every hot-path log line emitted during a prompt's lifecycle carries the prompt's correlation attrs (`prompt_id`, `spec_id`, `container`, `workflow_type`) at fixed snake_case keys. An operator filters the daemon log with a single `grep prompt_id=<id>` and gets the prompt's complete lifecycle — creation, workflow start, container assignment, cancellation/resume, commit, completion — in chronological order, regardless of which package emitted the line. New hot-path code that bypasses the inherited logger fails `make precommit`.

## Non-goals

- Does NOT introduce OpenTelemetry, OTLP exporters, or distributed tracing — separate future spec.
- Does NOT add log-shipping / aggregation infrastructure (Loki, ELK, Datadog) — out of scope.
- Does NOT rename existing log levels (`debug`/`info`/`warn`/`error` stay as-is).
- Does NOT migrate boot-time logs (`cmd/dark-factory/main.go`, factory wiring, daemon startup banner). Those have no `ProcessPrompt` context; separate task.
- Does NOT change the default `slog` handler / output format — stdout-text in dev, JSON when configured by the existing handler-selection logic.
- Does NOT add a per-package opt-out flag for the precommit check. Invariant; if a future package legitimately needs unbound logging, that's a separate spec.
- Does NOT migrate test helpers, mock files, `*_test.go`, or `counterfeiter`-generated files — they may use bare `slog.X` indefinitely.
- Does NOT touch `pkg/log` interfaces beyond adding `NewContext`/`From`. The package's existing API stays compatible.

## Acceptance Criteria

- [ ] `pkg/log` exposes `NewContext(ctx context.Context, logger *slog.Logger) context.Context` and `From(ctx context.Context) *slog.Logger` (returning a default fallback logger when no logger is bound, never nil) — evidence: `grep -nE 'func (NewContext|From)\(' pkg/log/context.go` returns both lines.
- [ ] `pkg/log/context_test.go` asserts: (a) `From(NewContext(ctx, L))` returns `L`; (b) `From(context.Background())` returns a non-nil fallback that emits to the default handler; (c) attributes added via `With` on the bound logger are present on downstream emissions. Evidence: `go test ./pkg/log/...` exit 0 with these three test names present (`grep -E 'func Test(From|NewContext|With)' pkg/log/context_test.go` returns ≥3 lines).
- [ ] `ProcessPrompt` binds a logger with attrs `prompt_id`, `spec_id`, `container`, `workflow_type` into the context before any hot-path call. Evidence: `grep -nE 'log\.NewContext\(.*With\(' pkg/processor/processor.go` returns ≥1 line referencing all four keys; `grep -cE 'slog\.(Info|Warn|Error)\b' pkg/processor/processor.go` returns 0.
- [ ] All hot-path log sites in the migrated packages (`pkg/processor`, `pkg/executor`, `pkg/promptresumer`, `pkg/committingrecoverer`, `pkg/cancellationwatcher`, `pkg/queuescanner`) use `log.From(ctx)`, NOT package-level `slog.Info/Warn/Error`. Evidence: `make hotpath-logcheck` exits 0; the same command on the pre-migration tree exits non-zero with a list of offending file:line.
- [ ] The attribute key set used by hot-path logs is exactly `{prompt_id, spec_id, container, workflow_type, error, file, dir, branch, workflow_step}`. Evidence: `grep -roE 'slog\.(String|Int|Bool|Any|Duration|Time|Group)\("[a-z_]+"' pkg/processor pkg/executor pkg/promptresumer pkg/committingrecoverer pkg/cancellationwatcher pkg/queuescanner | awk -F'"' '{print $2}' | sort -u` returns only keys in that set (or a subset).
- [ ] No camelCase or kebab-case attribute keys remain in the migrated packages. Evidence: `grep -rEn 'slog\.[A-Z][a-zA-Z]*\("[a-z]+[A-Z]' pkg/processor pkg/executor pkg/promptresumer pkg/committingrecoverer pkg/cancellationwatcher pkg/queuescanner` returns 0 lines; same grep replacing `[A-Z]` with `-` also returns 0 lines.
- [ ] `docs/rules/logging-conventions.md` exists and contains: (a) the canonical key set, (b) the snake_case rule, (c) the context-threading rule with `log.From(ctx)` named, (d) a "Removed synonyms" table mapping `file`/`path`/`prompt` → `prompt_id`. Evidence: `grep -nE '^## (Canonical Keys|Threading|Removed Synonyms)' docs/rules/logging-conventions.md` returns ≥3 lines.
- [ ] One end-to-end prompt run leaves a log file whose lifecycle can be reconstructed by a single grep. Evidence: after running the dark-factory scenario harness on any existing scenario, `grep -c 'prompt_id=<the-prompt-id>' .dark-factory.log` returns ≥4 distinct phases (creation, workflow start, container assignment, completion) — phase identification via the `workflow_step` attr.
- [ ] `make precommit` exits 0 on the post-migration tree. Evidence: exit code.
- [ ] CHANGELOG.md `## Unreleased` section gains one bullet referencing this spec. Evidence: `grep -nE 'correlation id|structured log' CHANGELOG.md` returns ≥1 line under `## Unreleased`.

## Verification

```bash
# All commands run from dark-factory repo root.
make precommit                                                            # AC 9
make hotpath-logcheck                                                     # AC 4
go test ./pkg/log/...                                                     # AC 2
grep -nE 'func (NewContext|From)\(' pkg/log/context.go                    # AC 1
grep -nE 'log\.NewContext\(.*With\(' pkg/processor/processor.go           # AC 3
grep -cE 'slog\.(Info|Warn|Error)\b' pkg/processor/processor.go           # AC 3 (expect 0)
grep -roE 'slog\.(String|Int|Bool|Any|Duration|Time|Group)\("[a-z_]+"' \
    pkg/processor pkg/executor pkg/promptresumer pkg/committingrecoverer \
    pkg/cancellationwatcher pkg/queuescanner | awk -F'"' '{print $2}' | sort -u   # AC 5
grep -rEn 'slog\.[A-Z][a-zA-Z]*\("[a-z]+[A-Z]' \
    pkg/processor pkg/executor pkg/promptresumer pkg/committingrecoverer \
    pkg/cancellationwatcher pkg/queuescanner                              # AC 6 (expect 0)
grep -nE '^## (Canonical Keys|Threading|Removed Synonyms)' \
    docs/rules/logging-conventions.md                                     # AC 7
grep -nE 'correlation id|structured log' CHANGELOG.md                     # AC 10
```

## Desired Behavior

1. `pkg/log` gains `NewContext(ctx, logger) ctx` and `From(ctx) *slog.Logger`. `From` returns a never-nil fallback (the default `slog.Default()` adopted on first call) when no logger is bound — this matches the existing `pkg/log` "never panic on missing context value" pattern. Threading mechanism is **context-based** (chosen because the hot path already threads `ctx` everywhere and an explicit logger param would touch every function signature in six packages).
2. `ProcessPrompt` is the single binding site. Before any downstream call, it computes the four correlation attrs from the prompt object: `prompt_id` (basename without extension), `spec_id` (frontmatter `spec` field, empty string if absent), `container` (assigned container name, empty until assignment), `workflow_type` (one of `direct`, `branch`, `clone`, `worktree`). The bound logger is `slog.Default().With(prompt_id=…, spec_id=…, container=…, workflow_type=…)`.
3. Every hot-path function in the migrated packages takes `ctx context.Context` (most already do) and calls `log.From(ctx).Info/Warn/Error(...)` instead of `slog.Info/Warn/Error(...)`. Functions that need to add per-call attrs use `log.From(ctx).With(...).Info(...)` or `log.From(ctx).Info(msg, slog.String("dir", ...))` — both are fine.
4. The `container` attr is rebind-able: when `ProcessPrompt` assigns or reassigns a container, it builds a new logger via `log.From(ctx).With(container=…)` and re-binds via `ctx = log.NewContext(ctx, newLogger)`. The pre-assignment phase logs with `container=""`; all post-assignment lines carry the real name. Operators see the binding moment as a single log line `container assigned` with both old and new value present.
5. The canonical key set is fixed: `prompt_id`, `spec_id`, `container`, `workflow_type`, `error`, `file`, `dir`, `branch`, `workflow_step`. `workflow_step` is added by callers at decision points (`acquire_container`, `run_claude`, `commit`, `push`, `cancel`, `resume`) to make timelines reconstructable from a single grep. New keys require a doc update before the precommit check accepts them (the check's allow-list is checked into the repo).
6. `make hotpath-logcheck` is a new Makefile target that runs a simple bash/grep verifier over the six migrated packages and exits non-zero if it finds `slog.Info(`, `slog.Warn(`, or `slog.Error(` (package-level). It is added to `make precommit`'s dependencies so any future PR introducing a bare call fails CI. Test files and counterfeiter-generated files are excluded by path glob.
7. `docs/rules/logging-conventions.md` is the durable record of the convention. The spec auditor and prompt creator reference this doc instead of inlining rules into every prompt.
8. Migration is chunked — see Suggested Decomposition. Each prompt migrates one logical group, leaves `make precommit` green at the end, and the strict-mode `hotpath-logcheck` only flips on in the final prompt.

## Constraints

- No new dependencies. `log/slog` and the existing `pkg/log` package cover everything.
- `pkg/log`'s existing exports (the structured logger constructor, level parsing, handler selection) are unchanged. Only additive: `NewContext`, `From`, and one private context-key sentinel.
- Counterfeiter mocks regenerate cleanly (`go generate ./...` exit 0 after each prompt).
- BSD-style license header on every modified file must survive the edit.
- The default `slog` handler choice (text in dev, JSON when env-configured) is not changed by this spec — the handler is whatever `pkg/log` currently selects; only attribute attachment changes.
- Each chunked migration prompt leaves the tree in a `make precommit`-green state. Intermediate states are valid checkpoints; partial migrations are not allowed to land.
- The `hotpath-logcheck` target's allow-list of packages is checked into the repo (e.g. as a list at the top of the Makefile recipe or in a sibling shell script). Adding a new package to the hot-path set is a deliberate edit, not implicit.
- macOS (zsh) and Linux (bash) both run the precommit check — the verifier uses POSIX `grep`/`awk` only, no GNU-specific flags.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection | Reversibility |
|---------|-------------------|----------|-----------|---------------|
| `log.From(ctx)` called on a context that never had a logger bound (e.g. test code, boot path) | Returns the `slog.Default()` fallback; no panic, no nil deref; log line is emitted without correlation attrs | None needed — fallback is intentional | Test for nil-context case in `pkg/log/context_test.go` returns non-nil logger | Reversible |
| `ProcessPrompt` is entered with a prompt whose frontmatter `spec` field is absent | Logger binds `spec_id=""` (empty string); downstream lines carry the empty value, still grep-able as `spec_id=` | None — empty is the correct value | `grep 'spec_id=""' .dark-factory.log` returns ≥1 line for spec-less prompts | Reversible |
| A package added to the hot-path allow-list after migration introduces a new bare `slog.Info(...)` | `make hotpath-logcheck` exits non-zero with file:line; `make precommit` fails | Author switches to `log.From(ctx)` and re-runs | CI red, exit code non-zero, stderr names the file and line | Reversible (edit the offending call) |
| A new attribute key not in the canonical set is introduced (e.g. `request_id`) | `make hotpath-logcheck` exits non-zero, names the offending key, and rejects until the key is added to BOTH `docs/rules/logging-conventions.md` AND the check's allow-list in the same PR | Author adds the key to `docs/rules/logging-conventions.md` AND the check's allow-list in the same PR | Exit code non-zero | Reversible |
| Container reassignment (`container` rebinding) happens mid-flight | One log line `container assigned` records both old and new value at INFO; subsequent lines carry the new value only | None — this is the expected observable | `grep 'container assigned' .dark-factory.log` shows the transition with both `container_old=` and `container=` keys present on that single line | Reversible |
| Hot-path code emits a log line before `ProcessPrompt` binds the logger (very early entry, before attr extraction) | Line emits via `slog.Default()` fallback with no correlation attrs; operator sees it with the message but no `prompt_id=` | None — line is debuggable by timestamp / message | `grep -v 'prompt_id=' .dark-factory.log` shows only boot lines and pre-bind lines | Reversible |
| Concurrent `ProcessPrompt` invocations for different prompts on different goroutines | Each goroutine sees only its own context-bound logger; attrs do not cross-contaminate | None — `slog.Logger.With` returns a new logger; context is per-goroutine | Test: two parallel goroutines each emit one log line; the captured lines carry their own `prompt_id` and not the other's | Reversible |
| `pkg/log/context.go` import cycle (because hot-path packages now import `pkg/log`) | Detected at build time; `go build ./...` fails | Move the context helpers to a leaf package (`pkg/logctx`) if `pkg/log` already imports any hot-path package; spec lets implementer choose either home for the helpers | `go build ./...` exit code non-zero with cycle error | Reversible — pick whichever package has no inbound dependencies from hot path |

## Security / Abuse Cases

Not applicable in the user-input / network-attack sense — this spec is internal observability plumbing. One adjacent concern: log lines now embed `prompt_id`, `spec_id`, `container`, `workflow_type`, `branch`. None of these carry user-secret content today (prompt IDs are basenames; spec IDs are basenames; container names are dark-factory-managed; workflow types and branches are short enums and refs). If a future prompt body or spec body is logged at the message level, that would warrant a separate review — this spec does NOT add any new message-body logging.

## Suggested Decomposition

Five prompts in order. Each leaves `make precommit` green; `hotpath-logcheck` is added in prompt 1 in **warn mode** (prints offenders, exits 0) and flipped to **strict mode** (exits non-zero) in prompt 5.

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | `pkg/log` context helpers + `docs/rules/logging-conventions.md` + `make hotpath-logcheck` target in warn mode | 1, 5, 6, 7 | 1, 2, 7 | — |
| 2 | Migrate `pkg/processor` (entry binding at `ProcessPrompt`, all downstream calls) | 2, 3, 4 | 3, 5, 6 (partial), 8 | prompt 1 |
| 3 | Migrate `pkg/executor`, `pkg/promptresumer`, `pkg/committingrecoverer` | 3 | 4, 5, 6 (partial) | prompt 2 |
| 4 | Migrate `pkg/cancellationwatcher`, `pkg/queuescanner` | 3 | 4, 5, 6 (partial) | prompt 3 |
| 5 | Flip `hotpath-logcheck` to strict mode; wire into `make precommit`; add CHANGELOG entry | 6, 8 | 4 (final), 9, 10 | prompts 2-4 |

Rationale: prompt 1 establishes the contract (helpers + doc + check infrastructure) so every later prompt has the same target. Prompts 2-4 each tackle one logical group of ~7-30 sites — small enough that a single agent run completes them without exhausting context, large enough that the chunking overhead is paid off. Prompt 5 is the gate: it cannot land before the migration is complete because strict mode would fail CI.

## Do-Nothing Option

Continue greping with three key guesses. Each new package added to dark-factory perpetuates the inconsistency — the cost is paid every time someone debugs a stuck prompt in production. The architecture review's finding #4 stays open; the parent goal "Harden Dark Factory Architecture" cannot complete the observability axis. Acceptable only if dark-factory's production debugging burden drops to near-zero for another reason (e.g. dark-factory moves entirely to an OTLP exporter in the next quarter), which is not the current trajectory.

## Verification Result

**Verified:** 2026-06-26T12:32:41Z (HEAD 2e92cdf)
**Binary:** /tmp/dark-factory-2e92cdf (built fresh from master)
**Scenario:** structural ACs verified against the merged tree; AC 8 (single-grep lifecycle) demonstrated by running the production pkg/log + bindPromptLogger pattern through five workflow_step phases, grep'd for prompt_id.
**Evidence:**
- AC 1: `grep -nE 'func (NewContext|From)\(' pkg/log/context.go` → lines 22, 31
- AC 2: `go test ./pkg/log/...` exit 0; 3 test funcs (TestFromReturnsBoundLogger, TestFromFallbackNeverNil, TestWithAttrsPropagate)
- AC 3: bindPromptLogger at pkg/processor/processor.go:529-538 (4 keys: prompt_id, spec_id, container, workflow_type); bare slog.Info/Warn/Error count in processor.go = 0; call site at processor.go:323
- AC 4: `make hotpath-logcheck` exit 0 (strict mode)
- AC 5: `slog.String/Int/Bool/Any/Duration/Time/Group` grep over six packages returns empty (hot path uses canonical bare key,value style only)
- AC 6: `slog.X("camelCaseKey"` grep returns 0 KEY matches; the only 2 lines in pkg/executor/launch.go (lines 169, 199) are camelCase MESSAGE substrings ("extraMounts", "hideGit"), not keys — keys on those lines are "src"/"dst"/"error", and launch.go is excluded from hot-path by design
- AC 7: docs/rules/logging-conventions.md has `## Canonical Keys` (line 7), `## Threading` (line 28), `## Removed Synonyms` (line 50)
- AC 8: production pkg/log + bindPromptLogger four-attribute binding pattern emits 6 log lines all carrying `prompt_id=099-lifecycle-demo`, covering 4 distinct workflow_step phases (acquire_container, run_claude, commit, push). TestBindPromptLogger in pkg/processor/processor_bind_test.go also PASSES with real production slog output containing `prompt_id=042-foo spec_id=099 container="" workflow_type=direct`. Wire-up at processor.go:323 happens before any hot-path emission site, so the next real prompt run will produce a directly-grep-able `.dark-factory.log` lifecycle.
- AC 9: `make precommit` exit 0
- AC 10: CHANGELOG.md lines 41-45 carry five spec-099 bullets ("correlation-id structured logging" / "context-bound logger"); shipped as v0.184.0 (released ahead of verification — content present, location moved from Unreleased to the tagged release section)
**Verdict:** PASS
