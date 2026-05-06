---
status: completed
spec: [073-simplify-merge-gate-by-relying-on-mergestatestatus]
summary: Removed autoReview, allowedReviewers, useCollaborators, maxReviewRetries, pollIntervalSec from Config and Defaults(); kept three user-visible fields as sentinels in partialConfig with friendly error detection in loadWithOverrides; updated factory.go, all three test files, and CHANGELOG.md.
container: dark-factory-380-spec-073-remove-config-fields
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-06T00:00:00Z"
queued: "2026-05-06T07:02:57Z"
started: "2026-05-06T07:26:49Z"
completed: "2026-05-06T07:35:33Z"
branch: dark-factory/simplify-merge-gate-by-relying-on-mergestatestatus
---

<summary>
- `AutoReview`, `AllowedReviewers`, `UseCollaborators`, `MaxReviewRetries`, and `PollIntervalSec` fields are removed from `config.Config` and `Defaults()`
- `validateAutoReview` function is removed from `config.go`; its `validation.Name(...)` entry in `Validate()` is removed
- The `mergePartialReview` function is deleted from `loader.go`; its call in `mergePartial` is removed
- `MaxReviewRetries` and `PollIntervalSec` are removed from `partialConfig` (silently become unknown YAML keys)
- `AutoReview`, `AllowedReviewers`, and `UseCollaborators` remain in `partialConfig` as sentinel-only fields; `loadWithOverrides` returns a per-field friendly error if any of these keys is present in `.dark-factory.yaml`
- `createBitbucketProviderDeps` in `factory.go` passes `nil` instead of `cfg.AllowedReviewers` to `NewBitbucketCollaboratorFetcher` (Bitbucket fetches reviewers from the API when the slice is nil)
- All five `validateAutoReview`-related It blocks are deleted from `config_test.go`; the default-values assertions for the removed fields are removed
- The full-YAML and `assertFullConfig` sections in `config_loader_test.go` drop the five removed fields; new It blocks assert that the three user-visible removed fields (`autoReview`, `allowedReviewers`, `useCollaborators`) each produce a clear, actionable error at load time
- `roundtrip_test.go` removes `maxReviewRetries`, `pollIntervalSec`, and `useCollaborators` entries from the DescribeTable, the `autoReview`-paired-yaml It block, and the now-stale exclusion map entries
- `CHANGELOG.md` gets an `## Unreleased` entry with a BREAKING-prefixed bullet
- `make precommit` exits 0
</summary>

<objective>
Remove the three user-visible config fields (`autoReview`, `allowedReviewers`, `useCollaborators`) and two internal-only fields (`maxReviewRetries`, `pollIntervalSec`) from `Config`, and make the loader surface a per-field friendly error when any of the three user-visible removed fields appears in `.dark-factory.yaml`. Prompt 1 (spec-073-remove-review-code) has already removed all code that reads these fields at runtime; this prompt removes the fields themselves and adds the migration guard.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read in full before editing:
- `pkg/config/config.go` — `Config` struct (~line 82), `Defaults()` (~line 123), `Validate()` (~line 200), `validateAutoReview` (~line 461)
- `pkg/config/loader.go` — `partialConfig` struct (~line 82), `loadWithOverrides` (~line 143), `mergePartial` (~line 247), `mergePartialReview` (~line 340)
- `pkg/factory/factory.go` — `createBitbucketProviderDeps` (~line 235); only the `NewBitbucketCollaboratorFetcher` call needs changing
- `pkg/config/config_test.go` — defaults assertions (~lines 43–50) and `validateAutoReview` test cases (~lines 710–825)
- `pkg/config/config_loader_test.go` — the full-YAML string and `assertFullConfig` function (~lines 760–840)
- `pkg/config/roundtrip_test.go` — `exclusions` map (~lines 32–43), `DescribeTable` entries (~lines 155–185), paired-yaml section (~lines 215–220)
</context>

<requirements>

## 1. Remove fields from `pkg/config/config.go`

### 1a. Remove five fields from the `Config` struct

In the `Config` struct, remove:
```go
AutoReview             bool                `yaml:"autoReview"`
MaxReviewRetries       int                 `yaml:"maxReviewRetries"`
AllowedReviewers       []string            `yaml:"allowedReviewers,omitempty"`
UseCollaborators       bool                `yaml:"useCollaborators"`
PollIntervalSec        int                 `yaml:"pollIntervalSec"`
```

Locate the exact lines by running:
```bash
grep -n "AutoReview\|MaxReviewRetries\|AllowedReviewers\|UseCollaborators\|PollIntervalSec" pkg/config/config.go
```

### 1b. Remove from `Defaults()`

In the `Defaults()` function (~line 155), the actual order is:
```go
AutoReview:        false,
MaxReviewRetries:  3,
PollIntervalSec:   60,
UseCollaborators:  false,
```

Remove all four lines. (`AllowedReviewers` has no default entry — it is a nil slice by default, which is already the zero value.)

### 1c. Remove `validateAutoReview` from `Validate()`

In the `Validate()` function (~line 213), remove:
```go
validation.Name("autoReview", validation.HasValidationFunc(c.validateAutoReview)),
```

### 1d. Delete the `validateAutoReview` function

Delete the entire function (lines ~461–475):
```go
// validateAutoReview validates the autoReview configuration.
func (c Config) validateAutoReview(ctx context.Context) error {
    ...
}
```

After deletion, verify no references remain:
```bash
grep -n "validateAutoReview\|AutoReview\|AllowedReviewers\|UseCollaborators\|MaxReviewRetries\|PollIntervalSec" pkg/config/config.go
```
Must return zero matches.

## 2. Update `pkg/config/loader.go`

### 2a. Remove `MaxReviewRetries` and `PollIntervalSec` from `partialConfig`

In `partialConfig`, remove:
```go
MaxReviewRetries       *int                  `yaml:"maxReviewRetries"`
PollIntervalSec        *int                  `yaml:"pollIntervalSec"`
```

These fields become silently-ignored YAML keys (YAML unmarshals them into `yaml.Node` and discards since there is no matching field).

### 2b. Keep `AutoReview`, `AllowedReviewers`, `UseCollaborators` in `partialConfig` as sentinel-only fields

Do NOT remove these three from `partialConfig`. Their continued presence allows `yaml.Unmarshal` to detect when a legacy config sets them. They will never be merged to `Config` (their merge function is deleted in 2c). Mark them with a comment to prevent future confusion:

```go
// Removed fields kept as sentinels to detect legacy configs.
// loadWithOverrides returns a friendly error if any of these is set.
AutoReview        *bool    `yaml:"autoReview"`
AllowedReviewers  []string `yaml:"allowedReviewers,omitempty"`
UseCollaborators  *bool    `yaml:"useCollaborators"`
```

### 2c. Delete `mergePartialReview` and remove its call

Delete the entire `mergePartialReview` function:
```go
// mergePartialReview merges auto-review and reviewer settings.
func mergePartialReview(cfg *Config, partial *partialConfig) {
    ...
}
```

In `mergePartial`, remove the call:
```go
mergePartialReview(cfg, partial)
```

`mergePartial` should now call only: `mergePartialWorkflow`, `mergePartialContainer`, `mergePartialProviders`, `mergePartialLimits`.

### 2d. Add friendly error detection in `loadWithOverrides`

In `loadWithOverrides`, after the `yaml.Unmarshal(data, &partial)` call (which succeeds) and before `mergePartial(&cfg, &partial)`, add detection for the three sentinel fields. Return a per-field error for each one that is present:

```go
// Detect removed config fields and return actionable errors.
if partial.AutoReview != nil {
    return LoadResult{}, errors.Errorf(ctx,
        "unknown field %q — autoReview was removed in v0.151.0; "+
            "configure GitHub branch protection on the repo to enforce review requirements",
        "autoReview",
    )
}
if len(partial.AllowedReviewers) > 0 {
    return LoadResult{}, errors.Errorf(ctx,
        "unknown field %q — allowedReviewers was removed in v0.151.0; "+
            "configure GitHub branch protection on the repo to enforce review requirements",
        "allowedReviewers",
    )
}
if partial.UseCollaborators != nil {
    return LoadResult{}, errors.Errorf(ctx,
        "unknown field %q — useCollaborators was removed in v0.151.0; "+
            "configure GitHub branch protection on the repo to enforce review requirements",
        "useCollaborators",
    )
}
```

Place this block AFTER `yaml.Unmarshal` succeeds and BEFORE the `mergePartial` call. The function returns on the FIRST offending field it encounters (no batching needed — one error is enough to block startup and prompt the user to remove the field).

**FREEZE all other logic in `loadWithOverrides`.** Do not touch the `workflow: pr` mapping, the worktree legacy mapping, or the override capture.

## 3. Update `pkg/factory/factory.go`

**Precondition check:** prompt 1 (`1-spec-073-remove-review-code`) already removed the GitHub-side `collaboratorFetcher` construction (which referenced `cfg.UseCollaborators` and `cfg.AllowedReviewers`). Verify before proceeding:

```bash
grep -n "cfg.AllowedReviewers\|cfg.UseCollaborators" pkg/factory/factory.go
# Expected: ONE match — only the Bitbucket call site at ~line 258 remains
```

If the GitHub references still exist, the prompt 1 work was incomplete — fix that first by deleting the GitHub `collaboratorFetcher` construction block in `createGitHubProviderDeps` (the function should only return prCreator, prMerger, brancher).

Then, in `createBitbucketProviderDeps`, change the `NewBitbucketCollaboratorFetcher` call to pass `nil` instead of `cfg.AllowedReviewers`:

```go
// BEFORE:
collaboratorFetcher := git.NewBitbucketCollaboratorFetcher(
    baseURL,
    token,
    coords.Project,
    coords.Repo,
    cfg.DefaultBranch,
    userFetcher,
    cfg.AllowedReviewers,
)

// AFTER:
collaboratorFetcher := git.NewBitbucketCollaboratorFetcher(
    baseURL,
    token,
    coords.Project,
    coords.Repo,
    cfg.DefaultBranch,
    userFetcher,
    nil,
)
```

`NewBitbucketCollaboratorFetcher` falls through to the Bitbucket API when `allowedReviewers` is nil — this is the correct behavior after removing the config field.

**FREEZE all other code in `factory.go`.** No other changes are needed there.

## 4. Update `pkg/config/config_test.go`

### 4a. Remove default-value assertions for removed fields

In the `"returns defaults"` It block (approximately lines 43–58), remove:
```go
Expect(cfg.AutoReview).To(BeFalse())
Expect(cfg.MaxReviewRetries).To(Equal(3))
Expect(cfg.PollIntervalSec).To(Equal(60))
Expect(cfg.UseCollaborators).To(BeFalse())
```

(`AllowedReviewers` has no default assertion — it was implicitly nil.)

### 4b. Delete five `validateAutoReview` It blocks

Delete all five It blocks related to `validateAutoReview` (approximately lines 710–825):
- `"fails for autoReview true with workflow direct"`
- `"fails for autoReview true with pr: false (no workflow field)"`
- `"fails for autoReview true with autoMerge false"`
- `"fails for autoReview true with no reviewer source"`
- `"succeeds for autoReview true with all required fields"`

Locate the exact spans by running:
```bash
grep -n "autoReview\|AutoReview\|AllowedReviewers" pkg/config/config_test.go
```

### 4c. Friendly-error It blocks

The friendly-error tests for removed config fields are added in `config_loader_test.go` per requirement 5c (where the Loader machinery lives). No additions in `config_test.go`.

## 5. Update `pkg/config/config_loader_test.go`

### 5a. Remove removed fields from the full YAML string

In `fullYAML()` (approximately lines 760–800), remove the five lines:
```yaml
autoReview: true
maxReviewRetries: 5
allowedReviewers:
  - test-reviewer
useCollaborators: true
pollIntervalSec: 30
```

### 5b. Remove removed fields from `assertFullConfig`

In `assertFullConfig`, remove:
```go
Expect(cfg.AutoReview).To(BeTrue())
Expect(cfg.MaxReviewRetries).To(Equal(5))
Expect(cfg.AllowedReviewers).To(Equal([]string{"test-reviewer"}))
Expect(cfg.UseCollaborators).To(BeTrue())
Expect(cfg.PollIntervalSec).To(Equal(30))
```

### 5c. Add three It blocks for friendly errors

**Important: `config.NewLoaderWithPath(path)` does NOT exist. Only `config.NewLoader()` exists, and it reads `.dark-factory.yaml` from the current working directory.** Mirror the existing `os.Chdir`-into-tmpDir + `config.NewLoader()` pattern already used in `config_loader_test.go`'s `BeforeEach`/`AfterEach`.

Find the existing pattern:
```bash
grep -B2 -A6 "config.NewLoader()" pkg/config/config_loader_test.go | head -30
```

Add these three It blocks adjacent to existing loader tests in the same `Describe` block. The surrounding `BeforeEach` already chdirs into a tmpDir, so `os.WriteFile(".dark-factory.yaml", ...)` writes to the right place:

```go
It("returns a friendly error when autoReview is set in YAML", func() {
    Expect(os.WriteFile(".dark-factory.yaml", []byte("autoReview: true\n"), 0600)).To(Succeed())
    _, err := loader.Load(ctx)
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("autoReview"))
    Expect(err.Error()).To(ContainSubstring("branch protection"))
})

It("returns a friendly error when allowedReviewers is set in YAML", func() {
    Expect(os.WriteFile(".dark-factory.yaml", []byte("allowedReviewers:\n  - alice\n"), 0600)).To(Succeed())
    _, err := loader.Load(ctx)
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("allowedReviewers"))
    Expect(err.Error()).To(ContainSubstring("branch protection"))
})

It("returns a friendly error when useCollaborators is set in YAML", func() {
    Expect(os.WriteFile(".dark-factory.yaml", []byte("useCollaborators: true\n"), 0600)).To(Succeed())
    _, err := loader.Load(ctx)
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("useCollaborators"))
    Expect(err.Error()).To(ContainSubstring("branch protection"))
})
```

`loader` is the `config.NewLoader()` instance from the existing `BeforeEach`.

## 6. Update `pkg/config/roundtrip_test.go`

### 6a. Remove stale exclusion map entries

In the `exclusions` map, remove:
```go
"AllowedReviewers": "slice collection, merged as-is without pointer indirection",
"AutoReview": "validation-coupled: requires pr+autoMerge+allowedReviewers; covered by paired-yaml test",
```

Both fields no longer exist on `Config`, so these exclusion entries are dead code.

### 6b. Remove three DescribeTable entries

In the `DescribeTable` (approximately lines 155–185), remove:
```go
Entry("maxReviewRetries", "maxReviewRetries", "9",
    func(cfg Config) { Expect(cfg.MaxReviewRetries).To(Equal(9)) }),
Entry("pollIntervalSec", "pollIntervalSec", "99",
    func(cfg Config) { Expect(cfg.PollIntervalSec).To(Equal(99)) }),
Entry("useCollaborators", "useCollaborators", "true",
    func(cfg Config) { Expect(cfg.UseCollaborators).To(BeTrue()) }),
```

### 6c. Remove the `autoReview` paired-yaml It block

In the paired-yaml section (~line 215), delete the It block:
```go
It("autoReview: true round-trips when all required fields are present", func() {
    yaml := "workflow: clone\npr: true\nautoMerge: true\nautoReview: true\nuseCollaborators: true\n"
    cfg, err := writeAndLoad(yaml)
    Expect(err).NotTo(HaveOccurred())
    Expect(cfg.AutoReview).To(BeTrue())
})
```

Replace it with an It block that verifies the friendly error is returned instead:

```go
It("autoReview: true in YAML returns a friendly error", func() {
    _, err := writeAndLoad("workflow: clone\npr: true\nautoMerge: true\nautoReview: true\n")
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("autoReview"))
    Expect(err.Error()).To(ContainSubstring("branch protection"))
})
```

## 7. Write `CHANGELOG.md` entry

Add an `## Unreleased` section immediately after the `# Changelog` header line (before `## v0.150.2`):

```markdown
## Unreleased

- BREAKING: removed `autoReview`, `allowedReviewers`, `useCollaborators`, `maxReviewRetries`, `pollIntervalSec` config fields. Use GitHub branch protection to gate merges. Configs containing any of the three user-visible removed fields (`autoReview`, `allowedReviewers`, `useCollaborators`) now fail at load time with a friendly error.
```

## 8. Verify compilation and tests

After all changes:

```bash
cd /workspace && make test
```

Then:
```bash
cd /workspace && make precommit
```

Both must exit 0.

Spot checks:
```bash
# No removed fields remain in config.go
grep -n "AutoReview\|AllowedReviewers\|UseCollaborators\|MaxReviewRetries\|PollIntervalSec" pkg/config/config.go
# Expected: zero matches

# Sentinel fields remain in partialConfig
grep -n "AutoReview\|AllowedReviewers\|UseCollaborators" pkg/config/loader.go
# Expected: 3 matches (the sentinel fields) + error detection blocks

# Merge function is gone
grep -n "mergePartialReview" pkg/config/loader.go
# Expected: zero matches

# Bitbucket collaborator fetcher gets nil
grep -n "AllowedReviewers" pkg/factory/factory.go
# Expected: zero matches
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT remove `AutoReview`, `AllowedReviewers`, `UseCollaborators` from `partialConfig` — they must remain as sentinel fields so `yaml.Unmarshal` populates them when the legacy key is present in YAML
- Do NOT remove `InReviewPromptStatus` from `pkg/prompt/prompt.go` — existing prompts may have `in_review` status; the status remains valid for backward compatibility
- Do NOT touch the Bitbucket PR-creator code or `NewBitbucketCollaboratorFetcher` implementation — only update the call site in factory.go to pass `nil`
- Wrap all new errors with `errors.Errorf` from `github.com/bborbe/errors` (no `fmt.Errorf`)
- Do not touch `go.mod` / `go.sum` / `vendor/`
- The `CHANGELOG.md` `## Unreleased` section must appear BEFORE `## v0.150.2` — do not replace existing version sections
- The friendly error blocks in `loadWithOverrides` must come AFTER `yaml.Unmarshal` and BEFORE `mergePartial`
- The version string in the error messages must be `v0.151.0` — this is the expected next minor version after the current `v0.150.2`
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Manual verification of the negative-control path:
```bash
cd /workspace
echo "autoReview: true" > /tmp/test-legacy.yaml
# Load it via Go test or small go run that calls config.NewLoaderWithPath("/tmp/test-legacy.yaml").Load(ctx)
# Expected error contains: "autoReview" and "branch protection"
```

Additional checks:
1. `grep -rn "AutoReview\|AllowedReviewers\|UseCollaborators\|MaxReviewRetries\|PollIntervalSec" pkg/config/config.go` — zero matches
2. `grep -n "mergePartialReview" pkg/config/loader.go` — zero matches
3. `grep -n "AllowedReviewers" pkg/factory/factory.go` — zero matches (the nil change in createBitbucketProviderDeps)
4. `grep -n "## Unreleased" CHANGELOG.md` — one match
5. `grep -rn "validateAutoReview" pkg/config/` — zero matches
6. `grep -c "autoReview\|allowedReviewers\|useCollaborators" pkg/config/config_loader_test.go` — should be low (only in the new friendly-error It blocks, not in fullYAML/assertFullConfig)
</verification>
