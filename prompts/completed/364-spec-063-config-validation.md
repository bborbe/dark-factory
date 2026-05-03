---
status: completed
spec: [063-bug-autorelease-overrides-pr-workflow]
summary: Added validateAutoReleaseAutoMerge method to Config, wired it into Validate(), added 6 new Ginkgo tests covering all three failure and three success cases, extracted validateServerPort to fix funlen lint violation, and added CHANGELOG Unreleased entry.
container: dark-factory-364-spec-063-config-validation
dark-factory-version: v0.145.1-3-g93401a1
created: "2026-05-03T12:00:00Z"
queued: "2026-05-03T11:27:21Z"
started: "2026-05-03T11:28:12Z"
completed: "2026-05-03T11:37:56Z"
---

<summary>
- Starting dark-factory daemon or run with `pr: true + autoMerge: false + autoRelease: true` now exits non-zero before any prompt is processed
- Error message names all three valid resolutions: set `autoMerge: true`, or set `autoRelease: false`, or set `pr: false`
- Rejection fires at config load (both during `.dark-factory.yaml` parsing and after `--set` overrides), not per-prompt
- All five valid `autoRelease` combinations from the workflows.md table continue to succeed
- The existing `workflow: direct + pr: true` rejection is unaffected
- New Ginkgo unit tests cover the rejection and each of the three valid resolutions
- `CHANGELOG.md` gains an `## Unreleased` entry for the new validation rule
</summary>

<objective>
Add a fail-fast validation rule to `Config.Validate()` that rejects the semantically invalid combination `pr: true + autoMerge: false + autoRelease: true`. This combination is logically impossible: `autoRelease` requires tagging the merged commit on master, but `autoMerge: false` means the feature branch is never merged automatically, so master never receives the change to tag. The error message must name all three valid resolutions.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-validation-framework-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Read these files in full before editing:
- `pkg/config/config.go` — full file; study `validateWorkflowPR` (~line 428) and `validateAutoReview` (~line 438) for the exact method-receiver pattern to follow; study the `Validate()` method (~line 173) to know where to insert the new validation step
- `pkg/config/config_test.go` — read the `autoMerge`/`autoRelease` tests (~line 413–567) to understand the existing valid/invalid combo coverage and the test struct pattern; note the test at ~line 546 ("succeeds for autoRelease true with autoMerge false (autoRelease is independent)") — that test uses `workflow: direct` with `pr: false`, so it must NOT be affected by the new rule

The spec this implements: `specs/in-progress/063-bug-autorelease-overrides-pr-workflow.md`
</context>

<requirements>

## 1. Add `validateAutoReleaseAutoMerge` method to `pkg/config/config.go`

After the existing `validateWorkflowPR` method (~line 428), add:

```go
// validateAutoReleaseAutoMerge rejects the combination of pr: true, autoMerge: false,
// and autoRelease: true. autoRelease requires tagging the merged commit on master, but
// autoMerge: false means the feature branch is never merged automatically — so there is
// no commit on master to tag.
func (c Config) validateAutoReleaseAutoMerge(ctx context.Context) error {
	if c.PR && !c.AutoMerge && c.AutoRelease {
		return errors.Errorf(
			ctx,
			"autoRelease: true with pr: true and autoMerge: false is invalid"+
				" (autoRelease cannot tag a commit that hasn't been merged to master);"+
				" set autoMerge: true, or set autoRelease: false, or set pr: false",
		)
	}
	return nil
}
```

## 2. Wire the new validator into `Validate()` in `pkg/config/config.go`

In the `validation.All{...}` list inside `Validate()`, add the new rule immediately after the existing `autoMerge` validator (which checks `autoMerge requires pr: true`). The existing `autoMerge` validator is at:

```go
validation.Name("autoMerge", validation.HasValidationFunc(func(ctx context.Context) error {
    if c.AutoMerge && !c.PR {
        return errors.Errorf(ctx, "autoMerge requires pr: true")
    }
    return nil
})),
```

Insert after it:

```go
validation.Name("autoRelease", validation.HasValidationFunc(c.validateAutoReleaseAutoMerge)),
```

Do NOT replace the existing `autoMerge` entry — add a new entry after it.

## 3. Add tests to `pkg/config/config_test.go`

Append the following `It` blocks inside the existing `Describe("Validate", func() {` block, after the last existing `autoRelease` test (~line 567).

The test helper struct pattern used by the file is a full `config.Config` literal with all required fields set. Follow the same pattern — copy the required fields from the adjacent tests.

```go
It("fails for pr=true + autoMerge=false + autoRelease=true with workflow branch", func() {
    cfg := config.Config{
        Workflow: config.WorkflowBranch,
        PR:       true,
        AutoMerge: false,
        AutoRelease: true,
        Prompts: config.PromptsConfig{
            InboxDir:      "prompts",
            InProgressDir: "prompts/in-progress",
            CompletedDir:  "prompts/completed",
            LogDir:        "prompts/log",
        },
        ContainerImage: pkg.DefaultContainerImage,
        Model:          "claude-sonnet-4-6",
        DebounceMs:     500,
    }
    err := cfg.Validate(ctx)
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("autoRelease: true with pr: true and autoMerge: false is invalid"))
    Expect(err.Error()).To(ContainSubstring("autoMerge: true"))
    Expect(err.Error()).To(ContainSubstring("autoRelease: false"))
    Expect(err.Error()).To(ContainSubstring("pr: false"))
})

It("fails for pr=true + autoMerge=false + autoRelease=true with workflow clone", func() {
    cfg := config.Config{
        Workflow: config.WorkflowClone,
        PR:       true,
        AutoMerge: false,
        AutoRelease: true,
        Prompts: config.PromptsConfig{
            InboxDir:      "prompts",
            InProgressDir: "prompts/in-progress",
            CompletedDir:  "prompts/completed",
            LogDir:        "prompts/log",
        },
        ContainerImage: pkg.DefaultContainerImage,
        Model:          "claude-sonnet-4-6",
        DebounceMs:     500,
    }
    err := cfg.Validate(ctx)
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("autoRelease: true with pr: true and autoMerge: false is invalid"))
})

It("fails for pr=true + autoMerge=false + autoRelease=true with workflow worktree", func() {
    cfg := config.Config{
        Workflow: config.WorkflowWorktree,
        PR:       true,
        AutoMerge: false,
        AutoRelease: true,
        Prompts: config.PromptsConfig{
            InboxDir:      "prompts",
            InProgressDir: "prompts/in-progress",
            CompletedDir:  "prompts/completed",
            LogDir:        "prompts/log",
        },
        ContainerImage: pkg.DefaultContainerImage,
        Model:          "claude-sonnet-4-6",
        DebounceMs:     500,
    }
    err := cfg.Validate(ctx)
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("autoRelease: true with pr: true and autoMerge: false is invalid"))
})

It("succeeds for pr=true + autoMerge=true + autoRelease=true (autoMerge resolves the conflict)", func() {
    cfg := config.Config{
        Workflow: config.WorkflowBranch,
        PR:       true,
        AutoMerge: true,
        AutoRelease: true,
        Prompts: config.PromptsConfig{
            InboxDir:      "prompts",
            InProgressDir: "prompts/in-progress",
            CompletedDir:  "prompts/completed",
            LogDir:        "prompts/log",
        },
        ContainerImage: pkg.DefaultContainerImage,
        Model:          "claude-sonnet-4-6",
        DebounceMs:     500,
    }
    err := cfg.Validate(ctx)
    Expect(err).NotTo(HaveOccurred())
})

It("succeeds for pr=true + autoMerge=false + autoRelease=false (autoRelease=false resolves the conflict)", func() {
    cfg := config.Config{
        Workflow: config.WorkflowBranch,
        PR:       true,
        AutoMerge: false,
        AutoRelease: false,
        Prompts: config.PromptsConfig{
            InboxDir:      "prompts",
            InProgressDir: "prompts/in-progress",
            CompletedDir:  "prompts/completed",
            LogDir:        "prompts/log",
        },
        ContainerImage: pkg.DefaultContainerImage,
        Model:          "claude-sonnet-4-6",
        DebounceMs:     500,
    }
    err := cfg.Validate(ctx)
    Expect(err).NotTo(HaveOccurred())
})

It("succeeds for pr=false + autoMerge=false + autoRelease=true (pr=false resolves the conflict)", func() {
    cfg := config.Config{
        Workflow: config.WorkflowDirect,
        PR:       false,
        AutoMerge: false,
        AutoRelease: true,
        Prompts: config.PromptsConfig{
            InboxDir:      "prompts",
            InProgressDir: "prompts/in-progress",
            CompletedDir:  "prompts/completed",
            LogDir:        "prompts/log",
        },
        ContainerImage: pkg.DefaultContainerImage,
        Model:          "claude-sonnet-4-6",
        DebounceMs:     500,
    }
    err := cfg.Validate(ctx)
    Expect(err).NotTo(HaveOccurred())
})
```

## 4. Add CHANGELOG entry

At the top of `CHANGELOG.md`, add a new `## Unreleased` section before the first `## vX.Y.Z` section:

```markdown
## Unreleased

- fix: reject pr: true + autoMerge: false + autoRelease: true at config load with actionable error naming all three valid resolutions
```

If `## Unreleased` already exists, append the bullet to it (do not create a duplicate section).

## 5. Run `make test`

```bash
cd /workspace && make test
```

All tests must pass before proceeding.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- The new validator must be a method receiver `func (c Config) validateAutoReleaseAutoMerge(ctx context.Context) error` — consistent with `validateWorkflowPR` and `validateAutoReview`
- The error message must contain the strings `"autoMerge: true"`, `"autoRelease: false"`, and `"pr: false"` so the acceptance-criteria grep for "three valid resolutions" is met
- The existing test at ~line 546 ("succeeds for autoRelease true with autoMerge false (autoRelease is independent)") uses `workflow: direct` and `PR: false` — it must continue to pass because the new rule only fires when `pr: true`
- The existing `workflow: direct + pr: true` rejection (from `validateWorkflowPR`) is untouched
- Do NOT touch `go.mod` / `go.sum` / `vendor/`
- Errors use `errors.Errorf(ctx, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`
- Tests use Ginkgo/Gomega in the existing `package config_test` test file; do not create a new test file
- Existing tests must still pass — do not delete or modify any existing test case
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "validateAutoReleaseAutoMerge" pkg/config/config.go` — method defined and wired into Validate
2. `grep -c "fails for pr=true.*autoRelease=true" pkg/config/config_test.go` — at least 3 matches (one per workflow)
3. `grep -c "succeeds for pr=true.*autoMerge=true.*autoRelease=true\|succeeds for pr=.*autoRelease=false\|succeeds for pr=false.*autoRelease=true" pkg/config/config_test.go` — at least 3 matches (three valid resolutions)
4. `grep -A2 "## Unreleased" CHANGELOG.md` — shows the fix: entry
</verification>
