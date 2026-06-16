---
status: verifying
approved: "2026-06-16T20:03:31Z"
generating: "2026-06-16T20:03:31Z"
prompted: "2026-06-16T20:03:31Z"
verifying: "2026-06-16T21:05:52Z"
branch: dark-factory/healthcheck-on-daemon-startup
---

## Summary

- Today the daemon only runs `preflightCommand` (project build, in-container, per-prompt). The pipeline stack itself — Docker daemon, claude-yolo image, container boot, `gh`, notifications — is never re-validated after the operator's last manual `dark-factory healthcheck` run.
- This spec adds a second, **additive** startup gate: the daemon invokes the existing `healthcheck` probe sequence in-process on `daemon` startup, before entering the prompt-watch loop.
- The gate has its own config (`healthcheckEnabled`, `healthcheckInterval`), its own cache (success-only, 8h default), and its own skip-flag (`--skip-healthcheck`).
- `preflightCommand` / `preflightInterval` / `--skip-preflight` keep their current semantics — UNTOUCHED.
- Failure is terminal, matching the existing preflight policy. Successful checks are cached; failed checks are never cached.

## Problem

The pipeline stack (Docker daemon reachable, claude-yolo image present, container boot succeeds, claude session usable, mounts work, `gh` authed when `pr:true`, notification creds valid when configured) can rot between operator-driven `dark-factory healthcheck` runs. When it rots — image bumped, token rotated, mount path renamed — the daemon picks up the next prompt and burns 5-10 minutes in container before surfacing a failure the operator could have caught at start. The existing `preflightCommand` proves the project compiles inside the container; it cannot prove the container can be reached or that `gh` will work. The two gates cover complementary surfaces.

The parent CLI (spec 095, shipped in v0.179.0/v0.179.1) deliberately deferred wiring this into the daemon. That deferral is now the bottleneck: operators forget to re-run the CLI, the daemon happily marches into a broken stack, and the very failure mode the healthcheck exists to prevent surfaces at the most expensive point.

## Goal

When `dark-factory daemon` starts, it invokes the existing healthcheck probe sequence in-process before the queue-watch loop begins. Successful results are cached for `healthcheckInterval`; subsequent restarts within that window skip the gate. Failure is terminal — the daemon exits non-zero with a category-naming cause, matching the existing preflight-failure policy. The operator can disable the gate entirely via `healthcheckEnabled: false`, or bypass it for one invocation via `--skip-healthcheck`. The existing `preflightCommand` gate is unchanged.

## Non-goals

- Do NOT modify `preflightCommand`, `preflightInterval`, or `--skip-preflight` — they keep current semantics exactly. Existing preflight tests must continue to pass.
- Do NOT add a per-prompt healthcheck call — the gate runs once at daemon startup; per-prompt overhead is unacceptable.
- Do NOT add a periodic mid-daemon healthcheck loop — separate task if a future operator demands it.
- Do NOT add healthcheck of arbitrary user-defined criteria — the probe sequence is the same fixed set the CLI runs.
- Do NOT expose the healthcheck as a user-configurable command string (e.g. `healthcheckCommand: "<cmd>"`) — the healthcheck is a built-in probe sequence, not a user shell command. The config shape is `healthcheckEnabled` (bool) by design; a future consumer demanding command-string variation would need a separate spec.
- Do NOT cache failed healthcheck results — operator action (image rebuild, auth rotation) requires re-running the gate on next start.
- Do NOT change `run` (one-shot) mode behavior — this spec only affects `daemon`.

## Acceptance Criteria

- [ ] On `dark-factory daemon` start with default config, slog line `healthcheck startup gate starting` appears BEFORE the fsnotify watcher actually starts — evidence: in `dark-factory daemon` stderr, the line number of `healthcheck startup gate starting` is less than the line number of `watcher started` (the actual fsnotify-`Watch()`-fired line). The earlier `watching for queued prompts` banner — emitted by the daemon before preflight/healthcheck even run — is not the gate's barrier; `watcher started` is.
- [ ] On healthcheck pass, slog line `healthcheck startup gate ok` appears with an `elapsed=` field naming non-zero duration (proves real probes ran, not a no-op stub), and the daemon enters the watch loop — evidence: `grep -E 'healthcheck startup gate ok.*elapsed=[0-9]+(\.[0-9]+)?(ms|s)' run.log` returns ≥1 line.
- [ ] On healthcheck failure, daemon exits non-zero with a category-naming error message identifying which probe failed (e.g. `healthcheck failed: healthcheck probe "image" failed`) — evidence: `dark-factory daemon; echo $?` prints non-zero; stderr contains the substring `healthcheck failed:` followed by the probe-name. The actual message is wrapped twice (gate emits `healthcheck failed: ...`; runner wraps it as `healthcheck startup gate: healthcheck failed: ...`; main.go prefixes `error: `). All three layers carry value: the `error:` prefix marks CLI failure, `startup gate:` identifies which subsystem, `healthcheck failed:` identifies the gate verdict — substring match catches them all.
- [ ] Successful healthcheck is cached for `healthcheckInterval`; a second `daemon` start within the window does NOT re-run the probes — evidence: second invocation's stderr contains `healthcheck cache hit, skipping` and does NOT contain `healthcheck startup gate starting`. Negative evidence: `grep -c 'healthcheck startup gate starting' run2.log` returns 0.
- [ ] Failed healthcheck is NOT cached — evidence: after a failed run, `ls <cache-dir>/healthcheck-*` returns no file containing the failed-run timestamp; OR, a second invocation after fixing the underlying failure re-runs the probes (stderr contains `healthcheck startup gate starting`, not `cache hit`).
- [ ] `healthcheckEnabled: false` disables the gate entirely — evidence: stderr contains `healthcheck gate disabled` and does NOT contain `healthcheck startup gate starting`; daemon enters watch loop normally. Negative evidence: `grep -c 'healthcheck startup gate starting' run.log` returns 0.
- [ ] `--skip-healthcheck` CLI flag bypasses the gate for one invocation, no cache read or write — evidence: stderr contains `healthcheck skipped via --skip-healthcheck`; cache file mtime unchanged before/after the run (`stat -f %m <cache-path>` returns the same value, or "no such file" both times).
- [ ] `--skip-healthcheck` is position-agnostic — evidence: `dark-factory --skip-healthcheck daemon` and `dark-factory daemon --skip-healthcheck` both bypass the gate (stderr contains `healthcheck skipped via --skip-healthcheck` in both).
- [ ] Effective-config slog output includes `healthcheckEnabled=<bool> healthcheckEnabledSource=<default|global|project|arg>` and `healthcheckInterval=<duration> healthcheckIntervalSource=<default|global|project|arg>` — evidence: each of `grep -c 'healthcheckEnabled=' run.log`, `grep -c 'healthcheckEnabledSource=' run.log`, `grep -c 'healthcheckInterval=' run.log`, `grep -c 'healthcheckIntervalSource=' run.log` returns ≥1 (each key appears at least once in the effective-config emission; tolerates single-line OR multi-line slog format).
- [ ] `preflightCommand` behavior is UNCHANGED — evidence: `git diff origin/master..HEAD -- pkg/preflight/` returns zero lines (preflight package is `pkg/preflight/`, not `pkg/runner/preflight*.go`); `git diff origin/master..HEAD -- pkg/runner/runner.go | grep -iE '^[-+].*preflight'` returns only `+` lines that ADD the new healthcheck call beside the existing preflight call, never `-` lines touching preflight semantics; existing preflight unit tests pass unmodified (covered by `make precommit`); `--skip-preflight` still works (existing tests cover this); `preflightInterval` cache unchanged.
- [ ] `make precommit` exits 0 in the dark-factory repo — evidence: exit code.
- [ ] `CHANGELOG.md` `## Unreleased` block gains an entry referencing the healthcheck startup gate — evidence: `grep -n -A1 '## Unreleased' CHANGELOG.md` returns lines containing `healthcheck` and `startup` / `daemon`.
- [ ] `docs/configuration.md` documents `healthcheckEnabled` and `healthcheckInterval` with default values and source-precedence note — evidence: `grep -n 'healthcheckEnabled' docs/configuration.md` returns ≥1 line; `grep -n 'healthcheckInterval' docs/configuration.md` returns ≥1 line.
- [ ] `docs/running.md` cross-references the new startup gate in its healthcheck section — evidence: `grep -inE 'healthcheck startup gate|daemon startup' docs/running.md` returns ≥1 line (case-insensitive; section heading uses TitleCase, body uses lowercase, either form satisfies).
- [ ] `docs/architecture-flow.md` "Preflight Failure Policy" section extended (or renamed) to cover the healthcheck startup gate's identical terminal-exit semantics — evidence: `grep -n 'healthcheck' docs/architecture-flow.md` returns ≥1 line in the failure-policy section.
- [ ] **Post-Deploy (Rung-2):** `/dark-factory:verify-spec` PASSes against a freshly built `dark-factory` binary started as `daemon` on ≥2 real local repos (e.g. the dark-factory repo itself and one other Go repo with a `.dark-factory.yaml`) — evidence: verify-spec output shows the gate firing on first start, the cache-hit path firing on second start within the window, and the disable-via-config path firing when `healthcheckEnabled: false`. This AC is the meta-lesson lock from spec 095: verify-spec MUST run before tagging the release.
  - `deploy_check:` `dark-factory --version | awk '{print $NF}'`
  - `deploy_target:` `$(git describe --tags --abbrev=0 2>/dev/null || git rev-parse --short HEAD)`

## Verification

```bash
# 1. Build + unit tests
cd ~/Documents/workspaces/dark-factory-healthcheck-startup
make precommit

# 2. Live runtime evidence — start daemon against ≥2 real repos, observe the gate
cd ~/Documents/workspaces/<repo-A>
dark-factory daemon 2>&1 | tee /tmp/run-A.log &
DAEMON_PID=$!
sleep 5
kill $DAEMON_PID
grep -n 'healthcheck startup gate starting' /tmp/run-A.log
grep -n 'healthcheck startup gate ok' /tmp/run-A.log

# 3. Second start within interval → cache hit (no re-run)
dark-factory daemon 2>&1 | tee /tmp/run-A2.log &
DAEMON_PID=$!
sleep 5
kill $DAEMON_PID
grep -n 'healthcheck cache hit, skipping' /tmp/run-A2.log
# Negative evidence:
[ "$(grep -c 'healthcheck startup gate starting' /tmp/run-A2.log)" -eq 0 ]

# 4. Repeat (1)-(3) against a second real repo

# 5. --skip-healthcheck bypass
dark-factory --skip-healthcheck daemon 2>&1 | tee /tmp/run-skip.log &
sleep 5; kill $!
grep -n 'healthcheck skipped via --skip-healthcheck' /tmp/run-skip.log

# 6. healthcheckEnabled: false (set in .dark-factory.yaml)
dark-factory daemon 2>&1 | tee /tmp/run-disabled.log &
sleep 5; kill $!
grep -n 'healthcheck gate disabled' /tmp/run-disabled.log

# 7. Failure path — set containerImage to a bogus tag, expect non-zero exit
# (manual; see Failure Modes table)

# 8. MANDATORY before release tag — meta-lesson from spec 095 incident
/dark-factory:verify-spec specs/in-progress/<NNN>-healthcheck-on-daemon-startup.md
# Must PASS against live daemon on ≥2 real repos. This is the lock that
# spec 095 lacked — the v0.179.x healthcheck CLI shipped broken because
# verify-spec was skipped. Do NOT cut a release tag without this.
```

## Desired Behavior

1. On `dark-factory daemon` start, the daemon invokes the existing in-process healthcheck probe sequence (the same one `dark-factory healthcheck` CLI uses) before the prompt-watch loop begins.
2. If a successful healthcheck result is cached within `healthcheckInterval` (cache key: SHA of `<containerImage>:<projectName>:<healthcheckInterval-seconds>`; cache value: `(timestamp, success=true)`), the startup call is a fast skip (no probes execute, log line emitted).
3. If the healthcheck fails, the daemon exits non-zero with a category-naming error message and emits the same notification-on-terminal-failure behavior the existing preflight terminal-failure path uses. The failure is NOT cached.
4. If `healthcheckEnabled: false` in config (default true), the startup call is a no-op and the daemon proceeds directly to the watch loop.
5. If `--skip-healthcheck` CLI flag is set (default false; position-agnostic), the startup call is bypassed for that invocation. No cache read, no cache write. Log line records the skip.
6. The effective-config log line emitted at startup includes `healthcheckEnabled`, `healthcheckEnabledSource`, `healthcheckInterval`, and `healthcheckIntervalSource` so the operator can audit which gate fired and from which config layer.
7. `preflightCommand` + `preflightInterval` + `--skip-preflight` are not touched — their behavior is byte-for-byte unchanged. Both gates run independently when both are configured; either failing is terminal.

## Constraints

- Reuse the existing healthcheck probe sequence — no parallel probe implementation. The factory builds the probe slice the same way `CreateHealthcheckCommand` does; the daemon-startup path calls `Run(ctx, []string{})` directly instead of dispatching through CLI args.
- Failure semantics match the existing "Preflight Failure Policy" in `docs/architecture-flow.md` (lines 91-95): terminal exit, no retry, no skip-and-continue. The daemon is the sole writer of the working tree and cannot self-repair (image rebuild, auth rotation need operator action).
- `preflightCommand` / `preflightInterval` / `--skip-preflight` must remain byte-for-byte unchanged in behavior. Existing tests under `pkg/runner` covering preflight must pass unmodified.
- Cache lives on the host filesystem (parallel to the existing preflight cache), is keyed by SHA of `<containerImage>:<projectName>:<healthcheckInterval-seconds>`, and stores only success records.
- Apply only to `dark-factory daemon`. `dark-factory run` (one-shot) is out of scope.
- The `healthcheck` CLI subcommand (`dark-factory healthcheck`) continues to exist with current behavior — this spec adds a daemon-startup invocation path; it does not modify or remove the CLI.
- See `docs/architecture-flow.md` Preflight Failure Policy and `docs/configuration.md` Preflight Baseline Check for the existing patterns the new gate must mirror.

## Failure Modes

| Trigger | Detection | Expected behavior | Reversibility | Recovery |
|---------|-----------|-------------------|---------------|----------|
| Docker daemon unreachable at startup | Probe fails; slog `healthcheck failed: docker daemon unreachable` | Daemon exits non-zero; notification fires (same path as preflight terminal failure); cache NOT written | Reversible — operator starts Docker | Operator starts Docker daemon, restarts `dark-factory daemon`; next startup re-runs the probes (no stale-cache trap) |
| claude-yolo image missing or unpullable | Probe fails; slog `healthcheck failed: image <ref> not available` | Daemon exits non-zero; cache NOT written | Reversible — operator pulls / rebuilds image | Operator pulls or rebuilds image, restarts daemon |
| `gh` auth expired (and `pr:true`) | Probe fails; slog `healthcheck failed: gh not authenticated` | Daemon exits non-zero; cache NOT written | Reversible — operator runs `gh auth login` | Operator re-auths gh, restarts daemon |
| Notification credentials invalid (and notifications configured) | Probe fails; slog `healthcheck failed: notification <channel> unreachable` | Daemon exits non-zero; cache NOT written | Reversible — operator rotates credentials | Operator updates credentials, restarts daemon |
| Cache file corrupted (unreadable / malformed) | Cache read fails | Treat as cache miss; re-run the gate; log a single warning line `healthcheck cache unreadable, re-running` | Reversible | Next successful run writes a fresh cache file |
| Two `dark-factory daemon` instances racing on the same cache file | One writes after the other reads | Last-write-wins on cache; both daemons run probes if both cache reads were misses. Instance-locking (spec 005) prevents two daemons running on the same repo at all; this row exists for cross-repo daemons sharing a cache root, which is acceptable (each repo has its own cache key) | Reversible | N/A — cache races are harmless because cache only stores success and probes are idempotent |
| Operator passes `--skip-healthcheck` while underlying probe would have failed | No probe runs; slog `healthcheck skipped via --skip-healthcheck` | Daemon proceeds; subsequent prompt execution may fail mid-run when the stack issue surfaces | Reversible | Operator must judge when to use this flag; intended as an explicit override |
| Healthcheck probe hangs (network timeout, slow daemon) | Per-probe timeout fires (existing probe timeouts apply) | Probe returns timeout error; daemon exits non-zero with `healthcheck failed: <probe> timed out` | Reversible | Operator investigates the slow dependency, restarts daemon |
| Clock skew makes cached timestamp appear in the future | Cache validity check sees future timestamp | Treat as cache miss; re-run the gate; log a single warning line | Reversible | Next successful run writes a fresh cache file |
| `healthcheckInterval` config value unparseable | Config validation at startup | Daemon exits non-zero at config-load with a parse-error message naming the field | Reversible | Operator fixes config, restarts daemon |

## Security / Abuse Cases

- `healthcheckInterval` is operator-controlled config (no attacker input crosses a trust boundary); a malicious value is at worst a denial-of-startup which the operator chose.
- The cache file lives in the user's home / dark-factory state dir under standard permissions; no privilege escalation surface.
- `--skip-healthcheck` is a CLI flag — only the operator running the daemon can set it. No remote actor can flip the gate off.
- Healthcheck probes are read-only against external systems (Docker daemon ping, image inventory check, `gh auth status`, notification channel reachability). They do not mutate state.

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Config schema + getters + defaults + effective-config log fields (`HealthcheckEnabled *bool`, `HealthcheckInterval string` in `pkg/config/config.go`, plus the two new log keys in `pkg/factory/factory.go`'s effective-config slog line) | 6 | effective-config log AC, `preflightCommand` unchanged AC, `make precommit` AC | — |
| 2 | Healthcheck startup-gate function + cache (build the gate function reusing the existing healthcheck probe builder; success-only cache keyed by SHA of `<containerImage>:<projectName>:<healthcheckInterval-seconds>`; failure path emits terminal-error log) | 1, 2, 3 | startup-log AC, gate-ok AC, gate-failure AC, cache-hit AC, never-cache-failure AC | prompt 1 |
| 3 | Wire gate into daemon startup + `--skip-healthcheck` CLI flag + `healthcheckEnabled: false` short-circuit (mirror `--skip-preflight`; position-agnostic; call the gate from the daemon startup path after preflight and before the watch loop) | 4, 5, 7 | disabled AC, skip-flag AC, position-agnostic AC, preflight-unchanged AC | prompts 1, 2 |
| 4 | Docs + CHANGELOG (`docs/configuration.md`, `docs/running.md`, `docs/architecture-flow.md`, `CHANGELOG.md` Unreleased) | — | CHANGELOG AC, configuration.md AC, running.md AC, architecture-flow.md AC | prompts 1-3 (so wording reflects the shipped behavior) |

Rationale: prompt 1 lands the config surface and the effective-config log line so subsequent prompts have observables; prompt 2 builds the gate logic in isolation (unit-testable without daemon wiring); prompt 3 wires it into the daemon-startup path and adds the skip-flag; prompt 4 documents the shipped behavior. The verify-spec live-runtime AC (Post-Deploy Rung-2) is exercised after all four prompts merge — see Verification section step 8.

## Do-Nothing Option

Operators continue to manually run `dark-factory healthcheck` before queuing prompts and hope they remember to re-run it after each image bump, token rotation, or notification-config change. When they forget — the dominant case — the daemon picks up the next prompt, burns 5-10 minutes in container, and surfaces a failure the CLI exists to prevent. Cost: ongoing operator-trust erosion in the daemon, sustained drag of repeated failed-prompt cycles, and the embarrassing parent-task incident (v0.179.x shipping a CLI that the daemon never invokes) sitting unresolved. The CLI exists; not wiring it into the daemon is leaving the feature half-built.
