---
status: completed
spec: [078-auto-approve-generated-prompts]
summary: 'Wired autoApprovePrompts into the spec generator: extended PromptManager interface with NormalizeFilenames, regenerated the Counterfeiter mock, added autoApproveGeneratedPrompts and approvePromptFromInbox methods to dockerSpecGenerator, called auto-approve from finalizePrompted, passed cfg.AutoApprovePrompts and cfg.Prompts.InProgressDir through CreateSpecGenerator in factory.go, added four new test cases covering audit-pass, audit-fail, already-approved idempotency, and disabled-flag paths, and added the CHANGELOG entry under ## Unreleased.'
container: dark-factory-386-spec-078-generator
dark-factory-version: v0.154.0
created: "2026-05-07T22:00:00Z"
queued: "2026-05-07T21:50:05Z"
started: "2026-05-07T22:23:42Z"
completed: "2026-05-07T22:27:33Z"
branch: dark-factory/auto-approve-generated-prompts
---

<summary>
- When `autoApprovePrompts` is `true`, the generator automatically audits each prompt it creates before handing off to the executor queue
- Audit runs the existing `/dark-factory:audit-prompt` slash command in the YOLO container — the same `executor.Execute()` invocation mechanism already used by generate-prompts
- A nil return from `executor.Execute()` means audit passed; a non-nil return (non-zero exit, timeout, crash) means audit failed — fail-closed semantics
- On audit pass, the generator performs the same approve operation as `dark-factory prompt approve <name>`: moves the file from inbox to queue, marks it approved, normalizes filenames
- On audit failure, the generator logs the failure prominently and stops auditing/approving remaining prompts for that spec; the spec stays in "prompted" state for human intervention
- Hand-written prompts (not produced by generate-prompts) are never touched — the auto-approve only runs on files returned by `executeAndFinalize`
- If a prompt is already absent from the inbox when auto-approve fires (manually approved), auto-approve skips it silently — no error, no double-approval
- The generator's `PromptManager` interface gains `NormalizeFilenames` so the approve operation can renumber the queue after each move
- `NewSpecGenerator` gains two new trailing parameters: `autoApprovePrompts bool` and `queueDir string` (the in-progress directory prompts are approved into)
- `CreateSpecGenerator` in `pkg/factory/factory.go` passes `cfg.AutoApprovePrompts` and `cfg.Prompts.InProgressDir`; no change to `CreateRunner` or `CreateOneShotRunner` callers
- All existing tests continue to pass with the two new trailing `false/""` arguments; new tests cover audit-pass, audit-fail, and already-approved idempotency paths
- `make precommit` passes
</summary>

<objective>
Wire the `autoApprovePrompts` config value (added in prompt 1) into the spec generator so that, when enabled, each newly generated prompt is audited via the existing YOLO executor mechanism and, on pass, approved using the same operation as `dark-factory prompt approve`. Audit failure stops further auto-approvals for the spec without changing the spec's status, so the user can intervene manually.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Prompt 1 (spec-078-config) must be merged before this prompt runs. Verify that `pkg/config/config.go` contains `AutoApprovePrompts bool` and that `pkg/factory/factory.go` references `cfg.AutoApprovePrompts` in `LogEffectiveConfig`.

Files to read in full before editing:
- `pkg/generator/generator.go` — the full file; key sections:
  - `dockerSpecGenerator` struct (~line 62) — where new fields are added
  - `NewSpecGenerator` constructor (~line 33) — where new params are added
  - `executeAndFinalize` (~line 164) — where `newFiles` slice is built and returned
  - `finalizePrompted` (~line 225) — where spec is set to "prompted" and `newFiles` processed; call auto-approve HERE after `sf.SetStatus(string(spec.StatusPrompted))` and `sf.Save(ctx)` succeed
- `pkg/generator/prompt_manager.go` — the `PromptManager` interface (add `NormalizeFilenames`)
- `pkg/generator/generator_test.go` — full file; understand the test structure before editing
- `pkg/cmd/approve.go` — `approveFromInbox` is the reference implementation for the approve operation (rename + Load + MarkApproved + Save + NormalizeFilenames)
- `pkg/factory/factory.go` — `CreateSpecGenerator` (~line 599); update to pass two new trailing args
- `mocks/generator-prompt-manager.go` — the Counterfeiter mock; regenerate after interface change
- `pkg/prompt/prompt.go` — `StripNumberPrefix` package function and `MarkApproved()` method on `PromptFile`; verify both exist before use
</context>

<requirements>

## 1. Extend `generator.PromptManager` interface in `pkg/generator/prompt_manager.go`

Add `NormalizeFilenames` to the interface:

```go
// PromptManager is the subset of prompt.Manager that the generator package uses.
type PromptManager interface {
    Load(ctx context.Context, path string) (*prompt.PromptFile, error)
    NormalizeFilenames(ctx context.Context, dir string) ([]prompt.Rename, error)
}
```

This matches the signature of `(*prompt.Manager).NormalizeFilenames` exactly. The generator needs it to renumber the queue after moving a prompt into it (same step as `approveFromInbox` in `pkg/cmd/approve.go`).

## 2. Regenerate the Counterfeiter mock

Run:
```bash
go generate ./pkg/generator/...
```

This regenerates `mocks/generator-prompt-manager.go` to include `NormalizeFilenames`. Verify the generated file compiles with `go build ./mocks/...`.

Existing tests using `mocks.GeneratorPromptManager` continue to work: Counterfeiter generates zero-value returns for the new method, and existing tests do not call `NormalizeFilenames` (they don't trigger the auto-approve path).

## 3. Update `dockerSpecGenerator` struct and `NewSpecGenerator` constructor

### 3a. Add two fields to the struct

In `dockerSpecGenerator`, add after `promptManager PromptManager`:

```go
autoApprovePrompts bool
queueDir           string // in-progress dir; prompts are approved into here
```

### 3b. Update `NewSpecGenerator` signature

Add two new parameters as the LAST parameters (after `pm PromptManager`):

```go
func NewSpecGenerator(
    executor executor.Executor,
    containerChecker executor.ContainerChecker,
    inboxDir string,
    completedDir string,
    specsDir string,
    logDir string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    slugMigrator slugmigrator.Migrator,
    generateCommand string,
    additionalInstructions string,
    maxPromptDuration time.Duration,
    pm PromptManager,
    autoApprovePrompts bool,   // NEW: enables audit+approve of generated prompts
    queueDir string,           // NEW: in-progress dir for approved prompts
) SpecGenerator {
    return &dockerSpecGenerator{
        // ... all existing fields ...
        promptManager:     pm,
        autoApprovePrompts: autoApprovePrompts,
        queueDir:          queueDir,
    }
}
```

**FREEZE all other fields and logic in `NewSpecGenerator`** — only add the two new parameters and wire them into the struct.

## 4. Add `autoApproveGeneratedPrompts` method

Add a new method on `*dockerSpecGenerator`. It is called from `finalizePrompted` after the spec is saved as "prompted". It is a no-op when `g.autoApprovePrompts` is false.

```go
// autoApproveGeneratedPrompts audits and approves each newly generated prompt.
// It runs the /dark-factory:audit-prompt slash command via the existing YOLO executor.
// On audit pass: moves the prompt from inbox to queue and marks it approved.
// On audit failure: logs the failure and stops processing remaining prompts for this spec.
// Returns nil in all cases — audit failure is surfaced via log but does not change spec status.
func (g *dockerSpecGenerator) autoApproveGeneratedPrompts(
    ctx context.Context,
    specBasename string,
    newFiles []string,
) {
    if !g.autoApprovePrompts {
        return
    }
    for _, promptPath := range newFiles {
        promptBasename := filepath.Base(promptPath)

        // Skip if the file is no longer in the inbox (already manually approved).
        if _, statErr := os.Stat(promptPath); os.IsNotExist(statErr) {
            slog.Info(
                "auto-approve: prompt no longer in inbox, skipping (likely manually approved)",
                "prompt", promptBasename,
            )
            continue
        }

        // Run audit via the existing executor mechanism.
        auditContainerName := "dark-factory-audit-" + specBasename + "-" +
            strings.TrimSuffix(promptBasename, ".md")
        auditLogFile := filepath.Join(
            g.logDir,
            "audit-"+specBasename+"-"+promptBasename+".log",
        )
        auditContent := "/dark-factory:audit-prompt " + promptPath

        slog.Info("auto-approve: auditing generated prompt", "prompt", promptBasename)
        if err := g.executor.Execute(ctx, auditContent, auditLogFile, auditContainerName); err != nil {
            slog.Error(
                "auto-approve: audit FAILED for generated prompt — remaining prompts for spec will not be auto-approved",
                "spec", specBasename,
                "prompt", promptBasename,
                "error", err,
            )
            return // stop; remaining prompts stay in inbox for human review
        }

        // Audit passed — approve the prompt (same operation as `dark-factory prompt approve`).
        if approveErr := g.approvePromptFromInbox(ctx, promptPath, promptBasename); approveErr != nil {
            slog.Error(
                "auto-approve: approve step failed after audit pass",
                "spec", specBasename,
                "prompt", promptBasename,
                "error", approveErr,
            )
            return // treat approve failure as fatal for this spec's auto-approve run
        }

        slog.Info("auto-approve: approved generated prompt", "prompt", promptBasename)
    }
}

// approvePromptFromInbox moves a prompt from inbox to the queue and marks it approved.
// This replicates the core of approveFromInbox in pkg/cmd/approve.go without the fuzzy search.
// If the file is no longer in inbox (already moved), the error is logged and returned.
func (g *dockerSpecGenerator) approvePromptFromInbox(
    ctx context.Context,
    inboxPath string,
    promptBasename string,
) error {
    // Strip any numeric prefix so NormalizeFilenames can assign the correct sequence number.
    stripped := prompt.StripNumberPrefix(promptBasename)
    queuePath := filepath.Join(g.queueDir, stripped)

    if err := os.Rename(inboxPath, queuePath); err != nil {
        return errors.Wrapf(ctx, err, "move prompt %s from inbox to queue", promptBasename)
    }

    pf, err := g.promptManager.Load(ctx, queuePath)
    if err != nil {
        return errors.Wrapf(ctx, err, "load prompt after move: %s", stripped)
    }
    pf.MarkApproved()
    if err := pf.Save(ctx); err != nil {
        return errors.Wrapf(ctx, err, "save approved prompt: %s", stripped)
    }

    if _, err := g.promptManager.NormalizeFilenames(ctx, g.queueDir); err != nil {
        // Non-fatal: log and continue — the prompt is approved even if renaming fails.
        slog.Warn("auto-approve: normalize filenames failed after approve", "error", err)
    }

    return nil
}
```

**Required imports** (add only what is not already imported in `generator.go`):
- `"github.com/bborbe/errors"` — already imported
- `"github.com/bborbe/dark-factory/pkg/prompt"` — add if not already present

Verify that `prompt.StripNumberPrefix` exists:
```bash
grep -n "func StripNumberPrefix" pkg/prompt/prompt.go
```
If the function is named differently, use the actual function name.

Verify that `pf.MarkApproved()` exists:
```bash
grep -n "func.*MarkApproved" pkg/prompt/prompt.go
```

## 5. Call `autoApproveGeneratedPrompts` from `finalizePrompted`

In `finalizePrompted`, AFTER `sf.Save(ctx)` succeeds and the spec is marked "prompted", call:

```go
// finalizePrompted loads the spec, inherits metadata to new prompts, and sets status to prompted.
func (g *dockerSpecGenerator) finalizePrompted(
    ctx context.Context,
    specPath string,
    newFiles []string,
) error {
    sf, err := spec.Load(ctx, specPath, g.currentDateTimeGetter)
    if err != nil {
        return errors.Wrap(ctx, err, "load spec file")
    }

    specBranch := sf.Frontmatter.Branch
    specIssue := sf.Frontmatter.Issue
    if specBranch != "" || specIssue != "" {
        if err := inheritFromSpec(ctx, newFiles, specBranch, specIssue, g.promptManager); err != nil {
            return errors.Wrap(ctx, err, "inherit spec metadata to prompts")
        }
    }

    sf.SetStatus(string(spec.StatusPrompted))
    if err := sf.Save(ctx); err != nil {
        return errors.Wrap(ctx, err, "save spec file")
    }

    // Auto-approve generated prompts if configured. This runs AFTER the spec is "prompted"
    // so that audit failure leaves the spec visible to the user in "prompted" state.
    // autoApproveGeneratedPrompts is a no-op when autoApprovePrompts is false.
    specBasename := strings.TrimSuffix(filepath.Base(specPath), ".md")
    g.autoApproveGeneratedPrompts(ctx, specBasename, newFiles)

    return nil
}
```

**IMPORTANT**: `autoApproveGeneratedPrompts` returns no error — audit failure is intentionally non-fatal to `finalizePrompted`. The spec stays in "prompted" state and the user intervenes manually. Do NOT propagate audit failures as errors from `finalizePrompted`.

## 6. Update `CreateSpecGenerator` in `pkg/factory/factory.go`

Pass the two new trailing arguments to `generator.NewSpecGenerator`:

```go
func CreateSpecGenerator(
    cfg config.Config,
    containerImage string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    slugMigrator slugmigrator.Migrator,
    promptManager *prompt.Manager,
) generator.SpecGenerator {
    return generator.NewSpecGenerator(
        executor.NewDockerExecutor(
            containerImage,
            project.Resolve(cfg.ProjectName).String(),
            cfg.Model,
            cfg.NetrcFile,
            cfg.GitconfigFile,
            cfg.Env,
            cfg.ExtraMounts,
            cfg.ResolvedClaudeDir(),
            cfg.ParsedMaxPromptDuration(),
            currentDateTimeGetter,
            formatter.NewFormatter(),
            false, // hideGit — spec generators never need .git masking
        ),
        executor.NewDockerContainerChecker(currentDateTimeGetter),
        cfg.Prompts.InboxDir,
        cfg.Prompts.CompletedDir,
        cfg.Specs.InboxDir,
        cfg.Specs.LogDir,
        currentDateTimeGetter,
        slugMigrator,
        cfg.GenerateCommand,
        cfg.AdditionalInstructions,
        cfg.ParsedMaxPromptDuration(),
        promptManager,
        cfg.AutoApprovePrompts,        // NEW: pass through from merged config
        cfg.Prompts.InProgressDir,     // NEW: queue dir for approved prompts
    )
}
```

**Only add the two trailing arguments.** Freeze all other arguments — any change to existing arguments is a bug.

## 7. Update `generator_test.go` — add trailing args to `NewSpecGenerator` call

In `generator_test.go`, the `BeforeEach` block constructs `sg` via `generator.NewSpecGenerator(...)`. Add `false, ""` as the last two arguments to preserve existing test behavior:

```go
sg = generator.NewSpecGenerator(
    executor,
    containerChecker,
    inboxDir,
    completedDir,
    specsDir,
    logDir,
    libtime.NewCurrentDateTime(),
    &mocks.SpecSlugMigrator{},
    "/dark-factory:generate-prompts-for-spec",
    "",
    0,
    promptMgr,
    false, // autoApprovePrompts — disabled for existing tests
    "",    // queueDir — not used when autoApprovePrompts is false
)
```

Find the call site precisely with:
```bash
grep -n "NewSpecGenerator" pkg/generator/generator_test.go
```

Do NOT guess line numbers.

## 8. Add new tests to `generator_test.go`

Add a new `Describe("auto-approve generated prompts", ...)` block. These tests use a dedicated `queueDir` temp directory. The `promptMgr` mock stubs are set per `It` block.

### 8a. When `autoApprovePrompts` is false, executor is never called for audit

Use `autoApprovePrompts: false`. After `sg.Generate(ctx, specPath)` returns nil (with a new file written in inboxDir by the executor stub), assert:
- `executor.ExecuteCallCount() == 1` (only the generate call, no audit call)
- The generated file remains in `inboxDir` (not moved to `queueDir`)

### 8b. When `autoApprovePrompts` is true and audit passes, prompt is approved

Set up:
- `executor` stub: first call (generate) writes a new file to `inboxDir` and returns nil; second call (audit) returns nil (audit passed)
- `promptMgr.NormalizeFilenamesReturns(nil, nil)`
- Real `promptMgr.LoadStub` that loads the actual file (same pattern as existing tests)
- `queueDir` is a real temp directory

Assert after `sg.Generate(ctx, specPath)`:
- `executor.ExecuteCallCount() == 2` (generate + audit)
- The second Execute call's content argument starts with `"/dark-factory:audit-prompt "`
- The generated file no longer exists in `inboxDir` (moved to `queueDir`)
- A file with the same base name (possibly renumbered) exists in `queueDir`
- `promptMgr.NormalizeFilenamesCallCount() == 1`

### 8c. When audit fails, prompt stays in inbox and remaining prompts are not audited

Set up a generate stub that writes TWO files to `inboxDir`. Audit executor stub:
- First audit call (for file 1): returns an error
- (No second audit call expected)

Assert after `sg.Generate(ctx, specPath)`:
- `executor.ExecuteCallCount() == 2` (generate + first audit only)
- Both generated files remain in `inboxDir` (neither approved)
- `promptMgr.NormalizeFilenamesCallCount() == 0`
- The function returns nil (spec stays in "prompted" state — audit failure is non-fatal)

### 8d. When prompt is already absent from inbox, auto-approve skips silently

Set up:
- Generate stub writes one file to `inboxDir`, then the test REMOVES that file (simulating manual approve)
- Audit executor stub: NOT called

Assert after `sg.Generate(ctx, specPath)`:
- `executor.ExecuteCallCount() == 1` (generate only, no audit)
- No error returned

**Note on test structure**: Each sub-test needs its own `SpecGenerator` instance (built with `autoApprovePrompts: true` and a real `queueDir` temp dir). Build it inside the `BeforeEach` or `It` block of the new `Describe`, NOT in the outer `BeforeEach` which creates the `false`-configured generator.

Pattern for creating the auto-approve generator in tests:
```go
queueDir, err = os.MkdirTemp("", "generator-queue-*")
Expect(err).NotTo(HaveOccurred())

sgAutoApprove = generator.NewSpecGenerator(
    executor,
    containerChecker,
    inboxDir,
    completedDir,
    specsDir,
    logDir,
    libtime.NewCurrentDateTime(),
    &mocks.SpecSlugMigrator{},
    "/dark-factory:generate-prompts-for-spec",
    "",
    0,
    promptMgr,
    true,     // autoApprovePrompts enabled
    queueDir, // where approved prompts land
)
```

Clean up `queueDir` in the `AfterEach` that cleans up `inboxDir`, etc.

## 9. Add CHANGELOG entry

Add under `## Unreleased` in `CHANGELOG.md`:

```markdown
- feat: When `autoApprovePrompts: true`, daemon audits and auto-approves each generated prompt via the existing YOLO executor; audit failure stops further auto-approvals for the spec without changing spec status
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Default behavior (when `autoApprovePrompts` is false) must be byte-for-byte identical to today: `autoApproveGeneratedPrompts` is an early return when `!g.autoApprovePrompts`
- The approve operation must replicate `approveFromInbox` in `pkg/cmd/approve.go` exactly: rename + Load + MarkApproved + Save + NormalizeFilenames. No new "auto-approved" status — `MarkApproved()` sets the same status as `dark-factory prompt approve`
- The audit invocation uses `g.executor.Execute()` — the same executor already present in `dockerSpecGenerator`. No second executor or second mechanism
- `autoApproveGeneratedPrompts` returns no error. Audit failure is non-fatal to `finalizePrompted`. The spec stays in "prompted" state
- Hand-written prompts (not in `newFiles` returned by `executeAndFinalize`) are never touched
- Wrap all errors with `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`
- `prompt.StripNumberPrefix` and `pf.MarkApproved()` must be grep-verified before use (requirement 4 includes the commands)
- `NormalizeFilenames` failure inside `approvePromptFromInbox` is non-fatal — log a warning, return nil (the prompt is already approved)
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Existing tests must still pass; the two new trailing parameters (`false`, `""`) are added to the existing `NewSpecGenerator` call in `generator_test.go`
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "NormalizeFilenames" pkg/generator/prompt_manager.go` — one occurrence in the interface
2. `grep -n "autoApprovePrompts\|queueDir" pkg/generator/generator.go` — struct fields, constructor params, and usage in `autoApproveGeneratedPrompts` (≥ 8 occurrences)
3. `grep -n "autoApproveGeneratedPrompts\|approvePromptFromInbox" pkg/generator/generator.go` — both functions defined (≥ 4 occurrences each: definition + call site)
4. `grep -n "AutoApprovePrompts\|InProgressDir" pkg/factory/factory.go` — two new trailing args in `CreateSpecGenerator` (2 occurrences)
5. `grep -n "false.*autoApprovePrompts\|false, \"\"" pkg/generator/generator_test.go` — updated BeforeEach call
6. `grep -n "Describe.*auto-approve" pkg/generator/generator_test.go` — the new test block exists
7. `go test ./pkg/generator/... -count=1 -v` — all generator tests pass, including the 4 new auto-approve cases
8. `grep -n "audit-prompt" pkg/generator/generator.go` — the audit content string `"/dark-factory:audit-prompt "` appears in `autoApproveGeneratedPrompts`
</verification>
