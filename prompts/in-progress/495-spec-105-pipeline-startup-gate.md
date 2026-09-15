---
status: approved
spec: [105-refuse-startup-on-unfinished-pipeline]
created: "2026-09-15T07:47:56Z"
queued: "2026-09-15T07:54:23Z"
---

<summary>

- The daemon gains a startup gate that refuses to start when the pipeline is not clean, and says which prompt numbers block it — a blocked queue can no longer masquerade as an idle one.
- The gate asks the same detector that powers `dark-factory doctor` for findings instead of scanning the tree itself — one detector set, two callers, so the two can never drift apart.
- The gate ships **disabled**, so today's daemon behaviour is unchanged; arming it is a deliberate one-line flip of a single named constant.
- The gate has independent enable and skip controls, mirroring the shape of the healthcheck gate that already guards daemon startup.
- The gate never caches a pass — a leftover that lands mid-session is caught on the next start, unlike the healthcheck gate which caches because its probes are expensive.
- `dark-factory doctor`'s own behaviour is untouched: it keeps its existing exit-code contract and shares one checker construction with the new gate instead of duplicating it.
- A clean-tree daemon still runs its baseline check first and still exits when the baseline is broken.

</summary>

<objective>
Add a daemon-startup gate that calls the `pkg/doctor` checker and refuses to start when it reports findings, naming the offending prompt numbers, so unfinished pipeline state is discovered at startup instead of when new work mysteriously never executes. The gate ships disabled and is wired into the daemon startup path only.
</objective>

<context>

Read `/workspace/CLAUDE.md` first for project conventions.

Read the parent spec end-to-end — it is the contract for this prompt:
- `/workspace/specs/in-progress/105-refuse-startup-on-unfinished-pipeline.md` — Problem, Goal, Non-goals (every row: no second detector, no mtime staleness, no caching, no auto-remediation, no exit-code change, do not arm by default, do not wire it into CI), Acceptance Criteria AC6, AC7, AC8, AC9.

PREREQUISITE: prompt 1 MUST already be on the tree. Verify with `grep -n 'CategoryMissingCompletedPrompt' /workspace/pkg/doctor/doctor.go`. If that returns nothing, STOP and report `Status: failed` with message `"prompt 1 (missing-completed detector) not yet on tree"`.

Read these files END-TO-END before editing:
- `/workspace/pkg/healthcheckgate/gate.go` — the gate shape to mirror: `Gate` interface with `Check(ctx) error`, `//counterfeiter:generate -o ../../mocks/healthcheck-gate.go --fake-name HealthcheckGate . Gate` above it, `NewGate(enabled bool, skip bool, ...)`, private `gate` struct, and a `Check` that short-circuits on `!enabled` then `skip` with a `slog.Info` line each.
- `/workspace/pkg/healthcheckgate/doc.go` and `/workspace/pkg/healthcheckgate/healthcheckgate_suite_test.go` — package doc + suite-file boilerplate (`package <pkg>_test`, `//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate`, `time.Local = time.UTC`, `format.TruncatedDiff = false`, `RunSpecs`).
- `/workspace/pkg/healthcheckgate/gate_test.go` — the `Context`/`It` layout for disabled / skip / pass / fail, and the fake-injection style (`&mocks.HealthcheckGateCommand{}`, `XReturns`, `XCallCount()`).
- `/workspace/pkg/doctor/doctor.go` — `Checker` interface (`Check(ctx context.Context) ([]Finding, error)`), `Finding` (`Category`, `TargetPaths`, `SpecID`, `Detail`, `FixCommand`), `Deps`, `NewChecker(deps Deps) Checker`.
- `/workspace/mocks/doctor-checker.go` — the existing counterfeiter fake you will inject in tests (`&mocks.DoctorChecker{}`, `CheckReturns([]doctor.Finding, error)`, `CheckReturnsOnCall(i, ...)`, `CheckCallCount()`).
- `/workspace/pkg/runner/runner.go` — `NewRunner` parameter list (the last four are `hideGit`, `preflightChecker`, `logWriter`, `healthcheckGate`, `skipContainerReconcile`), the `runner` struct, `Run` (the `runStartupPreflight` then `runStartupHealthcheck` sequence before the watcher loop), and `runStartupHealthcheck` as the method shape to mirror.
- `/workspace/pkg/runner/runner_test.go` — the 11 `runner.NewRunner(...)` call sites and the `Describe("startup healthcheck gate", ...)` block (~line 1654) whose structure (`newRunnerWithGate` closure, error-propagates / proceeds / nil-gate cases) you mirror.
- `/workspace/pkg/factory/factory.go` — `CreateRunner` (~line 351, note `healthcheckGate := CreateHealthcheckGate(...)` and the `runner.NewRunner(...)` call near the end), `CreateDoctorCommand` (~line 1314, the inline `doctor.Deps{...}` literal + `doctor.NewChecker`), `CreateHealthcheckGate` (~line 1390) as the factory-function shape, and `createPromptManager` (~line 206, a pure constructor returning `(*prompt.Manager, git.Releaser)`).
- `/workspace/pkg/factory/factory_test.go` — the `Describe("CreateRunner")` block (~line 43) and the pre-existing `CreateRunner.Run returns ErrPreflightFailed` test (~line 557) that AC9 depends on.
- `/workspace/pkg/config/config.go` — `Config`, `PromptsConfig`, `SpecsConfig` field names, and `Defaults()`.

Read these coding-plugin docs (in-container paths — the prompt runs inside a YOLO container):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`

</context>

<requirements>

## 1. Create `pkg/pipelinegate` (doc.go + gate.go)

**`/workspace/pkg/pipelinegate/doc.go`** — BSD license header copied verbatim from `/workspace/pkg/healthcheckgate/doc.go`, then:

```go
// Package pipelinegate refuses daemon startup when `dark-factory doctor`
// reports pipeline findings, naming the offending prompt numbers so a blocked
// queue cannot masquerade as an idle one.
package pipelinegate
```

**`/workspace/pkg/pipelinegate/gate.go`** — BSD license header, then exactly this public surface:

```go
//counterfeiter:generate -o ../../mocks/pipeline-gate.go --fake-name PipelineGate . Gate

// Gate refuses daemon startup when the pipeline is not clean.
type Gate interface {
	// Check returns nil when the gate is disabled, skipped, or doctor reports no
	// findings. Returns a findings-naming error when the pipeline is dirty
	// (the caller treats it as terminal).
	Check(ctx context.Context) error
}
```

- `DefaultEnabled` — a single exported named constant, value `false`, GoDoc'd as the default for the gate's `enabled` control: the gate ships unarmed because arming it is an operational decision owned outside this spec, and flipping this one constant arms it.
- `NewGate(enabled bool, skip bool, checker doctor.Checker) Gate` — pure constructor, private `gate` struct holding the three fields, no logic.
- `(g *gate) Check(ctx context.Context) error` — in this order:
  1. `if !g.enabled` → `slog.Info("pipeline gate disabled")`, return nil. The checker is NOT called on this path.
  2. `if g.skip` → `slog.Info("pipeline gate skipped")`, return nil. The checker is NOT called on this path either.
  3. `findings, err := g.checker.Check(ctx)`; on error return `errors.Wrap(ctx, err, "pipeline check")`.
  4. `if len(findings) == 0` → return nil.
  5. Return `errors.Errorf(ctx, "pipeline not clean: %d finding(s): %s", len(findings), summarizeFindings(findings))`.
- A private helper `summarizeFindings(findings []doctor.Finding) string` rendering one `"<category>: <detail>"` fragment per finding joined with `"; "`, in the order the checker returned them. `Detail` is what carries the offending prompt numbers (prompt 1 pins `prompt number 186 is missing from prompts/completed`), so the gate's error names them without re-deriving anything.

Hard requirements on this package: no cache field, no interval, no notifier, no `libtime`, no filesystem access (`os.ReadDir`/`filepath.Walk` must not appear), no second detection path — `doctor.Checker` is the only source of findings. Bare `slog` is correct here (`pkg/pipelinegate` is not one of the six hot-path packages; `healthcheckgate` does the same).

## 2. Generate the fake before writing tests

Run `cd /workspace && go generate -mod=mod ./pkg/pipelinegate/...` and confirm `/workspace/mocks/pipeline-gate.go` exists. Do NOT hand-write the mock. (`make precommit` regenerates all mocks from scratch, so the directive must be present and correct.)

## 3. Tests for the gate

**`/workspace/pkg/pipelinegate/pipelinegate_suite_test.go`** — copy the boilerplate from `healthcheckgate_suite_test.go` verbatim (`package pipelinegate_test`, the counterfeiter `//go:generate` line, `time.Local = time.UTC`, `format.TruncatedDiff = false`, `RunSpecs(t, "Test Suite", suiteConfig, reporterConfig)` with `suiteConfig.Timeout = 60 * time.Second`).

**`/workspace/pkg/pipelinegate/gate_test.go`** (`package pipelinegate_test`, Ginkgo v2 + Gomega) with a fake `&mocks.DoctorChecker{}` and these cases:

1. **AC7 disabled:** `NewGate(false, false, fakeChecker)` with a checker that returns two `missing-completed-prompt` findings → `Check` returns nil, and `fakeChecker.CheckCallCount()` is 0.
2. **AC7 skip:** `NewGate(true, true, fakeChecker)` with the same findings → `Check` returns nil, and `fakeChecker.CheckCallCount()` is 0.
3. **Enabled + clean:** `NewGate(true, false, fakeChecker)` with no findings → nil, `CheckCallCount()` is 1.
4. **AC6 enabled + findings:** findings whose `Detail` values name 186 and 187 → `Check` returns a non-nil error; `err.Error()` contains `"pipeline not clean"`, `"186"` and `"187"`.
5. **Checker error:** checker returns `stderrors.New("boom")` → non-nil error containing `"pipeline check"` and `"boom"`.
6. **AC8 no caching:** `fakeChecker.CheckReturnsOnCall(0, []doctor.Finding{}, nil)` and `CheckReturnsOnCall(1, findings, nil)`; two consecutive `Check` calls → the first succeeds, the second fails, `CheckCallCount()` is 2.
7. **Default:** `Expect(pipelinegate.DefaultEnabled).To(BeFalse())` — add a comment that the arming decision is owned outside this spec and flipping the constant makes this assertion fail on purpose, so the change is reviewed.

## 4. Wire the gate into the daemon startup path

**`/workspace/pkg/runner/runner.go`:**

- Add the parameter `pipelineGate pipelinegate.Gate` to `NewRunner` immediately after `healthcheckGate healthcheckgate.Gate` (keep `skipContainerReconcile` last), add the matching field to the `runner` struct next to `healthcheckGate`, wire it in the constructor's returned struct literal, and add the `"github.com/bborbe/dark-factory/pkg/pipelinegate"` import.
- In `Run`, directly after the `runStartupHealthcheck` block, add:

```go
	// Startup pipeline gate: refuse to start when the pipeline is not clean.
	if err := r.runStartupPipelineGate(ctx); err != nil {
		return err
	}
```

- Add the method, mirroring `runStartupHealthcheck`:

```go
// runStartupPipelineGate refuses daemon startup when `dark-factory doctor`
// reports findings. Returns nil when the gate is nil (not wired), disabled,
// skipped, or the pipeline is clean. Returns a findings-naming error when the
// pipeline is dirty — terminal, like the preflight and healthcheck gates.
func (r *runner) runStartupPipelineGate(ctx context.Context) error {
	if r.pipelineGate == nil {
		return nil
	}
	if err := r.pipelineGate.Check(ctx); err != nil {
		return errors.Wrap(ctx, err, "pipeline startup gate")
	}
	return nil
}
```

Ordering is deliberate: preflight, then healthcheck, then pipeline — the existing sequence is untouched and the new gate is appended after it. The gate is a daemon-only concern; `runner.NewOneShotRunner` is NOT changed.

**`/workspace/pkg/factory/factory.go`:**

- Extract the shared checker construction. `CreateDoctorCommand` currently builds `doctor.Deps{...}` inline; move that literal into a new unexported helper so exactly one place owns it:

```go
// createDoctorDeps builds the doctor Deps shared by `dark-factory doctor` and
// the daemon-startup pipeline gate — one detector definition, two callers.
// createPromptManager and spec.NewLister are pure constructors, so the extra
// construction is intentional and harmless.
func createDoctorDeps(
	cfg config.Config,
	verifyingStaleHours int,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) doctor.Deps
```

It builds `promptManager, _ := createPromptManager(cfg.Prompts.InboxDir, cfg.Prompts.InProgressDir, cfg.Prompts.CompletedDir, cfg.Prompts.CancelledDir, currentDateTimeGetter)`, a `spec.NewLister(currentDateTimeGetter, cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir, cfg.Specs.RejectedDir)`, and returns the same `doctor.Deps` literal that `CreateDoctorCommand` builds today (all eight dirs, `SpecLister`, `PromptManager`, `CurrentDateTimeGetter`, `VerifyingStaleHours: verifyingStaleHours`).

- In `CreateDoctorCommand`, replace the inline `deps := doctor.Deps{...}` with `deps := createDoctorDeps(cfg, verifyingStaleHours, currentDateTimeGetter)` and keep `checker := doctor.NewChecker(deps)`, the fixer construction, and the return exactly as they are. Its local `promptManager`, `releaser`, `specLister` and `autoCompleter` are still needed for the fixer — leave those lines in place.

- Add the gate factory next to `CreateHealthcheckGate`:

```go
// CreatePipelineGate builds the daemon-startup pipeline gate. The gate's
// enabled/skip logic and the doctor-checker call live in pipelinegate.gate.Check;
// this factory only constructs collaborators and passes them in.
//
// verifyingStaleHours is passed as 0: pkg/doctor's detectVerifyingStale treats a
// non-positive value as its default (24h), and `--verifying-stale-hours` is a
// doctor-only CLI flag that does not apply to the daemon gate.
func CreatePipelineGate(
	cfg config.Config,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) pipelinegate.Gate {
	return pipelinegate.NewGate(
		pipelinegate.DefaultEnabled,
		false, // skip: no CLI flag in this spec — arming is an operational decision owned outside it
		doctor.NewChecker(createDoctorDeps(cfg, 0, currentDateTimeGetter)),
	)
}
```

- In `CreateRunner`, next to the existing `healthcheckGate := CreateHealthcheckGate(...)` line, add `pipelineGate := CreatePipelineGate(cfg, currentDateTimeGetter)` and pass `pipelineGate` into `runner.NewRunner(...)` in the `healthcheckGate` position's immediate successor. The factory body stays branch-free.

## 5. Tests for the wiring

**`/workspace/pkg/runner/runner_test.go`** — every one of the 11 existing `runner.NewRunner(...)` call sites needs the new argument (`nil, // pipelineGate: no gate in tests`) inserted between the `healthcheckGate` argument and `skipContainerReconcile`. Then add a `Describe("startup pipeline gate", ...)` block next to `Describe("startup healthcheck gate", ...)`, using the same closure-helper pattern (`newRunnerWithPipelineGate(inboxDir, inProgressDir, completedDir string, gate pipelinegate.Gate) runner.Runner`) and `&mocks.PipelineGate{}`:

1. `Check` returns an error → `r.Run(ctx)` returns a non-nil error whose message contains `"pipeline startup gate"`, and `fakeGate.CheckCallCount()` is 1.
2. `Check` returns nil → `r.Run(runCtx)` returns nil (use the existing 500ms-timeout watcher/processor stubs pattern).
3. A nil `pipelinegate.Gate` interface (not a typed nil) is a no-op → `r.Run(runCtx)` returns nil.

Note: `runner_test.go` is ~1,780 lines. Keep the addition to roughly the size of the healthcheck block and mention in the completion report's `## Improvements` section if the file now warrants splitting.

**`/workspace/pkg/factory/factory_test.go`** — add a `Describe("CreatePipelineGate", ...)` block:

1. `CreatePipelineGate(cfg, libtime.NewCurrentDateTime())` returns a non-nil gate.
2. **Unarmed by default:** build a temp fixture (`GinkgoT().TempDir()`) with `prompts/completed/185-blocked.md` + `prompts/completed/188-later.md` (`status: completed`) and `prompts/in-progress/186-stuck.md` (`status: failed`), point `cfg.Prompts.InboxDir/InProgressDir/CompletedDir/CancelledDir` and `cfg.Specs.InboxDir/InProgressDir/CompletedDir/RejectedDir` at it, and assert `factory.CreatePipelineGate(cfg, libtime.NewCurrentDateTime()).Check(ctx)` returns nil — the gate ships unarmed, so a dirty pipeline does not refuse startup today. Add a comment that flipping `pipelinegate.DefaultEnabled` makes this fail on purpose.
3. Make (2) non-vacuous: assert that `doctor.NewChecker(...)` over the same `doctor.Deps` (dirs + `prompt.NewManager` + `spec.NewLister`) does report a `doctor.CategoryMissingCompletedPrompt` finding naming 186, so the fixture is known to be dirty. Write those fixture files inline in the test — the `createPromptFile` helper lives in the `pkg/doctor` test package and is not importable from `factory_test` (same content: `---\nstatus: <status>\nspec: \n---\n# Prompt`, mode `0600`, dirs `0750`).

The pre-existing `CreateRunner.Run returns ErrPreflightFailed` test must still pass unchanged — that is AC9's "clean-tree daemon still exits when the baseline is broken".

## 6. CHANGELOG

Append ONE bullet to `## Unreleased` in `/workspace/CHANGELOG.md` (create the section under the intro block if absent; do NOT disturb existing version sections):

```
- feat: add the daemon-startup pipeline gate (`pkg/pipelinegate`) — refuses to start when `dark-factory doctor` reports findings and names the offending prompt numbers, reusing the doctor checker instead of re-deriving state; ships disabled (`pipelinegate.DefaultEnabled = false`) and uncached by design (spec 105 prompt 2)
```

</requirements>

<constraints>

- Do NOT implement a second detector. The gate calls `pkg/doctor`; it does not re-derive state. Two callers of one detector set is the point of the spec.
- Do NOT compute staleness from file mtime, and do NOT touch `pkg/doctor`'s detectors — that is prompt 1's scope.
- Do NOT cache the gate result. `healthcheckgate` caches because a probe sequence is expensive; a pipeline scan is a filesystem read, and a cached pass would hide a leftover that landed mid-session.
- Do NOT auto-remediate. The gate reports and refuses; `doctor --fix` already owns repair.
- Do NOT arm the gate by default — `DefaultEnabled` is `false`. Do NOT add a config field, a CLI flag, or a `--skip-pipeline-gate` argument: arming is an operational decision owned outside this spec, and the `skip` control is a constructor parameter exercised by unit tests.
- Do NOT change `doctor`'s exit code contract for initialized repos (findings → exit 1), and do NOT wire the gate into CI or the merge-gate workflow.
- Do NOT change `runner.NewOneShotRunner` or the one-shot `run` path — the gate is daemon-only.
- Do NOT notify on gate failure — no notifier parameter, no notification event.
- `pkg/factory` and `pkg/runner` are NEUTRAL packages — introduce NO container tokens (`containerName`, `ContainerChecker`, `ContainerStopper`, `containerslot`); `hotpath-execution-naming-check` must stay green.
- All error wrapping via `github.com/bborbe/errors` (`errors.Wrap(ctx, err, "...")`, `errors.Errorf(ctx, "...")`). Never `fmt.Errorf`, never `context.Background()` inside `pkg/`.
- External test packages (`package pipelinegate_test`, `package runner_test`, `package factory_test`), Ginkgo v2 + Gomega, ≥80% statement coverage on the new package. Counterfeiter mocks only — never hand-write a fake.
- Do NOT commit — dark-factory handles git. Existing tests must still pass; `make precommit` must exit 0.

</constraints>

<verification>

```bash
cd /workspace

# 1. Compiles
go build -mod=mod ./...
# expected: exit 0

# 2. Tests for the new package and the two wired packages
go test -mod=mod ./pkg/pipelinegate/... ./pkg/runner/... ./pkg/factory/...
# expected: PASS — including the pre-existing `CreateRunner.Run returns ErrPreflightFailed`
# (AC9: the daemon still exits when the baseline is broken) and the healthcheck-gate block.

# 3. Generated fake
ls /workspace/mocks/pipeline-gate.go
# expected: the file is listed (regenerated by `make generate`; never hand-written)

# 4. Gate reuses the doctor checker — no second detection path
grep -n 'doctor.Checker' /workspace/pkg/pipelinegate/gate.go
# expected: >= 1 line (the injected checker)

grep -rn 'os.ReadDir\|filepath.Walk\|ReadDir' /workspace/pkg/pipelinegate/*.go
# expected: no output (the gate scans nothing itself)

! grep -qE 'cacheKey|CacheKey|Fresh\(|\.Write\(' /workspace/pkg/pipelinegate/gate.go
# expected: exit 0 (no cache plumbing; AC8 itself is enforced by the
# two-consecutive-Check unit test, which is the real evidence)

# 5. Wiring
grep -n 'pipelinegate.NewGate' /workspace/pkg/factory/factory.go
# expected: 1 line (inside CreatePipelineGate)

grep -n 'pipelinegate.DefaultEnabled' /workspace/pkg/factory/factory.go
# expected: 1 line (the gate ships unarmed)

grep -n 'createDoctorDeps' /workspace/pkg/factory/factory.go
# expected: >= 3 lines (declaration + CreateDoctorCommand + CreatePipelineGate)

grep -n 'runStartupPipelineGate' /workspace/pkg/runner/runner.go
# expected: >= 2 lines (the method and its call site in Run)

grep -n 'pipeline startup gate' /workspace/pkg/runner/runner.go
# expected: 1 line (the wrap message)

grep -c 'pipelineGate' /workspace/pkg/runner/runner.go
# expected: >= 3 (parameter, struct field, constructor assignment, nil check)

# 6. The daemon startup order is unchanged except for the appended gate:
#    preflight → healthcheck → pipeline, all before the watcher loop.
grep -n 'runStartupPreflight(ctx)\|runStartupHealthcheck(ctx)\|runStartupPipelineGate(ctx)' /workspace/pkg/runner/runner.go
# expected: the three call sites appear inside Run, in that order

# 7. Final gate
make precommit
# expected: exit 0
```

Before finishing, re-run the whole `<verification>` block and confirm every expectation holds, then walk AC6, AC7, AC8 and AC9 of the spec against the change and state in the completion report how each one is satisfied. AC9 in particular: the eight existing detectors' tests are untouched, `make precommit` is green, and a clean-tree daemon still runs `preflightCommand` and still exits when the baseline is broken.

</verification>

<!-- OPEN QUESTION for the human reviewer (not for the executing agent):
     1. The spec gives the gate `enabled` and `skip` controls (AC7) but no CLI flag for `skip`.
        This prompt therefore wires `skip` as a constructor parameter only (the factory passes
        `false`) and adds no flag. If the intent was an operator-facing `--skip-pipeline-gate`,
        that needs a separate spec: it would change `ParseArgs`'s 8-value return tuple and its
        call sites in main.go.
     2. No documentation change is requested by any AC, so none is made. docs/running.md's
        "Healthcheck startup gate (daemon)" section and docs/architecture-flow.md's terminal-
        policy paragraph still describe only preflight + healthcheck; an operator arming
        `DefaultEnabled` would find no doc for the new gate. Flag at audit time whether the
        arming decision should carry a doc update.
     3. The daemon-level end-to-end (armed gate refuses startup on a dirty pipeline) is
        operator-only: it needs `dark-factory daemon`, which needs docker + the lock. The
        spec's Verification section covers the doctor-level fixtures only; the gate's
        refuse-to-start behaviour is covered here by the runner + gate unit tests.
-->
