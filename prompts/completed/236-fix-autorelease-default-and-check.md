---
status: completed
summary: Removed autoRelease requires autoMerge validation and made handleDirectWorkflow respect autoRelease flag — commit-only when false, tag+push when true
container: dark-factory-236-fix-autorelease-default-and-check
dark-factory-version: v0.80.0-1-g2b37ac1
created: "2026-04-01T00:00:00Z"
queued: "2026-04-01T08:47:32Z"
started: "2026-04-01T08:47:33Z"
completed: "2026-04-01T09:00:00Z"
---

<summary>
- Direct workflow now respects the autoRelease flag instead of always releasing
- When autoRelease is false (default), changes are committed with changelog under "## Unreleased" but not tagged or pushed
- When autoRelease is true, current behavior preserved (tag + push)
- Branch completion also respects autoRelease — skips release when false
- Validation rule "autoRelease requires autoMerge" removed — autoRelease works in all workflows
- Default stays false — projects wanting auto-release must set `autoRelease: true`
</summary>

<objective>
Fix autoRelease so the flag is actually respected in all code paths. Currently handleDirectWorkflow always releases regardless of the flag — make it commit-only when autoRelease is false. Also remove the incorrect validation that ties autoRelease to autoMerge.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-testing.md` — Ginkgo/Gomega test conventions.

Key files to read before making changes:
- `pkg/config/config.go` — `Defaults()` function, `AutoRelease` field, `Validate()` method (contains "autoRelease requires autoMerge" rule to remove)
- `pkg/processor/processor.go` — `handleDirectWorkflow()` method (~line 1223), `handleBranchCompletion()` (~line 1267), autoRelease field usage
- `docs/configuration.md` — autoRelease documentation (currently says "requires autoMerge")
</context>

<requirements>

## 1. Remove "autoRelease requires autoMerge" validation

In `pkg/config/config.go`, in `Validate()`, remove the block:
```go
if c.AutoRelease && !c.AutoMerge {
    return errors.Errorf(ctx, "autoRelease requires autoMerge")
}
```

autoRelease should work independently in all workflows: direct, PR, and branch.

## 2. Respect autoRelease in handleDirectWorkflow

In `pkg/processor/processor.go`, in `handleDirectWorkflow()`, at the "With CHANGELOG" block (~line 1249):

Currently:
```go
// With CHANGELOG: rename ## Unreleased to version, bump version, tag, push
bump := git.DetermineBumpFromChangelog(ctx, ".")
nextVersion, err := p.releaser.GetNextVersion(gitCtx, bump)
```

Change to:
```go
// With CHANGELOG but autoRelease disabled: commit only, keep "## Unreleased"
if !p.autoRelease {
    if err := p.releaser.CommitOnly(gitCtx, title); err != nil {
        return errors.Wrap(ctx, err, "commit without release")
    }
    slog.Info("committed changes (autoRelease disabled, skipping tag)")
    return nil
}

// With CHANGELOG and autoRelease enabled: rename ## Unreleased to version, tag, push
bump := git.DetermineBumpFromChangelog(ctx, ".")
nextVersion, err := p.releaser.GetNextVersion(gitCtx, bump)
```

## 3. Verify handleBranchCompletion

`handleBranchCompletion()` (~line 1298) calls `handleDirectWorkflow` with empty featureBranch — the fix in requirement 2 applies here too. When `autoRelease: false`, branch completion will commit but skip tagging. Verify this is the case by reading the code — no additional change needed.

## 4. Update docs

In `docs/configuration.md`:
- Remove "requires autoMerge" from the autoRelease description
- Document that `autoRelease: false` (default) commits with "## Unreleased" changelog but skips tag/push
- Document that `autoRelease: true` tags and pushes after each prompt completion, works in all workflows

## 5. Update tests

In `pkg/config/config_test.go`:
- Remove or update test that validates "autoRelease requires autoMerge" error

In `pkg/processor/processor_test.go` or `processor_internal_test.go` (follow existing patterns):
- Add test: autoRelease=false with CHANGELOG → calls CommitOnly, does NOT call CommitAndRelease
- Add test: autoRelease=true with CHANGELOG → calls CommitAndRelease (current behavior)
- Verify existing tests still pass

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Default `autoRelease: false` stays unchanged
- Use `github.com/bborbe/errors` for error wrapping
- Existing tests must still pass (update validation test that asserts the removed rule)
</constraints>

<verification>
```bash
make precommit
```
</verification>
