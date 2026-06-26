---
status: completed
approved: "2026-06-26T07:19:46Z"
generating: "2026-06-26T07:19:47Z"
prompted: "2026-06-26T07:26:00Z"
verifying: "2026-06-26T12:40:33Z"
completed: "2026-06-26T12:41:31Z"
branch: dark-factory/centralize-subprocess-runner
---

## Summary

- dark-factory currently spawns external processes through five inconsistent patterns: a bounded runner, two ad-hoc `exec.CommandContext` sites, two `exec.Command` calls that take no context (can hang forever), and a docker-specific signal-protocol runner.
- The non-docker call sites lose warn-on-slow telemetry, lose timeouts, or — in the worst case — lose cancellability entirely.
- Make the existing `pkg/subproc.Runner` the sole spawn API for non-docker subprocesses; migrate the four bad sites; preserve docker-specific runners untouched.
- Add a hot-path lint gate (`make hotpath-execcheck`) modelled on `hotpath-logcheck` that fails CI if a hot-path package grows a raw `exec.Command(Context)?` call.
- No public API changes; existing Counterfeiter mocks regenerate cleanly; exit-code semantics preserved end-to-end.

## Problem

A 2026-06-25 architecture review (dimensions pass, finding #5 / cross-cutting consistency) found five different subprocess patterns scattered across the codebase. Each was a reasonable local decision; together they mean git operations silently slow without warning, the dirty-file check can hang on a broken filesystem, project-name resolution can hang forever because it takes no context, and the docker executor maintains a parallel implementation with its own signal protocol. Operators have no single place to read or change subprocess policy (warn threshold, timeout, cancellation semantics). New call sites added by future work will continue to drift unless centralization is enforced at lint time.

## Goal

Every non-docker subprocess spawn in `pkg/` runs through `pkg/subproc.Runner`. Docker-CLI spawn sites — which require their own SIGINT-then-SIGKILL escalation — remain in `pkg/executor` and are explicitly allow-listed. A `make hotpath-execcheck` gate, wired into `make precommit` in strict mode, fails the build if a new raw `exec.Command(Context)?` call appears outside `pkg/subproc/` and the docker allow-list. Public behaviour visible to callers (exit-code surfacing, stderr truncation at 8 KiB with `(truncated)` marker) is preserved across the migration.

## Non-goals

- Do NOT replace the docker CLI executor (`pkg/executor/executor.go`) — it has its own signal protocol; that's a separate concern.
- Do NOT add subprocess sandboxing (seccomp, AppArmor) — future hardening, separate spec.
- Do NOT add a per-package opt-out flag — invariant; if a future package legitimately needs raw `exec.Command`, that's a separate spec.
- Do NOT migrate `buca` / `make` / shell wrappers under `scripts/` — they are operator scripts, not Go code.
- Do NOT change `pkg/subproc.Runner`'s exported interface signatures. New helpers may be added but no existing method renamed, removed, or have its signature changed.

## Acceptance Criteria

- [ ] No raw `exec.Command` or `exec.CommandContext` call exists in `pkg/` outside `pkg/subproc/` and the docker allow-list — evidence: `bash scripts/hotpath-execcheck.sh strict` exits 0 from the repo root; deliberately injecting `exec.CommandContext(ctx, "git", "x")` into `pkg/git/git.go` and rerunning the script exits non-zero and names the offending file:line on stderr.
- [ ] `make hotpath-execcheck` exists as a Makefile target and runs the strict-mode script — evidence: `grep -nE '^hotpath-execcheck:' Makefile` returns 1 line and `make hotpath-execcheck` exits 0 on the migrated tree.
- [ ] `make precommit` invokes `hotpath-execcheck` — evidence: `grep -nE '^precommit:' Makefile` returns a line containing the token `hotpath-execcheck`; running `make precommit` on the migrated tree exits 0.
- [ ] Project-name resolution honours context cancellation — evidence: a unit test in `pkg/project` cancels the context before calling the resolver and asserts the function returns within 500 ms with a non-nil error wrapping `context.Canceled`. (500 ms accommodates CI runner load; cancellation latency is still semantically tight.) Test name is reported by `go test -v ./pkg/project/... -run CancelledCtx` and the output contains `PASS`.
- [ ] Dirty-file check honours a timeout — evidence: a unit test in `pkg/processor` injects a fake runner whose subprocess sleeps past the configured timeout; the test asserts the dirty check returns within `2 × DefaultTimeout` (upper bound 20 s) with an error matching `context.DeadlineExceeded`. `go test -v ./pkg/processor/... -run DirtyTimeout` output contains `PASS`.
- [ ] `pkg/git` operations route through `pkg/subproc.Runner` — evidence: `grep -nE 'exec\.Command(Context)?\(' pkg/git/*.go` returns 0 matches; an integration test in `pkg/git` provides a fake `subproc.Runner` and asserts every public method calls `RunWithWarnAndTimeout*` exactly once per spawn. `go test -v ./pkg/git/... -run RunnerInjected` output contains `PASS`.
- [ ] Stderr truncation behaviour is preserved — evidence: an integration test feeds a 16 KiB stderr stream through the migrated code path and asserts the returned error message contains `(truncated)` and is at most 8 KiB + a short suffix. `go test -v ./pkg/git/... -run TruncateStderr` output contains `PASS`.
- [ ] Counterfeiter mocks regenerate cleanly — evidence: `make generate` exits 0 and `git status --porcelain mocks/` shows zero modified files after the migration is complete (no stale mocks left over).
- [ ] Docker-CLI spawn sites remain in `pkg/executor` and are exempt — evidence: `bash scripts/hotpath-execcheck.sh strict` exits 0 even though `pkg/executor/{checker,stopper,executor,launch}.go` still contain `exec.CommandContext` calls; the allow-list is encoded as explicit file paths in the script (not by extension).
- [ ] Exit-code semantics unchanged — evidence: in `pkg/git`, a unit test runs `git -c invalid.option=x status` through the migrated path and asserts the returned error wraps an `*exec.ExitError` whose `ExitCode()` equals the same value observed before migration (captured in the test as a constant). `go test -v ./pkg/git/... -run ExitCodePropagated` contains `PASS`.

## Verification

```
make hotpath-execcheck      # exit 0 on migrated tree
make precommit              # exit 0; runs hotpath-execcheck as part of the chain
go test ./pkg/project/... ./pkg/processor/... ./pkg/git/... ./pkg/gitprovider/...
```

Additional gate test (manual, for spec verification):

```
# Inject a temporary raw exec call and confirm the gate fails:
printf '\nfunc tempRaw() { _ = exec.CommandContext(nil, "x") }\n' >> pkg/git/git.go
bash scripts/hotpath-execcheck.sh strict   # must exit non-zero
git checkout -- pkg/git/git.go
```

## Desired Behavior

1. `pkg/subproc.Runner` is the single spawn primitive for non-docker subprocesses. Existing `RunWithWarnAndTimeout` / `RunWithWarnAndTimeoutDir` remain. If a new spawn shape is needed (e.g. one-shot fire-forget without warn semantics), it is added as a new method, not by changing existing ones.
2. `pkg/project/name.go` resolves the project name through `subproc.Runner`, passing a real `context.Context`. The two former `exec.Command(...)` sites (lines 52, 66 at spec time) accept and honour ctx cancellation.
3. `pkg/processor/dirty.go` runs its `git status --short` probe through `subproc.Runner` with the default timeout. A stuck filesystem does not stall the processor loop.
4. `pkg/git/git.go`, `pkg/git/cloner.go`, `pkg/git/brancher.go`, `pkg/git/collaborator_fetcher.go`, and any sibling `pkg/git/*.go` files spawn exclusively through `subproc.Runner`. The 8 KiB `truncateStderr` helper is either preserved as a package-private helper in `pkg/git` or absorbed into `pkg/subproc` — agent decides at impl time which placement is cleaner.
5. `pkg/gitprovider/bitbucket/remote.go` spawns through `subproc.Runner` (it is a non-docker, non-allow-listed site).
6. `pkg/executor/command.go` retains its signal-protocol-aware `commandRunner` interface because docker stop/kill escalation depends on it. The migration only verifies whether the spawn primitive itself (`exec.Cmd` construction) could go through `subproc.Runner` without breaking the SIGINT-then-SIGKILL protocol; if it cannot without churn, it stays as-is and is added to the allow-list with an inline comment naming the reason.
7. `scripts/hotpath-execcheck.sh` exists, mirrors the shape of `scripts/hotpath-logcheck.sh` (warn / strict modes), greps for `exec\.Command(Context)?\(` outside `pkg/subproc/`, applies the docker allow-list, and skips `_test.go` files and counterfeiter-generated files.
8. The allow-list is encoded as explicit file paths in the script. The exact set at spec time is: `pkg/executor/checker.go`, `pkg/executor/stopper.go`, `pkg/executor/executor.go`, `pkg/executor/launch.go`, and — if Desired Behavior 6 keeps it untouched — `pkg/executor/command.go`. Adding a file to the allow-list later requires a separate spec.

## Constraints

- No new Go dependencies. `pkg/subproc` already exists.
- `pkg/subproc.Runner`'s exported interface is frozen: existing method names and signatures do not change. New methods may be added.
- Counterfeiter mocks under `mocks/` regenerate cleanly via `make generate`; no hand edits.
- Docker-CLI spawn sites in `pkg/executor/{checker,stopper,executor,launch}.go` are NOT in scope for migration. The allow-list keeps them exempt.
- `truncateStderr` semantics — 8 KiB cap with a literal `(truncated)` suffix — must be preserved end-to-end. A migrated git call producing a long stderr returns an error message indistinguishable in shape from the pre-migration behaviour.
- Exit-code semantics are preserved: callers using `errors.As` to unwrap `*exec.ExitError` and read `ExitCode()` continue to work. Wrapping is allowed; replacement of the underlying error type is not.
- The new script follows the existing convention in `scripts/hotpath-logcheck.sh`: POSIX shell, `warn` and `strict` modes, runs from repo root, skips test files and generated files.

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery | Reversibility |
|---|---|---|---|---|
| New PR adds raw `exec.CommandContext` to `pkg/git` | `make precommit` (CI) | `hotpath-execcheck strict` exits non-zero; names file:line on stderr | Author replaces with `subproc.Runner`; reruns `make precommit` | Reversible — never lands |
| Migration accidentally drops a docker-specific spawn from the allow-list | `make precommit` (CI) | Gate fails on a known-good file | Add file back to allow-list in `scripts/hotpath-execcheck.sh` with comment naming reason; rerun | Reversible |
| Migration changes `truncateStderr` cap silently | `go test ./pkg/git/... -run TruncateStderr` | Truncation test fails — output > 8 KiB + suffix or missing `(truncated)` marker | Restore 8 KiB cap; ensure the helper is used on the post-Runner error path | Reversible |
| `subproc.Runner`'s default 10 s timeout is too tight for `git clone` of a large repo | First slow clone in production logs a "subprocess skipped" warn and returns `context.DeadlineExceeded` to caller | Caller (`pkg/git/cloner.go`) surfaces the timeout error to the prompt; prompt fails fast instead of hanging | Adopt `NewRunnerWithThresholds` at the clone call site with a larger timeout — does NOT need a spec, callers choose their thresholds | Reversible |
| Counterfeiter regeneration produces a diff for the runner mock | `git status mocks/` after `make generate` | Diff appears in PR review | Commit the regenerated mock; no behaviour change | Reversible |
| Concurrency: two daemons call `subproc.Runner` simultaneously | `subproc.Runner` is stateless — each call constructs its own `exec.Cmd`; no shared mutable state | No interaction; both succeed or fail independently | `go test -race -v ./pkg/subproc/... -run Concurrent` reports no data race; output contains `PASS` | Reversible — race appears as red CI; revert offending caller |

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Add `hotpath-execcheck` gate script + Makefile target, wired into `precommit`, allow-list seeded with current docker-CLI sites; gate runs in *warn* mode initially so the rest of the migration proceeds without blocking. | 7, 8 | 2, 3, 9 | — |
| 2 | Migrate `pkg/project/name.go` and `pkg/processor/dirty.go` to `subproc.Runner` (highest-impact: name.go currently has no ctx at all). | 2, 3 | 4, 5 | prompt 1 (gate exists) |
| 3 | Migrate `pkg/git/*.go` (git.go, cloner.go, brancher.go, collaborator_fetcher.go) and `pkg/gitprovider/bitbucket/remote.go`; place or absorb `truncateStderr`. | 4, 5 | 6, 7, 10 | prompt 1 |
| 4 | Audit `pkg/executor/command.go` against `subproc.Runner`. If clean migration possible without breaking the SIGINT-then-SIGKILL protocol, migrate; otherwise add to allow-list with inline comment. | 6 | 9 (allow-list path) | prompt 1 |
| 5 | Flip `hotpath-execcheck` from `warn` to `strict` once prompts 2-4 land; verify `precommit` green; regenerate counterfeiter mocks. | 1 | 1, 8 | prompts 2, 3, 4 |

Rationale: prompt 1 lands the gate first so subsequent migrations have a build-time guard against regressions. Prompts 2 and 3 can run in parallel after prompt 1 — they touch disjoint packages. Prompt 4 is a judgement call isolated to one file. Prompt 5 is the strict-mode flip and is the last gate; running it earlier would block prompts 2-4.

## Do-Nothing Option

The current code keeps working — the four bad sites are all in low-traffic paths. The architecture review's concern is consistency and future drift, not a live incident. Cost of not doing this: every new spawn site is one more place where an author has to pick between five patterns and is likely to copy the worst nearby example. Project-name resolution remains a latent hang risk on a slow git remote. Dirty-file check remains a latent hang risk on a broken filesystem. Both have been observed once each in operator anecdote but not as outage-class events. Acceptable to defer for one quarter; not acceptable to defer indefinitely.

## Verification Result

**Verified:** 2026-06-26T12:40:33Z (HEAD f71ce1c)
**Binary:** /tmp/dark-factory-f71ce1c (`dark-factory dev` built from HEAD)
**Scenario:** Direct execution of spec `## Verification` commands plus AC-specific gate/test artifacts; no scenario file referenced.
**Evidence:**
- AC 1: `bash scripts/hotpath-execcheck.sh strict` exit=0 on clean tree; after injecting `exec.CommandContext(nil,"x")` into `pkg/git/git.go`, exit=1 with stderr `hotpath-execcheck: raw exec.Command(Context) found ... pkg/git/git.go:274:func tempRaw() { _ = exec.CommandContext(nil, "x") }`; reverted, exit=0.
- AC 2-3: `grep -nE '^hotpath-execcheck:' Makefile` → `54:hotpath-execcheck:` (1 line); `grep -nE '^precommit:' Makefile` → `16:precommit: ... hotpath-execcheck ...`; `make hotpath-execcheck` exit=0; `make precommit` exit=0 (full chain incl. addlicense + check-changelog).
- AC 4: `go test -v ./pkg/project/... -ginkgo.focus="CancelledCtx"` → `Ran 1 of 12 Specs ... SUCCESS! -- 1 Passed | 0 Failed`.
- AC 5: `go test -v ./pkg/processor/... -ginkgo.focus="DirtyTimeout"` → `Ran 1 of 196 Specs ... SUCCESS! -- 1 Passed | 0 Failed`.
- AC 6: production `pkg/git/*.go` (excluding `_test.go`) has zero `exec.Command(Context)?(` matches; `-ginkgo.focus="RunnerInjected"` → `Ran 7 of 227 Specs ... SUCCESS!`.
- AC 7: `-ginkgo.focus="TruncateStderr"` → `Ran 1 of 227 Specs ... SUCCESS!`.
- AC 8: `make precommit` (which invokes `make generate` + `make addlicense`) exit=0; post-run `git status --porcelain mocks/` empty.
- AC 9: `bash scripts/hotpath-execcheck.sh strict` exit=0 despite raw `exec.CommandContext(` in `pkg/executor/checker.go:87`, `pkg/executor/stopper.go:32`, `pkg/executor/executor.go` (8 sites), and `pkg/executor/launch.go` — allow-list working.
- AC 10: `-ginkgo.focus="ExitCodePropagated"` → `Ran 1 of 227 Specs ... SUCCESS!`.
- External: PRs [#38](https://github.com/bborbe/dark-factory/pull/38) (prompts 1-4), [#40](https://github.com/bborbe/dark-factory/pull/40) (prompt 5 strict flip), [#41](https://github.com/bborbe/dark-factory/pull/41) (preflight stderr regression fix) all MERGED; `Linked Prompts: 5/5` from `dark-factory spec show 100`.
**Verdict:** PASS
