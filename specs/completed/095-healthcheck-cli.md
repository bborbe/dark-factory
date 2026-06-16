---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-06-16T12:51:33Z"
generating: "2026-06-16T12:54:16Z"
prompted: "2026-06-16T13:09:26Z"
verifying: "2026-06-16T14:30:02Z"
completed: "2026-06-16T19:20:52Z"
branch: dark-factory/healthcheck-cli
---

## Summary

- Add a new `dark-factory healthcheck` CLI subcommand that probes the entire pipeline-execution stack (Docker daemon, container image, container boot, Claude session, workspace mount, optional GitHub auth, optional notifications channel) on demand, before the operator queues real specs or prompts.
- One-shot, fail-fast, ~10s wall-clock with the Claude probe (~2s without); exit code 0 = green; non-zero = categorized failure table written to stdout.
- Healthcheck is all-or-nothing — there is NO flag to skip any probe. Skipping the highest-failure-rate component (the Claude session probe) would defeat the entire purpose of a pre-flight stack check.
- Closes the diagnostic gap left by the runtime container-liveness check (spec 043) and the project-build preflight: those detect vanished containers and broken project builds respectively, neither catches a broken Claude session, a stale `gh` token, or an expired notifications token before the first prompt burns 5-10 minutes.
- Container-boot logic is extracted into a shared helper so the new command and scenario 003 cannot drift.

## Problem

When the operator launches an overnight 40-repo run, a single pipeline-stack regression — a UID-remap change in the claude-yolo image, an expired `ANTHROPIC_API_KEY`, a stale `gh` token, an old Telegram bot token — silently bleeds hours. The first symptom today is a prompt that mysteriously stalls or fails 5-10 minutes after it starts executing. There is no on-demand "is the whole pipeline actually green right now" probe: `dark-factory doctor` only detects host-side spec/prompt state anomalies, the spec 043 container-liveness check only fires after a container vanishes mid-run, and the project's `preflightCommand` only checks the project build, not the dark-factory plumbing. Scenario 003 covers most of this but requires the sandbox repo, manual setup, and a fresh binary in `/tmp/` — it is not callable by the operator at any moment.

## Goal

After this work, the operator can run `dark-factory healthcheck` from any project that has a `.dark-factory.yaml` and, in roughly ten seconds, receive a deterministic verdict on whether the full pipeline-execution stack will work for the next prompt: Docker daemon reachable, image present, container boots without privilege regression, Claude session inside the container actually responds, workspace mount is writable, and — when the project's config enables them — `gh auth status` passes and the configured notifications channel accepts a POST. A non-zero exit returns a categorized table that names which probe failed and at which step, in the same shape as the existing `dark-factory doctor` output.

## Non-goals

- Periodic or scheduled healthcheck — one-shot CLI only; the daemon does not call `healthcheck` automatically. Do NOT add a `--watch` or scheduling flag — invariant; if a future consumer demands periodic probing, that's a separate spec.
- Replacing `preflightCommand` — `preflightCommand` is a project-build gate that runs before each prompt; `healthcheck` is a pipeline-stack gate the operator runs on demand.
- Replacing or duplicating `dark-factory doctor` — `doctor` inspects host-side spec/prompt state anomalies; `healthcheck` exercises the runtime execution stack.
- Replacing scenario 003 — the scenario remains the pre-release smoke test that publishes a real prompt and inspects log output. The new command shares the container-boot helper with the scenario; it does not subsume it.
- Healthcheck of arbitrary user-defined criteria — the seven probes listed below are the entire surface. Do NOT add a `customProbes` config field or a plugin hook — invariant; if a future consumer demands a custom probe, that's a separate spec.
- A `--probe=<name>` selector or any per-probe skip flag — invariant. Skipping any probe means the operator can't trust the verdict. If you need a faster smoke, fix the slow probe, don't skip it.
- Auto-fix mode — `healthcheck` reports only; it does not mutate Docker state, pull images, or rewrite config.

## Acceptance Criteria

- [ ] The binary built from this branch exposes a `healthcheck` subcommand — evidence: `dark-factory healthcheck --help` exits 0 and stdout contains the literal substring `Usage: dark-factory healthcheck`.
- [ ] All seven probes execute in the order Docker → image → boot → Claude → mount → gh → notifications when run on a green stack — evidence: running `dark-factory healthcheck` against a known-green sandbox writes one log line per probe to stderr at INFO level in that exact order (`grep -nE 'probe=(docker|image|boot|claude|mount|gh|notifications)' stderr.log` lists the seven probes in the listed order).
- [ ] Fail-fast ordering is observed — evidence: with the Docker daemon stopped (or `DOCKER_HOST=tcp://127.0.0.1:1` to simulate unreachable), `dark-factory healthcheck` exits non-zero AND `grep -c 'probe=image' stderr.log` returns 0 (image probe never ran because docker probe failed first).
- [ ] On all-green, the command exits 0 — evidence: `dark-factory healthcheck; echo $?` prints `0` against a sandbox with `pr: false` and no `notifications` config.
- [ ] On any probe failure, the command exits non-zero with a categorized table on stdout — evidence: with `containerImage` in `.dark-factory.yaml` pointed at a non-existent tag (e.g. `claude-yolo:does-not-exist`), `dark-factory healthcheck` exits non-zero AND stdout contains a row with category `image` on its own line followed by an indented detail line naming the missing tag. Format mirrors `dark-factory doctor`'s two-line table shape. Evidence: `grep -E '^image$' stdout.log` returns ≥1 line AND `grep -E 'claude-yolo:does-not-exist' stdout.log` returns ≥1 line.
- [ ] Healthcheck is all-or-nothing — there is NO flag to skip the Claude probe. Skipping the highest-failure-rate component would defeat the purpose. Evidence: `dark-factory healthcheck --no-claude` exits non-zero with `error: unknown flag: "--no-claude"` AND `dark-factory healthcheck --help` stdout does NOT contain `--no-claude`.
- [ ] When the claude session is genuinely broken, the Claude probe fails and stdout shows a `claude` category row — evidence: this is verified by unit tests in `pkg/cmd/healthcheck/probes_test.go` (`claudeProbe` failure path: subproc-runner returns non-zero exit → probe returns wrapped error containing "claude session probe failed"). NOTE: in-band runtime fail-injection via env vars is NOT possible because the YOLO container mounts `~/.claude-yolo` which contains OAuth credentials that take precedence over `ANTHROPIC_API_KEY`/`ANTHROPIC_AUTH_TOKEN` — this matches production behavior and is intentional. The unit-test path is sufficient evidence; live failure surfaces naturally when claude-yolo / upstream API actually break.
- [ ] The `gh` probe runs only when `pr: true` — evidence: (a) with `pr: false` in `.dark-factory.yaml`, `grep -c 'probe=gh' stderr.log` returns 0. (b) The `pr: true` case is verified by unit tests in `pkg/cmd/healthcheck_test.go` (orchestration block: factory-pre-trimmed slice excludes `gh` when `cfg.PR == false`, includes it when `cfg.PR == true`). Live-runtime evidence for the `pr: true` half requires a project configured with `workflow: branch|clone|worktree` because config validation rejects `workflow: direct + pr: true` — that's an inherent constraint of the config schema, not a healthcheck defect. The unit-test path is sufficient evidence for the gating contract.
- [ ] The notifications probe runs only when notifications are configured — evidence: with no `notifications` key in `.dark-factory.yaml`, `grep -c 'probe=notifications' stderr.log` returns 0; with a configured channel, the same grep returns ≥1.
- [ ] Wall-clock budget on green (developer laptop, image already pulled, Docker daemon warm): full run ≤ 45 seconds — evidence: `time dark-factory healthcheck` reports `real` < 45s. (The Claude probe alone can take 5-30s on cold-start depending on auth + upstream API latency; the budget accommodates worst-case cold-start while still catching genuinely stuck sessions.) On CI / loaded hosts the budget relaxes to ≤ 90s.
- [ ] Container-launch logic is shared between the healthcheck probes and the production prompt-executor — `executor.BuildDockerRunArgs(opts ContainerLaunchOpts)` in `pkg/executor/launch.go` is called by `dockerExecutor.buildDockerCommand` (production prompts) AND by `bootProbe`, `mountProbe`, `claudeProbe`. Evidence: `grep -n 'executor.BuildDockerRunArgs' pkg/cmd/healthcheck/probes.go` returns ≥1 match AND `grep -n 'BuildDockerRunArgs' pkg/executor/executor.go` returns ≥1 match (via the `buildDockerCommand` method). Scenario 003 stays markdown-only and cross-references `dark-factory healthcheck` as the automated path: `grep -n 'dark-factory healthcheck' scenarios/003-smoke-test-container.md` returns ≥1 line.
- [ ] No bare `return err` is introduced by the new code — evidence: `git diff origin/master..HEAD -- '*.go' | grep -nE '^\+\s*return err$'` returns 0 lines.
- [ ] All errors use `errors.Wrapf` from `github.com/bborbe/errors` — evidence: `git diff origin/master..HEAD -- '*.go' | grep -nE '^\+.*fmt\.Errorf'` returns 0 lines in files under `pkg/cmd/healthcheck` or `pkg/runner/probe`.
- [ ] `make precommit` is green on the branch — evidence: exit code 0.
- [ ] `CHANGELOG.md` has at least one healthcheck-related entry in either the `## Unreleased` block OR the most-recent released `## vX.Y.Z` block. Evidence: `grep -nE '^- .*healthcheck' CHANGELOG.md` returns ≥1 line. The looser location requirement accommodates dark-factory's auto-release behaviour: every commit that touches `## Unreleased` triggers a tag-and-rename to `## vX.Y.Z`, so between spec approval and spec verification the entries may have shifted from Unreleased to a released block. Either form is acceptable.
- [ ] `docs/running.md` documents the command — evidence: `grep -nE '^#+ .*[Hh]ealthcheck' docs/running.md` returns ≥1 line AND `grep -n 'dark-factory healthcheck' docs/running.md` returns ≥1 line.
- [ ] `docs/troubleshooting.md` lists `dark-factory healthcheck` as the first thing to run for a mysteriously-failing prompt — evidence: `grep -n 'dark-factory healthcheck' docs/troubleshooting.md` returns ≥1 line AND that line number is less than the line number of the next pre-existing `^## ` heading after the introductory section.

## Verification

```
make precommit
```

Manual verification:

```
# Green path
dark-factory healthcheck                        # → exit 0, no failures table, ≤ 45s

# Unknown flag — healthcheck is all-or-nothing
dark-factory healthcheck --no-claude            # → exit non-zero, "unknown flag" error

# Broken image
sed -i.bak 's/^containerImage:.*/containerImage: claude-yolo:does-not-exist/' .dark-factory.yaml
dark-factory healthcheck                        # → non-zero, table shows image failure
mv .dark-factory.yaml.bak .dark-factory.yaml

# Broken Claude session
ANTHROPIC_API_KEY=sk-invalid dark-factory healthcheck   # → non-zero, table shows claude failure
```

## Desired Behavior

1. **A new `healthcheck` subcommand is registered.** Invoking `dark-factory healthcheck` runs the probe sequence and exits with code 0 on full pass, non-zero on any failure. `dark-factory healthcheck --help` prints usage to stdout and exits 0.

2. **The probe sequence runs in fixed order, fail-fast.** The order is:
   1. Docker daemon reachable (a `docker version`-equivalent query succeeds).
   2. The configured `containerImage` is present locally (no implicit pull — absence is a failure, not a remediation).
   3. A container started from `containerImage` boots cleanly: no `root/sudo privileges` error, no UID-remap regression, `/workspace` is writable from inside.
   4. `claude -p "reply with exactly: OK"` invoked inside the container exits 0 and its stdout contains the literal string `OK`. Probe runs under a hard timeout of 30s (cold-start `claude` + Anthropic/MiniMax round-trip can take 15-25s on a fresh container).
   5. The host's `/workspace` mount target is writable from inside the container (a touch-and-remove probe in `/workspace`).
   6. (Only when `pr: true` in the resolved config) `gh auth status` exits 0.
   7. (Only when a notifications channel is configured) an HTTP POST to the configured channel returns a 2xx response. The POST body is a minimal "healthcheck" payload (exact wording agent decides at impl time); the channel receives a single message per healthcheck run.

   On the first probe that fails, remaining probes are skipped and the command exits non-zero. The pre-failure probes are still reported as passing in the output table.

3. **Output is a categorized table on stdout, structured logs on stderr.** Each probe emits a structured log line on stderr at INFO level on success and ERROR level on failure (`log/slog`), including the probe name as a field (e.g. `probe=docker`). On exit, stdout carries a human-readable table whose first column is the probe category and whose row shape mirrors `dark-factory doctor`'s output. On all-pass, stdout shows `all probes passed` (or equivalent — agent decides exact wording at impl time).

4. **No probe-skip flag exists.** The only flag the command accepts is `--help`/`-h`. Any other arg is rejected with `unknown flag: "..."`. The claude probe in particular is non-optional; healthcheck is all-or-nothing.

5. **Probes 6 and 7 are config-gated, not flag-gated.** The `gh` probe is skipped iff `pr: false` in the resolved `.dark-factory.yaml`. The notifications probe is skipped iff no notifications channel is configured. There are no `--no-gh` / `--no-notifications` flags.

6. **Container-boot logic is a shared package symbol.** The container-boot probe (step 3) and scenario 003's boot check call the same symbol under `pkg/runner/`. The exact name (`Probe`, `BootContainer`, etc.) and signature are decided at impl time, but a single symbol is the contract; scenario 003 is refactored to call it. No copy-paste boot logic.

7. **The subcommand is wired through the existing Cobra command tree.** `SilenceUsage: true`; no `os.Exit` inside the `RunE`; errors are returned and wrapped with `errors.Wrapf` from `github.com/bborbe/errors`. The constructor pattern mirrors `pkg/cmd/approve.go` and `pkg/cmd/doctor.go`: an interface with a `Run(ctx, args) error` method, a `//counterfeiter:generate` directive, and a `NewHealthcheckCommand(...)` constructor.

## Constraints

- The shape of `dark-factory doctor`'s exit semantics (0 = clean, non-zero = findings) and table layout must be preserved by `healthcheck` — operators read both outputs and the mental model must stay one model.
- `pkg/runner/health_check.go` (spec 043, periodic container-liveness check) is not modified by this spec. It is a different surface; both can co-exist.
- The container-boot helper extracted under `pkg/runner/` must remain callable from both the `healthcheck` command and the scenario-003 harness; the helper must not depend on Cobra, on the `pkg/cmd/` package, or on the scenarios package — only on `pkg/runner/` and below.
- The `.dark-factory.yaml` schema is not extended by this spec (no new fields, no new defaults).
- All new Go code conforms to the project's coding rules: `errors.Wrapf` from `github.com/bborbe/errors` for every error path; no bare `return err`; no `fmt.Errorf`; `log/slog` to stderr.
- The probe execution must respect context cancellation: `Ctrl-C` aborts within ~1s and the command exits with a non-zero code distinct from a probe failure (agent decides exact code at impl time).
- See `docs/troubleshooting.md` for the operator-facing diagnostic flow this command slots into; the doc update lives under Acceptance Criteria.
- See `docs/running.md` for the operator-facing "how to run dark-factory" surface this command is added to.

## Failure Modes

| Trigger | Detection | Expected behavior | Reversibility | Recovery |
|---------|-----------|-------------------|---------------|----------|
| Docker daemon not running | docker version query returns connection error | Probe 1 fails; probes 2-7 skipped; stdout shows `docker` row with the connection error message; exit non-zero | Reversible | Operator starts Docker Desktop / `dockerd`; re-runs `dark-factory healthcheck` |
| Configured `containerImage` not present locally | `docker image inspect <image>` returns not-found | Probe 2 fails; probes 3-7 skipped; stdout shows `image` row naming the missing tag; exit non-zero | Reversible | Operator runs `docker pull <image>`; re-runs healthcheck |
| Container boots but immediately exits with `root/sudo privileges` (UID-remap regression) | Container exit code non-zero AND log contains `root/sudo` substring | Probe 3 fails; stdout shows `boot` row with the captured log line; exit non-zero | Reversible | Operator rolls back the offending image tag; re-runs healthcheck |
| `claude -p` returns non-zero or stdout missing literal `OK` | Probe exit code or stdout-match check | Probe 4 fails; stdout shows `claude` row with truncated stdout/stderr (cap at e.g. 200 chars); exit non-zero | Reversible | Operator rotates `ANTHROPIC_API_KEY` or fixes Claude install; re-runs healthcheck |
| `claude -p` hangs | Per-probe hard timeout fires (target ≤ 10s) | Probe 4 fails with timeout category; container is killed before the command returns; exit non-zero | Reversible (container is one-shot) | Operator investigates Claude availability; re-runs healthcheck |
| `/workspace` not writable from inside container (mount perms / readOnly bug) | Touch-and-remove probe inside container returns EACCES or EROFS | Probe 5 fails; stdout shows `mount` row with the captured errno text; exit non-zero | Reversible | Operator fixes mount config (`extraMounts`, host perms); re-runs healthcheck |
| `gh auth status` returns non-zero (token expired / scope changed) | Exit code | Probe 6 fails; stdout shows `gh` row with `gh`'s stderr captured; exit non-zero | Reversible | Operator runs `gh auth login`; re-runs healthcheck |
| Notifications POST returns 4xx (bot token revoked / webhook disabled) | HTTP status code | Probe 7 fails; stdout shows `notifications` row with the HTTP status and a snippet of response body (cap at e.g. 200 chars); exit non-zero | Reversible | Operator regenerates the channel credentials in `.dark-factory.yaml`; re-runs healthcheck |
| Notifications POST hangs | Per-probe HTTP-client timeout (target ≤ 5s) | Probe 7 fails with timeout category | Reversible | Operator inspects channel health; re-runs healthcheck |
| Two `dark-factory healthcheck` invocations run concurrently against the same project | Throwaway container names are random per invocation (e.g. `dark-factory-healthcheck-<random>`); no shared lockfile; observed by running two `healthcheck` invocations back-to-back and inspecting `docker ps` to confirm distinct container names | Both runs complete independently; no shared state between them | Reversible | N/A — no recovery needed |
| `Ctrl-C` mid-probe | Context cancellation propagates | The active probe aborts within ~1s; any spawned container is killed; exit code non-zero AND distinct from probe-failure code | Reversible (container is one-shot) | Operator re-runs healthcheck |
| Clock skew between host and container | Probes do not depend on absolute timestamps; per-probe timeouts use the host monotonic clock | No effect on probe verdicts | N/A | N/A |
| Disk full on host (container cannot write throwaway files) | Probe 5 returns ENOSPC | Probe 5 fails with mount category; exit non-zero | Reversible | Operator frees disk; re-runs healthcheck |

## Security / Abuse Cases

- The Claude probe sends a fixed, minimal prompt (`reply with exactly: OK`) — no user input is interpolated, so prompt-injection surface is nil.
- The notifications probe POSTs a fixed body to the configured channel — no user input crosses the boundary beyond the channel URL/token already in the resolved config. The probe must not log the channel token at INFO or ERROR level; only the URL host and the HTTP status code (token redaction handled by existing notifications logic — agent confirms at impl time).
- The container-boot probe uses a unique container name per invocation (e.g. `dark-factory-healthcheck-<random>`); it never reuses or matches an active prompt's container name, so it cannot collide with a running prompt.
- The command is read-only with respect to host state: no spec/prompt files are mutated, no `.dark-factory.yaml` is rewritten, no images are pulled.
- The container spawned by probe 3 has the same mount/UID configuration as a real prompt container; it inherits the same security posture and adds no new attack surface.
- Per-probe hard timeouts (Claude ≤ ~10s, notifications HTTP ≤ ~5s) prevent unbounded hangs from a malicious or broken external endpoint.

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Extract shared container-boot helper under `pkg/runner/` (likely `pkg/runner/probe.go`); refactor scenario 003 to call it; no behavior change for scenario 003 | 6 | container-boot-shared AC | — |
| 2a | Add `pkg/cmd/healthcheck.go` skeleton (interface + constructor + Cobra wiring + `--no-claude` flag) and the four LOCAL probe implementations (Docker, image, boot via prompt 1's helper, mount) with unit tests per probe in isolation | 1, 2, 3 (probes 1-3 + 5), 7 | command-exists, help-no-claude, probe-order (probes 1-3,5 portion), fail-fast (Docker stopped case), exit-zero-on-green (with `--no-claude`), exit-nonzero-on-fail (image case), no-bare-err, errors-wrapf, precommit-green | prompt 1 |
| 2b | Add the three EXTERNAL probe implementations (Claude `claude -p` inside container, `gh auth status`, notifications HTTP POST) with unit tests per probe in isolation | 3 (probes 4, 6, 7), 4, 5 | no-claude-skips, claude-fails-on-bad-token, gh-config-gated, notif-config-gated, wall-clock | prompt 2a |
| 3 | Documentation: `docs/running.md` healthcheck section; `docs/troubleshooting.md` "first thing to run" entry; `CHANGELOG.md` `## Unreleased` entry | — | changelog-entry, running-md-section, troubleshooting-first-thing | prompt 2b |

Rationale: prompt 1 establishes the shared helper so subsequent prompts cannot fork the boot logic. Prompt 2 is split into 2a (local probes — no external calls, fast unit tests) and 2b (external probes — Claude/gh/notifications, slower mocked tests) to keep each prompt within the prompt-creator's context budget (~7 ACs each) and to land the no-token-spending probes before the token-spending one. Prompt 3 is a docs-only patch with a tight blast radius. No new scenario is added — scenario 003 already covers container-boot behavior end-to-end, and the new probes are unit-testable in isolation with mocks; the four-condition scenario rule from `docs/rules/scenario-writing.md` does not warrant a new E2E.

## Do-Nothing Option

Operators continue to discover broken pipeline state 5-10 minutes into the first real prompt of an overnight 40-repo run. The cost is bounded but recurring: every claude-yolo image bump, every API-key rotation, every Telegram bot-token expiry creates a silent regression window. Scenario 003 covers most of this surface but requires the sandbox repo and manual setup, so it is not a viable on-demand probe. Acceptable in the short term; increasingly expensive as prompt volume and repo count grow.

## Verification Result

**Verified:** 2026-06-16T19:14:38Z (HEAD 127f588)
**Binary:** /tmp/dark-factory-127f588 (built from HEAD)
**Scenario:** Live walk of all 17 ACs against dark-factory's own `.dark-factory.yaml` (`pr: false`, no notifications); broken-image and DOCKER_HOST=tcp://127.0.0.1:1 stages forced for AC 3 and AC 5.
**Evidence:**
- AC 1: `healthcheck --help` exit 0, stdout contains `Usage: dark-factory healthcheck`
- AC 2/4: green run exit 0 in 3.913s, stderr probes ordered docker→image→boot→claude→mount (gh/notifications config-gated off)
- AC 3: `DOCKER_HOST=tcp://127.0.0.1:1` → exit 1, `grep -c 'probe=image'` = 0 (fail-fast confirmed)
- AC 5: `containerImage: claude-yolo:does-not-exist` → exit 1, stdout `image\n  container image "claude-yolo:does-not-exist" not present locally`
- AC 6: `--no-claude` rejected with `unknown flag: "--no-claude"`; `--help` contains 0 `--no-claude` references
- AC 7/8: ginkgo suite `go test ./pkg/cmd/healthcheck/...` passes; claudeProbe failure path asserts `claude session probe failed` (probes_test.go:207, :218); orchestration block asserts `probe=gh` absent when `pr: false` (healthcheck_test.go:203-214)
- AC 9: green run stderr has 0 `probe=notifications` lines (no notifications configured)
- AC 10: `time` reports `real` = 3.913s (≪ 45s)
- AC 11: `executor.BuildDockerRunArgs` referenced in probes.go (8 hits) and executor.go (line 511, 521, 537); scenario 003 cross-references `dark-factory healthcheck` (line 9)
- AC 12/13: `git diff origin/master..HEAD` shows 0 bare `return err` and 0 `fmt.Errorf` in healthcheck dirs
- AC 14: `make precommit` exit 0 (ready to commit)
- AC 15: 7 healthcheck entries in CHANGELOG.md (lines 13, 14, 18, 31, 32, 33, 34)
- AC 16: `docs/running.md` line 264 `## Healthcheck` heading + multiple `dark-factory healthcheck` references
- AC 17: `docs/troubleshooting.md` line 3 mentions `dark-factory healthcheck` < next `## ` at line 5
**Verdict:** PASS
