---
status: approved
tags:
    - dark-factory
    - spec
approved: "2026-07-12T18:29:05Z"
generating: "2026-07-12T18:37:26Z"
branch: dark-factory/local-execution-backend
---

## Summary

- Add a second execution backend — `local` — that runs the LLM steps (prompt execution, and generation) as a **local subprocess** in the current process/cwd instead of spawning a Docker container.
- Selected via a layered config field `backend: docker|local` (default `docker`), same precedence as `maxContainers` / `hideGit` (global ← project ← `--set`).
- Implements the existing backend-neutral `Executor` (and, as needed, `ExecutionChecker` / `ExecutionStopper`) interfaces — no new interface. This is the "separate spec" that `102-executor-backend-neutral-naming` explicitly deferred, and follows its `docs/execution-backends.md` § Adding a Backend blueprint (≤3 files).
- Everything else is unchanged: git orchestration (clone / branch / `merge origin/master` / commit / push) already lives in the Go binary, and the daemon/run lifecycle (generate → approve → execute → complete) is identical. Only "how the claude step is launched" changes: local subprocess vs `docker run`.

## Problem

`dockerExecutor` is the only `Executor` today: every LLM step does `docker run <claude-yolo image>`. That is correct on a laptop, but wrong when dark-factory itself runs **inside** a container. The `github-dark-factory-agent` (cluster implementer, [[Lift Dark-Factory Daemon into Agent Framework]]) is a k8s Job whose pod IS a claude-yolo-equivalent image (claude CLI + Go toolchain + git/gh baked in). Having it spawn *nested* claude-yolo containers is Docker-in-Docker: it needs a mounted `docker.sock` / privileged access, doubles image-pull + startup latency per prompt, and is redundant because the pod already has the entire execution environment. The goal's Non-goals bar DinD outright. Spec 102 built and proved the neutral seam but left the impl to a follow-up — this one.

## Goal

With `backend: local`, dark-factory runs prompt generation and prompt execution by invoking `claude` (and the prompt's DoD commands) directly in the current environment — no `docker run`, no mounts, no nested container — and returns the same structured result, writing the same log files. With `backend: docker` (the default, unset behavior) nothing changes. A repo/agent that runs dark-factory inside a suitably-equipped container opts in with one config line; the same code ships both modes.

## Non-goals

- Removing, changing, or deprecating `dockerExecutor`. It stays the default and is untouched at runtime.
- Auto-detecting the backend ("am I inside a container?"). The operator opts in explicitly via config; guessing is out of scope.
- Sandboxing / isolation for the local backend (gVisor, seccomp, firewall/tinyproxy). Local execution trusts its host environment by design — it is only for already-sandboxed callers (the agent pod).
- Any change to git handling, the spec/prompt lifecycle, workflow separation modes (`direct`/`branch`/`worktree`/`clone`), or the on-disk contract.
- Provisioning the toolchain. The local backend assumes `claude` + required tools already exist in `PATH`; it does not install them (that is the image's job).

## Assumptions

- The neutral `Executor` / `ExecutionChecker` / `ExecutionStopper` interfaces from spec 102 are stable and backend-agnostic (verified: `pkg/executor/executor.go`, `docs/execution-backends.md`).
- The factory constructs the executor/checker/stopper at the `NewDockerExecutor` / `NewDockerExecutionChecker` / `NewDockerExecutionStopper` call sites in `pkg/factory/factory.go` and these are the only wiring points (exact lines drift on edits — see `docs/execution-backends.md` § Adding a Backend for the frozen file list).
- The generator (`dockerSpecGenerator`, `pkg/generator/generator.go`) holds an injected `executor.Executor` and runs generation via `executor.Execute` — verified — so selecting the local backend in the factory routes generation in-process automatically, with no generator change.
- The prompt content, temp-file handoff, and the formatter/log pipeline in `dockerExecutor.Execute` are reusable by a local backend (only the process-launch differs: `claude …` vs `docker run … claude …`).
- `claude` in the current `PATH`, run with `CLAUDE_CONFIG_DIR` / auth already present, produces the same JSONL stream `dockerExecutor` parses today.

## Desired Behavior

1. **Config field.** New layered field `backend` with values `docker` (default) and `local`, resolvable from global config, project `.dark-factory.yaml`, and `--set backend=local`, same precedence + `*Source` reporting as `maxContainers`.
2. **Effective-config log.** Startup effective-config line emits `backend=<docker|local>` and `backendSource=<default|global|project|cli>`.
3. **Local executor.** A new `pkg/executor/local_subprocess.go` with `localSubprocessExecutor` implementing `Executor`:
   - `Execute(ctx, promptContent, logFile, executionID)` writes the same prompt temp file, then runs `claude` as a **local subprocess** in the current working directory (already the checked-out repo), streaming stdout through the SAME formatter/raw-log pipeline `dockerExecutor` uses. No `docker run`, no bind mounts.
   - `Reattach` — the local backend does **NOT** support reattach: a local subprocess dies with the dark-factory process, so there is nothing to re-attach to. `Reattach` returns a typed sentinel (`ErrReattachUnsupported`); the caller (`pkg/promptresumer`) treats a local-backend execution as not-reattachable and recovers by **re-running** the prompt — safe because execution commits per prompt, so a restarted run resumes from the last committed prompt. Never silently succeed.
   - `StopAndRemoveContainer(ctx, executionID)` — terminate the child process group (SIGTERM → grace → SIGKILL), the local analog of docker stop/kill.
   - `ExecutionChecker` / `ExecutionStopper` local impls live in the same file with `NewLocalSubprocess*` constructors (agent-decides which are actually needed for the local backend — discoverable at impl time from what the resumer/cancellation paths require).
4. **Factory switch — covers execution AND generation.** `pkg/factory/factory.go` selects the local constructors instead of the docker ones when `backend == local`, at the `NewDocker*` call sites. Because the generator takes the same injected `executor.Executor` (see Assumptions), this single switch routes BOTH prompt execution and spec→prompts generation in-process — no separate generation wiring. No other factory line changes; no caller package changes (they depend only on the neutral interfaces).
5. **Fail-closed on a missing environment.** With `backend: local`, if `claude` (or a required tool) is not on `PATH`, fail with a clear, actionable error naming the missing binary — never fall back to docker silently, never run a broken command.
6. **Healthcheck / preflight.** The docker-specific probes (`docker run boot/claude/mount`) do not apply to `local`. Under `backend: local` they are skipped or replaced with a local-equivalent check (agent-decides: skip vs. a `claude --version` probe — local, reversible). No docker daemon is required when `backend: local`.

## Constraints

- Default (`backend` unset) behavior is byte-for-byte identical to today — `dockerExecutor` runs, same labels, same argv. No regression for existing docker users.
- Reuse the existing neutral interfaces — introduce NO new interface. Container vocabulary stays confined to the docker packages (do not re-leak; the `hotpath-execution-naming-check` gate from spec 102 must still pass).
- Keep the change small — the executor impl, the factory switch, the config field, plus the generation-path routing and healthcheck adaptation. If it sprawls beyond that, the abstraction (spec 102) was leakier than documented — stop and reassess.
- No docker daemon may be required at runtime when `backend: local` (so it works in a pod with no docker.sock).
- `make precommit` passes; counterfeiter mocks regenerate cleanly.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| `backend` unset | Defaults to `docker`; unchanged | Expected |
| `backend: local`, `claude` not on PATH | Fail-closed at startup/first-exec with "claude not found on PATH; backend: local requires claude in the environment" — surfaced as a non-zero exit + a single ERROR log line naming the missing binary | Operator fixes the image / PATH |
| dark-factory (pod) restarts mid-local-execution | Child died with the process; `Reattach` returns `ErrReattachUnsupported`; the restarted run sees the prompt still uncommitted/`in-progress` and re-runs it — per-prompt commits make re-run safe | Automatic (re-run) |
| `backend: local`, prompt DoD needs a tool absent from PATH | Prompt fails with the underlying tool error (same as today inside a container missing the tool) | Add the tool to the image |
| `backend: local` on a host without docker | Works — no docker needed | Expected (the point) |
| `backend: docker` on a host without docker | Same as today (docker required) | Expected |
| Invalid `backend` value in YAML/CLI | Startup validation error listing valid values (`docker`, `local`) | Operator fixes config |
| Child subprocess hangs past `maxPromptDuration` | `StopAndRemoveContainer` kills the process group (SIGTERM→SIGKILL) — same timeout semantics as docker | Automatic |

## Security / Abuse Cases

The whole point of this feature is to **trade container isolation for in-process execution** — so the trust boundary must be explicit.

- **Local backend runs `claude` (and the prompt's DoD commands) with the FULL credentials and filesystem of the dark-factory process** — no container sandbox. Spec- and prompt-driven content is executed directly against the host. This is acceptable ONLY because the intended caller (the `github-dark-factory-agent` k8s Job pod) is *itself* the isolation boundary: a single-tenant, per-task, ephemeral pod with scoped credentials.
- **Abuse case — `backend: local` on a shared / multi-tenant / developer host.** An operator enabling `local` outside an already-isolated caller exposes that host's credentials + filesystem to prompt content. Mitigation: `backend` defaults to `docker` (opt-in only); document in `docs/execution-backends.md` that `local` MUST run only inside an already-isolated single-tenant environment; the feature does NOT auto-enable and does NOT auto-detect "am I in a container" (Non-goals) — the operator makes an explicit, documented choice.
- **No new untrusted input crosses a boundary that docker didn't already cross** — the same prompt content already runs claude with repo write access inside the container today; `local` removes the container wall but not the content's existing capabilities. The delta is *host* exposure, addressed by the single-tenant-caller constraint above.
- **Fail-closed, never fall back** (Desired Behavior 5): a misconfigured `local` on a host without `claude` must error, not silently switch to docker (which could mask that the operator's isolation assumption is wrong).

## Acceptance Criteria

- [ ] `backend` is a layered config field, values `docker|local`, default `docker`, resolvable global/project/`--set` with the `maxContainers` precedence + `*Source` reporting — evidence: table test asserts resolved value + `*Source` for each of default / global / project / `--set`.
- [ ] Startup effective-config log includes `backend=…` and `backendSource=…`.
- [ ] With `backend` unset or `docker`, the docker container is spawned exactly as today (labels, image, argv unchanged) — no behavior change — evidence: a golden test on the built docker argv is byte-identical to pre-change.
- [ ] With `backend: local`, prompt **execution** runs `claude` as a local subprocess in cwd — no `docker run` is invoked (evidence: a test/interception asserts the docker CLI is never called; the same prompt runs and produces the parsed result + log files).
- [ ] With `backend: local`, prompt **generation** (spec→prompts) also runs in-process (no `docker run`).
- [ ] Backend selection is the only factory change — `pkg/promptresumer` is the ONE allowed caller-package exception: it gains a single `errors.Is(err, ErrReattachUnsupported)` → re-queue branch (the restart-recovery path); every other caller (`pkg/runner`, `pkg/processor`, …) is untouched and compiles against the unchanged neutral interfaces — evidence: `git diff --stat pkg/runner pkg/processor` is empty; `pkg/promptresumer` shows ONLY the small sentinel-branch addition; `go build ./...` exits 0.
- [ ] `backend: local` with `claude` absent from PATH fails closed with a clear error; never falls back to docker.
- [ ] No docker daemon is required at runtime when `backend: local` (verified: run a prompt with docker unavailable).
- [ ] The `hotpath-execution-naming-check` gate still passes (no container vocabulary leaked into neutral packages).
- [ ] Unit tests cover the local executor — evidence: `TestLocalExecute` asserts a subprocess ran + result parsed with no docker call; `TestLocalMissingClaudeFailsClosed` asserts the error string contains `claude not found on PATH`; `TestLocalStopKillsProcessGroup` asserts SIGTERM→SIGKILL on the child; `TestReattachUnsupported` asserts `Reattach` returns `ErrReattachUnsupported`; all exit 0.
- [ ] `make precommit` passes; `go generate ./...` leaves mocks clean — evidence: both exit 0 and `git status --porcelain pkg/` shows zero modified mocks.
- [ ] promptresumer recovery is tested — a fake executor whose `Reattach` returns `executor.ErrReattachUnsupported` causes the prompt to be re-queued (`MarkApproved` observed) and `resumePrompt` returns nil (daemon not failed); a control case asserts a non-sentinel reattach error still returns the wrapped "reattach to container" error.

## Suggested Decomposition

Multi-layer spec — prompt ordering for the prompt-creator (generation needs no layer of its own: it rides the factory switch via the injected `Executor`):

| # | Prompt focus | Covers DB | Covers AC | Depends on |
|---|---|---|---|---|
| 1 | Config field `backend` (docker\|local, default docker) + layering + validation + effective-config log | 1–2 | 1–2 | — |
| 2 | `localSubprocessExecutor` in `pkg/executor/local_subprocess.go` — `Execute` (subprocess + formatter pipeline), fail-closed on missing claude, `Reattach`→`ErrReattachUnsupported`, `StopAndRemove` process-group kill; + local `ExecutionChecker`/`ExecutionStopper` if needed; unit tests | 3, 5 | 4, 7, 10 | 1 |
| 3 | Factory switch at the `NewDocker*` sites behind `backend` — routes BOTH execution and generation; golden-argv + caller-diff guards | 4 | 3, 5, 6 | 1, 2 |
| 4 | Healthcheck/preflight adaptation for `local` (skip docker probes / optional `claude --version`); no-docker-daemon path | 6 | 8, 9 | 3 |

Note: the `docs/execution-backends.md` blueprint promised "≤3 files" for the executor+factory+config triad; this spec knowingly adds the healthcheck/preflight adaptation (prompt 4) as a 4th touch, since the docker probes are meaningless under `local`. Executor/factory/config stay within the promised triad.

## Verification

```
make precommit
```

Manual (the real target): build the `github-dark-factory-agent` image (claude-yolo base + `go install dark-factory@<this-version>`), run a dark-factory spec inside it with `--set backend=local --set hideGit=true` against a cloned draft-PR branch, and confirm: (a) no nested container is created (`docker ps` on the host shows none spawned by the run), (b) generation + execution complete and commit, (c) with docker.sock absent the run still succeeds.

## Do-Nothing Option

Leave `dockerExecutor` as the only backend. The `github-dark-factory-agent` cluster pipeline cannot avoid Docker-in-Docker — it would need a privileged pod with a mounted `docker.sock` to spawn nested claude-yolo containers, contradicting the goal's Non-goals and adding per-prompt image-pull latency. Without this backend the entire cluster-execution goal ([[Lift Dark-Factory Daemon into Agent Framework]]) stays blocked. Spec 102 already paid the cost of making the seam clean specifically so this backend would be a small, low-risk addition — doing nothing wastes that investment.
