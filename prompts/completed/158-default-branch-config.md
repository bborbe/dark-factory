---
status: completed
summary: Added defaultBranch config field to Config struct, implemented functional options pattern on NewBrancher with WithDefaultBranch option, updated CreateProcessor signature with defaultBranch param, fixed pre-existing race condition in verification gate test, added new tests for all code paths, updated CHANGELOG.md
container: dark-factory-158-default-branch-config
dark-factory-version: v0.32.1
created: "2026-03-10T10:36:29Z"
queued: "2026-03-10T10:36:29Z"
started: "2026-03-10T10:36:41Z"
completed: "2026-03-10T10:55:20Z"
---

<summary>
- Projects can configure a default branch name in .dark-factory.yaml, removing the hard dependency on GitHub's gh CLI
- When defaultBranch is set in config, the brancher uses that value directly instead of calling gh repo view
- When defaultBranch is not set, the existing gh CLI fallback is used (backwards compatible)
- Enables dark-factory to work with Bitbucket, GitLab, and other non-GitHub hosting providers
- Works out of the box for any Git hosting provider without additional tooling
</summary>

<objective>
Add a `defaultBranch` config field to `.dark-factory.yaml` so dark-factory works with non-GitHub repos (Bitbucket, GitLab, etc.). Currently `DefaultBranch()` always calls `gh repo view` which fails for non-GitHub repos. When configured, the brancher should return the configured value directly without calling `gh`.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/config/config.go` — Config struct and defaults.
Read `pkg/git/brancher.go` — Brancher interface and implementation. `DefaultBranch()` uses `gh repo view`. `MergeOriginDefault()` calls `DefaultBranch()`.
Read `pkg/factory/factory.go` — Where `NewBrancher()` is called (lines ~248 and ~398).
Read `pkg/git/brancher_test.go` — Existing tests.
Read `pkg/processor/processor.go` — Where `MergeOriginDefault` and `DefaultBranch` are called.
</context>

<implementation>
Functional options pattern to follow:

```go
type BrancherOption func(*brancher)

func WithDefaultBranch(branch string) BrancherOption {
	return func(b *brancher) {
		if branch != "" {
			b.configuredDefaultBranch = branch
		}
	}
}

func NewBrancher(opts ...BrancherOption) Brancher {
	b := &brancher{}
	for _, opt := range opts {
		opt(b)
	}
	return b
}
```
</implementation>

<requirements>
1. Add `DefaultBranch string `yaml:"defaultBranch"`` field to `Config` struct in `pkg/config/config.go`. No default value (empty string means "use gh CLI fallback").

2. Update `NewBrancher()` in `pkg/git/brancher.go` to accept an optional default branch override:
   - Change signature to `NewBrancher(opts ...BrancherOption) Brancher`
   - Add `BrancherOption` functional option type
   - Add `WithDefaultBranch(branch string) BrancherOption` that sets a configured default branch on the brancher struct
   - Add a `configuredDefaultBranch string` field to the `brancher` struct

3. Update `DefaultBranch()` method in `pkg/git/brancher.go`:
   - If `b.configuredDefaultBranch != ""`, return it directly (no gh call)
   - Otherwise, fall through to existing `gh repo view` logic (unchanged)

4. Update both `NewBrancher()` call sites in `pkg/factory/factory.go`:
   - Pass `git.WithDefaultBranch(cfg.DefaultBranch)` unconditionally at both call sites
   - `WithDefaultBranch("")` must be a no-op (empty string = use gh CLI fallback)

5. Update `mocks/brancher.go` by running `go generate -mod=vendor ./pkg/git/...` (counterfeiter)

6. Add tests in `pkg/git/brancher_test.go`:
   - Test: `NewBrancher(WithDefaultBranch("main"))` → `DefaultBranch()` returns `"main"` without calling gh
   - Test: `NewBrancher()` without option → `DefaultBranch()` attempts gh CLI (existing behavior)
   - Test: `MergeOriginDefault()` with configured default branch uses the configured value

7. Add test in `pkg/config/config_test.go`:
   - Test: Config with `DefaultBranch: "master"` validates successfully
   - Test: Config without `DefaultBranch` validates successfully (optional field)
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `NewBrancher()` with no arguments must behave identically to current behavior (backwards compatible)
- Do NOT remove or change the `gh repo view` fallback — it must remain as the default when no config is set
- Do NOT change the `Brancher` interface — only the constructor and implementation change
- Use functional options pattern (consistent with Go idioms)
- Vendor mode: all commands use `-mod=vendor`
</constraints>

<verification>
```
make precommit
```

Must pass with no errors.
</verification>

<success_criteria>
- `defaultBranch` field exists in Config struct
- `NewBrancher(WithDefaultBranch("master"))` returns configured branch without gh CLI
- `NewBrancher()` without option falls back to gh CLI (unchanged behavior)
- Both factory.go call sites pass the config value when set
- All existing tests pass
- New tests cover configured and unconfigured paths
- `make precommit` passes
</success_criteria>
