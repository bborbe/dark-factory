---
status: completed
spec: [055-preflight-baseline-check]
summary: Wired preflight.Checker into processor struct, NewProcessor, CreateProcessor, CreateRunner, and CreateOneShotRunner; prompts skip when baseline is broken; added internal tests and architecture docs.
container: dark-factory-321-spec-055-wire-and-docs
dark-factory-version: v0.125.1
created: "2026-04-19T12:00:00Z"
queued: "2026-04-19T16:15:50Z"
started: "2026-04-19T16:35:06Z"
completed: "2026-04-19T16:50:06Z"
branch: dark-factory/preflight-baseline-check
---

<summary>
- The processor now runs the preflight baseline check before executing any queued prompt
- When preflight returns false (baseline broken), the prompt stays queued: no status change, no retry increment, no container started
- The preflight checker is wired into `checkPreflightConditions` as the first gate, before dirty-file and git-lock checks
- `NewProcessor` and `CreateProcessor` accept a `preflight.Checker` parameter; `CreateRunner` and `CreateOneShotRunner` construct the checker from `cfg.PreflightCommand`, `cfg.ParsedPreflightInterval()`, and the existing extraMounts/containerImage/projectName config fields
- When `preflightCommand` is empty (or preflight checker is nil), behavior is identical to today
- `docs/architecture-flow.md` documents the new preflight step in Phase 2 of the execution flow
- All existing tests continue to pass
- `make precommit` passes
</summary>

<objective>
Wire `preflight.Checker` into the processor and factory so the daemon runs the baseline check before each prompt. After this prompt, a broken baseline causes the daemon to skip all queued prompts (with notification) until the baseline is fixed.

**Preconditions:** Prompts 1 and 2 have been executed.
- `Config.PreflightCommand`, `Config.PreflightInterval`, `Config.ParsedPreflightInterval()` are in place in `pkg/config/config.go`
- `pkg/preflight` package exists with `Checker` interface, `NewChecker` constructor, and generated mock at `mocks/preflight-checker.go`
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Key files to read before editing:
- `pkg/processor/processor.go` — `processor` struct (line ~117), `NewProcessor` constructor (line ~53), `checkPreflightConditions` (line ~920)
- `pkg/processor/processor_test.go` — `newTestProcessor` helper function (~line 40) to understand how to extend the constructor call
- `pkg/processor/processor_internal_test.go` — `checkPreflightConditions` and `checkDirtyFileThreshold` test blocks (search for "checkDirtyFileThreshold") to understand internal test patterns
- `pkg/processor/export_test.go` — exported symbols for internal tests
- `pkg/factory/factory.go` — `CreateProcessor` (line ~627), `CreateRunner` (line ~256), `CreateOneShotRunner` (line ~359)
- `docs/architecture-flow.md` — Phase 2 execution flow table (lines ~47–64), "What Runs Where" table (lines ~166–176)
- `mocks/preflight-checker.go` — generated fake (created by prompt 2's `go generate`)
</context>

<requirements>

## 1. Add `preflightChecker` field to the `processor` struct

In `pkg/processor/processor.go`, add to the `processor` struct after `maxPromptDuration`:

```go
preflightChecker  preflight.Checker // nil = disabled
```

Add the import:
```go
"github.com/bborbe/dark-factory/pkg/preflight"
```

## 2. Add `preflightChecker` parameter to `NewProcessor`

In `NewProcessor` (around line 53), add as the last parameter:

```go
preflightChecker preflight.Checker,
```

And in the returned `&processor{...}` initializer, add:

```go
preflightChecker:  preflightChecker,
```

## 3. Update `checkPreflightConditions` to call the preflight checker

In `checkPreflightConditions` (around line 920), add the preflight check AS THE FIRST GATE, before the git index lock check:

```go
// checkPreflightConditions runs all pre-execution skip checks in order.
// Returns (true, nil) if the prompt should be skipped this cycle.
func (p *processor) checkPreflightConditions(ctx context.Context) (bool, error) {
	// Baseline preflight check — must pass before any container starts
	if p.preflightChecker != nil {
		ok, err := p.preflightChecker.Check(ctx)
		if err != nil {
			// Unexpected internal error: log and skip this cycle without failing the prompt
			slog.Warn("preflight checker error, skipping prompt this cycle", "error", err)
			return true, nil
		}
		if !ok {
			slog.Info("preflight: baseline broken — prompt stays queued until baseline is fixed")
			return true, nil
		}
	}

	if p.checkGitIndexLock() {
		slog.Warn("git index lock exists, skipping prompt — will retry next cycle")
		return true, nil
	}
	return p.checkDirtyFileThreshold(ctx)
}
```

## 4. Update `CreateProcessor` in `pkg/factory/factory.go`

Add `preflightChecker preflight.Checker` as the last parameter to `CreateProcessor`:

```go
func CreateProcessor(
	// ... existing params ...
	hideGit bool,
	preflightChecker preflight.Checker,  // ADD THIS
) processor.Processor {
```

Add the import:
```go
"github.com/bborbe/dark-factory/pkg/preflight"
```

Pass `preflightChecker` as the last argument to `processor.NewProcessor(...)`:

```go
return processor.NewProcessor(
	// ... existing args ...
	autoRetryLimit, maxPromptDuration,
	preflightChecker,  // ADD THIS
)
```

## 5. Update `CreateRunner` in `pkg/factory/factory.go`

In `CreateRunner` (around line 256), create the preflight checker before calling `CreateProcessor`. Add after the `dirtyFileChecker`/`gitLockChecker` block:

```go
// Create preflight checker from config. nil when preflightCommand is empty (disabled).
var preflightChecker preflight.Checker
if cfg.PreflightCommand != "" {
	projectRoot, rootErr := os.Getwd()
	if rootErr != nil {
		slog.Warn("preflight: could not determine project root, preflight disabled", "error", rootErr)
	} else {
		preflightChecker = preflight.NewChecker(
			cfg.PreflightCommand,
			cfg.ParsedPreflightInterval(),
			projectRoot,
			cfg.ContainerImage,
			cfg.ExtraMounts,
			n,
			projectName,
		)
	}
}
```

Then pass `preflightChecker` as the last arg to `CreateProcessor(...)`.

Add the import if not already present:
```go
"github.com/bborbe/dark-factory/pkg/preflight"
```

Note: `os` is likely already imported; verify before adding.

## 6. Update `CreateOneShotRunner` in `pkg/factory/factory.go`

Apply the identical change to `CreateOneShotRunner` (around line 359). Add the preflight checker creation block (same code as step 5) before the inline `CreateProcessor(...)` call, and pass `preflightChecker` as the last arg.

## 7. Update `newTestProcessor` in `pkg/processor/processor_test.go`

Find the `newTestProcessor` helper function (around line 40). Add `preflightChecker preflight.Checker` as the last parameter and pass it through to `processor.NewProcessor(...)`.

All call sites of `newTestProcessor` in `processor_test.go` will need to be updated to pass `nil` as the last argument (search for all calls to `newTestProcessor` in the file).

Add the import in `processor_test.go`:
```go
"github.com/bborbe/dark-factory/pkg/preflight"
```

**Important:** Search for ALL call sites of `newTestProcessor` in `processor_test.go` and add `nil` as the last argument. There may be many — do not miss any.

## 8. Export `PreflightChecker` field for internal tests in `pkg/processor/export_test.go`

Read the existing `export_test.go` to understand what it exports. Add an exported accessor for the `preflightChecker` field so internal tests can inject it:

```go
// SetPreflightChecker injects a preflight.Checker for internal processor tests.
func (p *processor) SetPreflightChecker(c preflight.Checker) {
	p.preflightChecker = c
}
```

Add the import:
```go
"github.com/bborbe/dark-factory/pkg/preflight"
```

## 9. Write internal tests for `checkPreflightConditions` in `pkg/processor/processor_internal_test.go`

Find the existing `checkDirtyFileThreshold` or `checkPreflightConditions` test block. Add a new `Describe("checkPreflightConditions — preflight checker", ...)` block:

```go
Describe("checkPreflightConditions — preflight checker", func() {
	var (
		ctx            context.Context
		proc           *processor
		fakeNotifier   *mocks.Notifier
		fakeChecker    *mocks.PreflightChecker
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeNotifier = &mocks.Notifier{}
		fakeNotifier.NotifyReturns(nil)
		fakeChecker = &mocks.PreflightChecker{}
		proc = &processor{}
		proc.SetPreflightChecker(fakeChecker)
	})

	It("returns skip=true when preflight checker returns false", func() {
		fakeChecker.CheckReturns(false, nil)
		skip, err := proc.CheckPreflightConditions(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(skip).To(BeTrue())
	})

	It("returns skip=false when preflight checker returns true", func() {
		fakeChecker.CheckReturns(true, nil)
		skip, err := proc.CheckPreflightConditions(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(skip).To(BeFalse())
	})

	It("returns skip=true when preflight checker returns an error (non-fatal)", func() {
		fakeChecker.CheckReturns(false, errors.New(ctx, "internal error"))
		skip, err := proc.CheckPreflightConditions(ctx)
		Expect(err).NotTo(HaveOccurred()) // error is absorbed, not propagated
		Expect(skip).To(BeTrue())
	})

	It("returns skip=false when no preflight checker is set (nil)", func() {
		proc.SetPreflightChecker(nil)
		skip, err := proc.CheckPreflightConditions(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(skip).To(BeFalse())
	})
})
```

For this test to work, `CheckPreflightConditions` must be exported in `export_test.go` (see step 8). Add the export:

```go
// CheckPreflightConditions exposes checkPreflightConditions for internal tests.
func (p *processor) CheckPreflightConditions(ctx context.Context) (bool, error) {
	return p.checkPreflightConditions(ctx)
}
```

Also import `"github.com/bborbe/errors"` in the test file if not already present.

## 10. Update `docs/architecture-flow.md`

### 10a. Add preflight to the Phase 2 execution flow table

Find the table in "Phase 2: Prompt Execution" (around line 47). Currently step 4 is "Setup workflow". Insert a new row for preflight **between step 3 and step 4**, renumbering the subsequent steps:

```
 3.5  Host         Preflight baseline check (preflightCommand in container, cached by HEAD SHA)
```

Specifically, the table currently looks like:
```
 3    Host         Assemble final prompt (see "Prompt Assembly" below)
 4    Host         Setup workflow (branch switch or clone)
 5    Host         Start Docker container with assembled prompt
```

Update it to:
```
 3    Host         Assemble final prompt (see "Prompt Assembly" below)
 3.5  Host         Preflight baseline check (preflightCommand in container, cached by HEAD SHA)
 4    Host         Setup workflow (branch switch or clone)
 5    Host         Start Docker container with assembled prompt
```

(Keep the step numbering from 4 onwards unchanged — the `.5` suffix makes this clear without renumbering all steps.)

### 10b. Add preflight to the "What Runs Where" table

Find the "What Runs Where" table (around line 166). Add a new row:

```markdown
| preflightCommand | Docker container | Verify baseline before prompt; same image as YOLO, cached by HEAD SHA |
```

Insert it before the "validationCommand" row.

## 11. Write CHANGELOG entry

Add or extend `## Unreleased` at the top of `CHANGELOG.md`:

```
- feat: wire preflight baseline checker into processor — prompts skip (not fail) when baseline is broken
- docs: document preflight step in architecture-flow.md execution table and what-runs-where table
```

## 12. Run `make test`

```bash
cd /workspace && make test
```

Must pass before `make precommit`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- When `preflightChecker` is nil (disabled), `checkPreflightConditions` behavior must be IDENTICAL to today — zero code-path change
- Preflight failure must NOT call `handlePromptFailure`, must NOT increment `retryCount`, must NOT move any files — returning `(true, nil)` from `checkPreflightConditions` achieves this (the caller's `return nil` skips the current cycle cleanly)
- When a preflight checker error occurs, log it as WARN and return `(true, nil)` — never propagate the error to the caller (that would risk marking the prompt as failed)
- All existing `newTestProcessor` call sites in `processor_test.go` must be updated — missing one causes a compile error
- The `CreateProcessor` signature change means every call site in `factory.go` must also be updated — there are two: one in `CreateRunner` and one in `CreateOneShotRunner`
- Use `errors.Wrap` from `github.com/bborbe/errors` (no `fmt.Errorf`)
- External test packages use `package processor_test`; internal tests use `package processor`
- The `//nolint:funlen` annotation on `CreateProcessor` must be preserved (it's already there)
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `grep -n "preflightChecker\|PreflightChecker" pkg/processor/processor.go` — at least 3 matches (field, param, usage in checkPreflightConditions)
2. `grep -n "preflightChecker\|PreflightChecker\|preflight.NewChecker" pkg/factory/factory.go` — at least 4 matches (CreateProcessor param, CreateRunner creation, CreateOneShotRunner creation, pass-through)
3. `grep -n "preflight" docs/architecture-flow.md` — at least 2 matches
4. `go test ./pkg/processor/...` — passes
5. `go build ./...` — no compile errors
</verification>
