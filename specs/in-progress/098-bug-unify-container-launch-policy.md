---
status: prompted
approved: "2026-06-24T18:54:23Z"
generating: "2026-06-24T19:17:17Z"
prompted: "2026-06-24T19:17:17Z"
branch: dark-factory/bug-unify-container-launch-policy
---

## Summary

- On OrbStack, `dark-factory daemon` startup aborts at the claude healthcheck probe with `claude session probe failed: stdout=""`, because the probe launches the `claude-yolo` container without the Linux capabilities its entrypoint requires (`NET_ADMIN`, `NET_RAW`).
- Root cause is architectural, not "a forgotten flag": the per-invocation `ContainerLaunchOpts` value (which feeds the shared argv builder) is constructed at two independent sites — the executor's prompt-run path and the healthcheck probes' launch path. Only the executor site sets `CapAdd`. The probes' intermediate struct does not even carry a capability field.
- The probes' stated contract — "exercise the same launch path production uses" — is therefore broken by construction. Any future drift (new env var, new mount, new label, new capability) added at the executor site silently bypasses the probe site, so the healthcheck either falsely passes (probe doesn't exercise the flag, real prompts fail) or falsely fails (probe exercises a flag executor doesn't use).
- Fix collapses the launch surface to a single source of truth — one shared "every dark-factory container needs these flags" policy value, derived once from config + environment, consumed identically by the executor and the probes. Per-invocation differences (container name, entrypoint, command, prompt-only mounts, label overlay) are passed in alongside, not parallel-constructed.
- Bug verification replays the original OrbStack startup; architectural verification asserts that `NET_ADMIN` appears in exactly one non-test file and that `ContainerLaunchOpts{...}` is composed in exactly one non-test site.

## Problem

The daemon's healthcheck gate is supposed to guarantee that if it passes, real prompt containers will boot the same way the probe boots. Today that guarantee is structural fiction: the argv builder is shared, but the input struct it consumes is built twice, in two files, by two different code paths, with two different field sets. Only one of those paths sets the Linux capabilities `claude-yolo` requires to run its iptables-based egress firewall. The visible symptom is a daemon that refuses to start on OrbStack — a supported development environment — without the user reaching for `--skip-healthcheck`. The deeper problem is that the gate has no mechanism to stay honest: every future addition to the executor's launch shape is a latent probe drift.

## Goal

The set of launch-shape inputs that are intrinsic to "this is a dark-factory container" — image, project identity, mounts, base environment, capabilities — is materialised exactly once per daemon process and consumed identically by the executor and by the healthcheck probes. The `claude-yolo` container that the probe launches boots with the same capabilities, same mounts, and same base environment as the container the executor launches for a real prompt. A reader who wants to add a new launch-shape concern (new cap, new mount, new env var, new label) has exactly one place to add it, and both the probe and the executor pick it up with no further changes. The daemon starts cleanly on OrbStack without `--skip-healthcheck`.

## Non-goals

- Do NOT add capabilities beyond `NET_ADMIN` and `NET_RAW` — the current set is the canonical set; if a future container needs more, that is a separate spec.
- Do NOT make capabilities configurable via `.dark-factory.yaml` — capabilities are intrinsic to the container image, not a per-project knob. If a future consumer demands variation, that is a separate spec.
- Do NOT refactor `BuildDockerRunArgs` itself — the argv assembly is already shared and correct.
- Do NOT add capabilities to the `docker version` or `docker image inspect` probes — those do not invoke `claude-yolo`.
- Do NOT migrate to docker-compose, libpod, or any higher-level container abstraction.
- Do NOT touch the executor's prompt-file mount insertion, `validateClaudeAuth`, or post-run commit logic.
- Do NOT introduce a feature flag or opt-out for the unified launch policy — escape hatches on the very behavior the fix ships re-create the divergence this spec removes.

## Reproduction

Dark-factory version: feature worktree `feature/unify-container-launch` of `bborbe/dark-factory` (HEAD of the worktree at spec-creation time). The bug also reproduces on `master` at the same SHA the worktree branched from.

Environment: OrbStack on macOS (Linux VM backend; OrbStack rejects iptables syscalls without explicit caps).

### Smallest reproduction (container-level, isolates the cap requirement)

```bash
docker run --rm -e DEBUG=1 docker.io/bborbe/claude-yolo:v0.10.1 echo hello
# exit code: 4
# stderr contains: iptables: Permission denied (you must be root)
# (the entrypoint's init-firewall.sh fails before reaching `echo hello`)

docker run --rm --cap-add NET_ADMIN --cap-add NET_RAW \
  -e DEBUG=1 docker.io/bborbe/claude-yolo:v0.10.1 echo hello
# exit code: 0
# stdout: hello
```

### Daemon-level reproduction (the user-visible failure)

```bash
cd /any/project/with/a/.dark-factory.yaml
dark-factory daemon
# expected: daemon enters "watching for queued prompts"
# actual: startup aborts with:
#   healthcheck: claude session probe failed: stdout=""
# (the probe container exited non-zero with empty stdout because
# init-firewall.sh failed before producing any output)
```

### Code-level reproduction (the architectural defect)

```bash
grep -rn "ContainerLaunchOpts{" pkg/ | grep -v _test.go
# returns TWO matches:
#   pkg/executor/executor.go  — the prompt-run path; sets CapAdd
#   pkg/cmd/healthcheck/probes.go  — the probe path; omits CapAdd

grep -rn "NET_ADMIN" pkg/ | grep -v _test.go
# returns ONE match (executor.go) — the probe path is silently missing it
```

## Expected vs Actual

**Expected** (per the probe code's own docstring at `pkg/cmd/healthcheck/probes.go` describing the boot/mount/claude probes as sharing `executor.BuildDockerRunArgs` so probes "hit the exact same launch path production uses"): the probe container boots with the same capability set, mount set, and base environment as a real prompt container. On OrbStack with no `--skip-healthcheck` flag, `dark-factory daemon` enters its watch loop.

**Actual**: the probe path builds its own `ContainerLaunchOpts` from a probe-specific intermediate struct (`ProbeLaunchConfig`) that has no capability field. The probe container is launched without `--cap-add NET_ADMIN --cap-add NET_RAW`. On OrbStack the `claude-yolo` entrypoint's `init-firewall.sh` fails with `iptables: Permission denied`, the container exits non-zero, the probe reports `stdout=""`, and daemon startup aborts.

## Why this is a bug

The probe's documented contract (its own docstring) is that it exercises the same launch path as production. It does not, and it cannot — the argv builder is shared but its input is not. This is the canonical shape of a contract that drifts silently: it was true the day the probe was written, but the next time the executor's launch shape changed (capability addition), the probe path was not updated because the probe path is invisible from the executor file. A user on a supported development environment (OrbStack) cannot start the daemon without disabling the very gate that exists to protect them.

## Workaround

`dark-factory daemon --skip-healthcheck` starts the daemon, but the user loses the gate's protection for the same prompt-run path that does work. Not a real fix — the daemon's startup contract is "if healthcheck passes, prompts will boot"; bypassing the gate means prompts may still boot, but the user has no a-priori signal.

## Acceptance Criteria

- [ ] **Single source of truth for the cap set** — evidence: `grep -rn "NET_ADMIN" pkg/ | grep -v _test.go | wc -l` returns exactly `1`. The one match is in the new shared-launch-policy file.
- [ ] **Single source of truth for `ContainerLaunchOpts` composition** — evidence: `grep -rn "ContainerLaunchOpts{" pkg/ | grep -v _test.go | wc -l` returns exactly `1` (down from `2` today). The one match is in the policy's per-invocation builder.
- [ ] **Executor no longer hardcodes the cap set inline** — evidence: `grep -n "NET_ADMIN" pkg/executor/executor.go` returns 0 lines.
- [ ] **Probes derive their launch opts from the shared policy** — evidence: `grep -n "ContainerLaunchOpts{" pkg/cmd/healthcheck/probes.go` returns 0 lines; the probes' `launchOpts` function reads from the same policy value the executor uses.
- [ ] **Probe argv contains the canonical caps** — evidence: a unit test named `TestProbeArgvContainsCaps` (or DescribeTable equivalent) in `pkg/cmd/healthcheck/probes_test.go` asserts the assembled argv slice for the claude / boot / mount probes contains both `--cap-add=NET_ADMIN` and `--cap-add=NET_RAW`. Test exit code 0; `go test ./pkg/cmd/healthcheck/...` passes.
- [ ] **Executor argv still contains the canonical caps** — evidence: `pkg/executor/executor_test.go` lines asserting `--cap-add=NET_ADMIN` / `--cap-add=NET_RAW` (currently around lines 451-452) continue to pass unchanged. `go test ./pkg/executor/...` passes.
- [ ] **Regression-lock test: a new field in the policy reaches both call sites** — evidence: a test adds a synthetic cap (e.g. `SYS_PTRACE`) to a test-only override of the policy, then asserts the cap appears in BOTH the executor's assembled argv AND each probe's assembled argv. The test fails if either site is updated independently of the policy. `go test` of the regression-lock file exits 0.
- [ ] **Bug reproduction no longer reproduces (the verification-mandatory check)** — evidence: on OrbStack, `dark-factory daemon` (no `--skip-healthcheck` flag) reaches its watch loop. The probe's container produces non-empty stdout. Captured: terminal log shows the daemon's "watching for queued prompts" (or equivalent ready-state) line and no `claude session probe failed` line.
- [ ] **Container-startup-site inventory is complete** — evidence: a comment block or doc snippet (in the policy file or `docs/`) enumerates every `exec.Command(..., "docker", ...)` and equivalent `docker run` call site in `pkg/`, naming each site as either (a) routed through the shared policy, or (b) explicitly out of scope with reason (e.g. `docker version`, `docker image inspect` — not `claude-yolo`). **Mechanical check**: the inventory's enumerated set of `file:line` pairs equals, line-for-line, the output of `grep -rnE 'exec\.Command(Context)?\(.*"docker"' pkg/ | grep -v _test.go`. Any line in the grep output not appearing in the inventory (or vice versa) fails the AC.
- [ ] **`make precommit` exits 0 in the changed modules** — evidence: exit code 0 for `make precommit` at the repo root.
- [ ] **Existing executor tests pass unchanged** — evidence: `go test ./pkg/executor/...` exits 0 without modification to assertions outside the cap-related lines covered above. `git diff pkg/executor/executor_test.go` shows changes only in the bodies of cap-related assertions if any (preferably zero diff).

## Verification

End-to-end (bug-mandatory replay of the original failure):

```bash
# In any project with a .dark-factory.yaml, on OrbStack:
cd /Users/bborbe/Documents/workspaces/recurring-task-creator-weekday-list
dark-factory daemon
# expect: daemon enters its watch loop ("watching for queued prompts" or equivalent);
#         no "claude session probe failed: stdout=\"\"" line in output.
```

Architectural invariants (must hold post-fix):

```bash
cd /Users/bborbe/Documents/workspaces/dark-factory-unify-container-launch

# Cap mentioned in exactly one production file:
grep -rn "NET_ADMIN" pkg/ | grep -v _test.go
# expect: exactly one line, in the new shared-launch-policy file.

# ContainerLaunchOpts composed in exactly one non-test site:
grep -rn "ContainerLaunchOpts{" pkg/ | grep -v _test.go
# expect: exactly one line, in the policy's per-invocation builder.

# Executor file no longer carries the inline cap literal:
grep -n "NET_ADMIN" pkg/executor/executor.go
# expect: zero lines.

# Probes file no longer composes ContainerLaunchOpts directly:
grep -n "ContainerLaunchOpts{" pkg/cmd/healthcheck/probes.go
# expect: zero lines.
```

Build / test:

```bash
make precommit
# expect: exit 0
go test ./pkg/executor/... ./pkg/cmd/healthcheck/...
# expect: exit 0
```

## Desired Behavior

1. A new shared value — a "base launch policy" — is constructed exactly once per daemon process from project config + environment. It carries every input that is intrinsic to "this is a dark-factory container": container image, project identity (name + root), Claude data directory, host home, base environment map, extra mounts, netrc / gitconfig paths, hide-git flag, and the canonical capability set (`NET_ADMIN`, `NET_RAW`).
2. The executor's prompt-run path derives its per-invocation `ContainerLaunchOpts` from this shared policy plus the per-invocation extras (container name, entrypoint, command, env overlay, label overlay, prompt-file mount). It no longer carries an inline capability literal.
3. The healthcheck probes (boot, mount, claude) derive their per-invocation `ContainerLaunchOpts` from the same shared policy plus their per-invocation extras (container name, entrypoint, command). The probe-specific intermediate struct that previously omitted capabilities is either removed or redefined to carry only the per-invocation subset.
4. Every probe that launches a `claude-yolo` container produces argv containing `--cap-add=NET_ADMIN` and `--cap-add=NET_RAW`, identically to the executor's argv.
5. A regression-lock test exercises the "one site to update" invariant by adding a synthetic capability through the shared policy and asserting it appears in BOTH the executor's and each probe's assembled argv. Adding a capability or other launch-shape field at the executor's call site, bypassing the policy, must cause the regression-lock test to fail.
6. The container-startup-site inventory enumerates every `docker run`-style call site in the codebase and classifies each as routed-through-policy or explicitly-out-of-scope. The `docker version` and `docker image inspect` probes remain explicitly out of scope (they do not invoke `claude-yolo`); the spec-generation path is audited and either folded in or documented as already routed through the executor.
7. On OrbStack, with no `--skip-healthcheck` flag, `dark-factory daemon` enters its watch loop. The claude probe's container exits zero with non-empty stdout.

## Constraints

- `BuildDockerRunArgs` (the argv assembly function in `pkg/executor/launch.go`) and its public signature must not change — it is the already-shared layer that this spec preserves.
- `ContainerLaunchOpts` (the struct passed to the argv builder) may gain fields or be split, but the executor's existing prompt-run behavior (including `insertPromptFileMount` at the executor boundary, `validateClaudeAuth`, and post-run commit logic) must be preserved.
- Existing executor tests in `pkg/executor/executor_test.go` must continue to pass. The cap-related assertions (currently around lines 451-452) must continue to assert presence of `--cap-add=NET_ADMIN` and `--cap-add=NET_RAW`.
- The canonical cap set is exactly `{NET_ADMIN, NET_RAW}` — no additions, no subtractions.
- Capabilities are not exposed in `.dark-factory.yaml` or any other user-facing config surface.
- The probes' docstring contract ("share the same launch path production uses") must become true by construction, not by convention.

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery |
|---|---|---|---|
| `claude-yolo` container exits non-zero during the claude probe even with caps present (e.g. OrbStack iptables policy still rejects) | Daemon startup log shows `claude session probe failed` with non-empty stderr | Daemon refuses to start; surfaces the container's stderr verbatim so the user can see the iptables error message | User reads stderr; if root cause is environment policy, user runs `dark-factory daemon --skip-healthcheck` (workaround) and files a follow-up bug |
| Image pull fails for `claude-yolo` before any probe argv is exercised | `docker image inspect` probe fails before the claude probe runs | Daemon refuses to start; surfaces the docker error | User runs `docker pull` manually; retries `dark-factory daemon` |
| Two daemon processes race on `dark-factory daemon` startup in the same project | Both processes attempt healthcheck independently; each constructs its own policy from the same config | Each daemon's policy is constructed independently from the same config — they produce identical argv; no shared in-memory state | No recovery action needed; if a downstream component cares about single-instance, it asserts a lock outside this spec's scope |
| Code change adds a launch-shape concern (new cap, new mount, new env var) at the executor call site, bypassing the policy | Regression-lock test fails in CI | `make precommit` / `go test` exits non-zero; PR cannot merge | Developer moves the field to the shared policy and retries |
| Config drift: project root path differs between when the daemon starts and when the executor runs (e.g. symlink change) | The policy captured at startup carries the root from startup time; the executor uses that captured value | Behavior matches today's executor — policy is captured once at startup, not re-resolved per invocation | If re-resolution is needed, that is a follow-up spec (out of scope here) |

## Security / Abuse

The container is launched with elevated Linux capabilities (`NET_ADMIN`, `NET_RAW`). This is unchanged from today's executor behavior — the spec does not widen the attack surface; it only ensures the probe matches it. No new user input crosses a trust boundary. The shared policy is constructed from already-validated config sources.

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Audit and inventory: enumerate every `exec.Command(..., "docker", ...)` and `docker run`-style call site in `pkg/`. Confirm the spec-generation path's launch shape. Output: a markdown inventory committed alongside the policy file. | 6 | 9 | — |
| 2 | Introduce the shared launch policy: new type carrying image / project / mounts / base env / netrc / gitconfig / hide-git / caps; per-invocation builder method; unit tests for argv composition (presence of caps and base mounts). No call-site changes yet. | 1 | 1, 5 (partial) | prompt 1 (inventory informs the policy's surface) |
| 3 | Migrate the executor's prompt-run path to derive `ContainerLaunchOpts` from the policy. Remove the inline `CapAdd` literal. Existing executor tests must still pass unchanged. | 2 | 2, 3, 6, 10 | prompt 2 |
| 4 | Migrate the healthcheck probes to derive `ContainerLaunchOpts` from the policy. Remove or redefine `ProbeLaunchConfig` so it carries only the per-invocation subset. Add unit tests for argv composition on each probe. | 3, 4 | 4, 5 | prompt 2 (independent of prompt 3 in principle, but easier serialised) |
| 5 | Regression-lock test: synthetic-cap injection through the policy, asserting both call sites pick it up. Lives in a single test file that imports both the executor and the probes' argv-assembly entry points. | 5 | 7 | prompts 3 and 4 |
| 6 | Bug-verification on OrbStack: replay the original reproduction (`dark-factory daemon` with no `--skip-healthcheck`); capture the watch-loop log line. Update `CHANGELOG` per project convention. | 7 | 8 | prompts 3, 4, 5 |

Rationale: prompt 1 is research-only — the inventory drives the policy's surface area. Prompt 2 lands the new type without disturbing callers. Prompts 3 and 4 migrate the two call sites independently; either could land first, but serialising them makes review smaller. Prompt 5 is the structural guarantee against future drift — it must come last so it actually exercises both migrated sites. Prompt 6 is the bug-mandatory runtime replay.

## Do-Nothing Option

The daemon continues to fail on OrbStack without `--skip-healthcheck`. Users who hit the failure learn the flag and bypass the gate; the gate's protection is lost for them. The architectural defect (two construction sites for the same struct) silently accumulates further drift on every future launch-shape change. Each new field added at the executor call site re-imposes the same bug class for future users on future environments. Net cost grows unboundedly while the visible symptom stays small until it doesn't.
