# ADR-0001: YOLO Container Security Model

**Status:** Accepted (Phase 1 — structural plumbing only)
**Date:** 2026-06-25
**Surfaced by:** 2026-06-25 architecture review (dimensions pass, finding #6 / config and blast radius)

## Context

The YOLO container is the execution sandbox for every dark-factory prompt. It runs `claude-yolo` with the operator's Claude OAuth credentials mounted, plus the project working tree.

### Pre-ADR baseline (audit findings)

`pkg/executor/launch.go:34` `BuildDockerRunArgs` had four security gaps:

1. **Runs as root.** No `--user` flag; container runs as the default user of the `claude-yolo` image (root).
2. **Credentials mounted read-write.** `claudeDir:/home/node/.claude` lets the agent rewrite `~/.credentials.json` from inside the container.
3. **No resource limits.** No `--memory`, `--cpus`, `--pids-limit`. A runaway agent can OOM the host or pin all CPUs.
4. **No isolation guarantee.** A compromised prompt could exfiltrate the OAuth token, run unauthenticated API calls under the operator's account, or wedge the host.

### Constraints

- Existing prompts must continue to execute (no regression).
- Claude SDK may need write access to refresh OAuth tokens — this requires dev validation before enforcing `:ro`.
- Resource limits must be operator-configurable (different machines, different budgets).

## Decision

Implement security hardening in two phases.

### Phase 1 (this PR) — structural plumbing

Add the following fields to `launchpolicy.ContainerLaunchOpts`:

| Field | Type | Emits | Default |
|---|---|---|---|
| `RunAsUser` | `string` | `--user <value>` | `""` (no change — root) |
| `MemoryLimit` | `string` | `--memory <value>` | `""` (no limit) |
| `CPULimit` | `string` | `--cpus <value>` | `""` (no limit) |
| `PIDsLimit` | `int` | `--pids-limit <N>` | `0` (no limit) |
| `ClaudeDirReadOnly` | `bool` | `:ro` suffix on claudeDir mount | `false` (rw) |

`BuildDockerRunArgs` emits each flag only when the corresponding field is non-zero / non-empty. **Defaults preserve current production behavior** — Phase 1 ships zero functional change. Operators who set these fields in config get the protection immediately.

`launchpolicy.Policy` exposes the fields via `NewPolicy` (additional optional args) and threads them through `BuildOpts`.

### Phase 2 (follow-up task)

Wire factory defaults to enforce the security model in production:

- `RunAsUser = "1000:1000"` (non-root, matches `claude-yolo` image's `node` user).
- `ClaudeDirReadOnly = true` — but **first validate in dev** that Claude SDK token refresh still works (filed as follow-up).
- `MemoryLimit = "8g"`, `CPULimit = "4"`, `PIDsLimit = 1024` — sensible defaults, operator-overridable via config.

Phase 2 ships in a separate PR after dev validation of each enforcement.

## Why two phases

- **Dev validation is required** for the `:ro` claudeDir mount. If Claude SDK needs to write the credential file (token refresh), forcing `:ro` breaks every prompt. Validating this requires running a live prompt in dev — not something a single PR can verify in CI.
- **No-regression** invariant is easier to defend when Phase 1 ships zero behavior change. Operators opt in.
- **Reviewability** — Phase 1 is mechanical; Phase 2 is policy.

## Out of scope

- Replacing Docker with gVisor / Firecracker — future hardening, separate proposal.
- Capability set tightening — `CanonicalCaps` is governed by spec 098; not changed here.
- Network isolation — separate ADR if needed.
- Per-prompt secrets injection — orthogonal.

## Compliance / verification

- **Architectural invariant:** `BuildDockerRunArgs` argv contains `--user` / `--memory` / `--cpus` / `--pids-limit` iff the matching opt field is non-zero. Unit tests verify both directions.
- **Backward compatibility:** existing tests must pass without modification (Phase 1 defaults match prior behavior).
- **Future drift detection:** Phase 2 will add an integration test that production policy emits all four flags.

## References

- `pkg/executor/launch.go:34` `BuildDockerRunArgs`
- `pkg/launchpolicy/policy.go` `Policy`, `ContainerLaunchOpts`
- Architecture review: 2026-06-25
- Follow-up task: [[Phase 2 — Enable Dark-Factory YOLO Container Security Defaults]] (to be filed after Phase 1 lands)
