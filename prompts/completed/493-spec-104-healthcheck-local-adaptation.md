---
status: completed
spec: [104-local-execution-backend]
summary: Disabled the daemon-startup healthcheck gate under backend:local via a pure healthcheckEnabledForBackend(cfg) helper wired into CreateHealthcheckGate — no docker daemon required at runtime; docker path unchanged. Documented the local backend's trust boundary + usage in docs/execution-backends.md (de-hypotheticalized the Adding-a-Backend section) and added a table test. Implemented manually; make precommit green.
completed: "2026-07-12T21:32:18Z"
created: "2026-07-12T19:00:00Z"
queued: "2026-07-12T19:08:22Z"
---

<summary>

- Makes the daemon startup healthcheck gate a no-op when `backend: local`, because its probes all boot Docker containers that are meaningless (and unavailable) in the local backend.
- Guarantees the daemon can start and run a prompt with `backend: local` on a host that has no Docker daemon at all — the whole point of the feature.
- Documents in the execution-backends guide that `local` MUST only be used inside an already-isolated single-tenant environment (the agent pod), that it trusts the host, and that it never auto-detects or falls back.
- Leaves the `backend: docker` path completely unchanged — the healthcheck still runs its full docker probe sequence as today.
- Adds a test proving the local-backend gate short-circuits and never touches docker.

</summary>

<objective>
Adapt the daemon-startup healthcheck so that under `backend: local` the docker probe sequence is skipped (the gate is constructed disabled), so no docker daemon is required at runtime; and document the local backend's trust boundary and correct usage in `docs/execution-backends.md`. The `backend: docker` path is unchanged.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/104-local-execution-backend.md` — Desired Behavior 6; Non-goals; Constraints ("No docker daemon required when backend: local"); Security / Abuse Cases (the whole section — the doc text comes from here); Failure Modes rows "backend: local on a host without docker", "backend: local, claude not on PATH"; Acceptance Criteria 8, 9.

Read these coding-plugin docs (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — construction-only factory, a backend-conditional gate is a construction concern.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — table tests, coverage.
- `/home/node/.claude/plugins/marketplaces/coding/docs/documentation-guide.md` — doc style.

PREREQUISITE: prompts 1–3 MUST be on the tree. If `grep -rn 'createExecutor' pkg/factory/factory.go` returns nothing, STOP and report `Status: failed` with message "prompt 3 (factory switch) not yet on tree".

Read these files END-TO-END before editing:
- `/workspace/pkg/factory/factory.go` — `CreateHealthcheckGate(ctx, cfg, skipHealthcheck, projectName, n, currentDateTimeGetter)` (line ~1335) builds the gate via `healthcheckgate.NewGate(cfg.HealthcheckEnabledValue(), skipHealthcheck, ...)`. The FIRST arg to `NewGate` is the `enabled bool`. Also read `CreateHealthcheckCommand` (line ~1370) — it wires the docker/image/boot/claude/mount probes.
- `/workspace/pkg/healthcheckgate/gate.go` — `NewGate(enabled bool, skip bool, ...)` and `Check(ctx)` (line ~74): when `!enabled`, `Check` logs `"healthcheck gate disabled"` and returns nil WITHOUT running any probe (no docker call). This is the exact short-circuit to leverage.
- `/workspace/pkg/config/config.go` — `HealthcheckEnabledValue() bool` (line ~288); `cfg.Backend` and `config.BackendLocal` (from prompt 1).
- `/workspace/docs/execution-backends.md` — the whole file. You will APPEND a new section documenting the local backend's trust boundary and correct usage.

Verified facts (do not re-derive):
- The gate's `enabled=false` path is a clean, reversible short-circuit — no probe runs, no docker daemon is contacted. Forcing `enabled=false` under `backend: local` is the "skip" option the spec offers (Desired Behavior 6), chosen here over a `claude --version` probe because it is the smallest, most reversible change and adds no new probe surface.
- The standalone `dark-factory healthcheck` CLI subcommand (`pkg/cmd/healthcheck.go`) is an EXPLICIT operator invocation of the docker probes, NOT part of the daemon runtime path. It is OUT OF SCOPE for this prompt — do NOT modify it. AC 9 concerns the daemon RUNTIME ("run a prompt with docker unavailable"), which the daemon gate change covers. Note this scope boundary in the completion report.

</context>

<requirements>

## 1. Disable the healthcheck gate under `backend: local` via a pure helper

The factory body must stay a pure composition — NO conditional or logging in the factory body (go-factory-pattern). Extract the enabled decision into a tiny pure helper and have `CreateHealthcheckGate` call it in ONE composition statement.

In `/workspace/pkg/factory/factory.go`, add the helper (place near the other `create*`/helper funcs):

```go
// healthcheckEnabledForBackend reports whether the daemon-startup healthcheck
// gate should run. Under backend: local the docker probes are meaningless (no
// docker daemon required — spec 104), so the gate is always disabled; otherwise
// it follows the configured value.
func healthcheckEnabledForBackend(cfg config.Config) bool {
	if cfg.Backend == config.BackendLocal {
		return false
	}
	return cfg.HealthcheckEnabledValue()
}
```

In `CreateHealthcheckGate`, replace the first argument to `healthcheckgate.NewGate(...)` so it reads:

```go
enabled := healthcheckEnabledForBackend(cfg)
// ... healthcheckgate.NewGate(enabled, skipHealthcheck, ...)
```

Pass `enabled` as the first argument to `healthcheckgate.NewGate(...)` instead of `cfg.HealthcheckEnabledValue()`. The factory body has NO `if`/no `slog` call — the branch and its rationale live entirely in the pure helper. Do NOT change any other argument. Do NOT alter the probe construction in `CreateHealthcheckCommand` (the probes are simply never run when the gate is disabled).

This satisfies AC 8 (no docker daemon required at runtime when `backend: local`): with the gate disabled, daemon startup contacts no docker daemon via the healthcheck path, and prompt execution goes through the local executor (prompt 2/3) which also never calls docker.

## 2. Test the pure helper (external test via export_test.go)

`healthcheckEnabledForBackend` is unexported and the test is `package factory_test`. Expose it to the external test through the same seam prompt 3 uses: add a re-export line to `/workspace/pkg/factory/export_test.go`, e.g. `var HealthcheckEnabledForBackend = healthcheckEnabledForBackend`.

Add a table test (in `/workspace/pkg/factory/factory_test.go` or the `backend_switch_test.go` from prompt 3, `package factory_test`) asserting the `enabled` computation:
- `backend: docker` (or unset) → follows `HealthcheckEnabledValue()` (both true and false cases).
- `backend: local` → always `false`, even when `HealthcheckEnabled` is true (proves the override wins).

Keep the existing docker-path healthcheck tests passing unchanged.

## 3. Document the local backend's trust boundary and usage

Append a new section to `/workspace/docs/execution-backends.md` titled `## The Local Backend (backend: local)`. Content (sourced from the spec's Security / Abuse Cases and Non-goals — write in the doc's existing prose style):
- What it does: runs `claude` (and the prompt's DoD commands) as a local subprocess in the current working directory, no docker run, no bind mounts, no nested container. Selected via `backend: local` (config field; default `docker`).
- Trust boundary (verbatim intent from the spec Security section): the local backend runs claude with the FULL credentials and filesystem of the dark-factory process — there is no container sandbox. This is acceptable ONLY because the intended caller (an already-isolated, single-tenant, ephemeral pod such as the github-dark-factory-agent Job) IS the isolation boundary.
- MUST-NOT: never enable `backend: local` on a shared, multi-tenant, or developer host — it exposes that host's credentials and filesystem to prompt content. The feature defaults to `docker`, does not auto-enable, and does not auto-detect "am I in a container" (Non-goals) — the operator makes an explicit, documented choice.
- Fail-closed: with `backend: local`, if `claude` is not on `PATH`, dark-factory errors with `claude not found on PATH` and never silently falls back to docker.
- Reattach: a local subprocess dies with the dark-factory process; on restart the local backend cannot reattach — the prompt is re-queued and re-run (safe because execution commits per prompt).
- Healthcheck: the docker probes are skipped under `backend: local` (no docker daemon required); the toolchain (`claude` + DoD tools) is assumed present in `PATH` (provisioned by the image, not by dark-factory).
- Update the "Adding a Backend" section's note if it still says the local backend is "hypothetical" — change wording to reflect that it now exists (`pkg/executor/local_subprocess.go`). Keep the neutral-vs-container-vocabulary rules intact.

## 4. CHANGELOG

Append ONE bullet to `## Unreleased` in `/workspace/CHANGELOG.md`:
```
- feat: skip the docker healthcheck probes under backend: local so no docker daemon is required at runtime; document the local backend trust boundary and usage in docs/execution-backends.md (spec 104 prompt 4)
```

</requirements>

<constraints>

- The `backend: docker` (default) path is UNCHANGED — the full docker probe sequence still runs exactly as today. The override only forces `enabled=false` when `cfg.Backend == config.BackendLocal`.
- No docker daemon may be contacted on the daemon runtime path when `backend: local` (AC 8). The disabled gate is the mechanism.
- Do NOT modify the standalone `dark-factory healthcheck` CLI subcommand (`pkg/cmd/healthcheck.go`) — it is an explicit operator docker probe, out of scope.
- Do NOT auto-detect the backend or auto-enable local (Non-goals). The gate override keys off the explicit `cfg.Backend` value only.
- `pkg/factory` is a NEUTRAL package — the change must introduce NO container tokens (`containerName`/`ContainerChecker`/`ContainerStopper`/`containerslot`); `hotpath-execution-naming-check` must stay green.
- `make precommit` passes; mocks regenerate cleanly.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# compiles
go build -mod=mod ./...
# expected: exit 0

# factory tests (docker path unchanged + new local short-circuit)
go test -mod=mod ./pkg/factory/... ./pkg/healthcheckgate/...
# expected: PASS

# gate disabled under local (source guard — stable token, not a log string)
grep -n 'healthcheckEnabledForBackend' pkg/factory/factory.go
# expected: >= 1 line (the pure helper + its call site)

# doc section present
grep -n 'The Local Backend' docs/execution-backends.md
# expected: >= 1 line
grep -n 'single-tenant\|MUST' docs/execution-backends.md
# expected: >= 1 line (trust-boundary text)

# naming gate still green
make hotpath-execution-naming-check; echo "exit=$?"
# expected: exit=0

# changelog entry
grep -n 'spec 104 prompt 4' CHANGELOG.md
# expected: >= 1 line

make precommit
# expected: exit 0
```

</verification>
