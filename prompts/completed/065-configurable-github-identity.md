---
status: completed
spec: [013-configurable-github-identity]
summary: Added configurable GitHub identity via GH_TOKEN for bot account PR creation
container: dark-factory-065-configurable-github-identity
dark-factory-version: v0.14.5
created: "2026-03-04T19:42:19Z"
queued: "2026-03-04T19:42:19Z"
started: "2026-03-04T19:42:19Z"
completed: "2026-03-04T19:53:54Z"
---
<objective>
Add configurable GitHub identity via GH_TOKEN so dark-factory can create PRs under a bot account, enabling separate identities for dark-factory and pr-reviewer.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read ALL markdown files in ~/Documents/workspaces/coding/docs/ for Go patterns.
Precondition: pkg/config, pkg/git, pkg/factory exist and are tested.
Spec: specs/014-configurable-github-identity.md
</context>

<requirements>
1. Update `Config` in `pkg/config/config.go`:
   - Add `GitHubConfig` struct with `Token string \`yaml:"token"\``
   - Add `GitHub GitHubConfig \`yaml:"github"\`` field to `Config`
   - Add `resolveEnvVar(value string) string` — if value matches `${VAR_NAME}` pattern, return `os.Getenv("VAR_NAME")`, otherwise return as-is
   - Add `ResolvedGitHubToken() string` method on `Config` that calls resolveEnvVar
   - `Defaults()` leaves `GitHub` as zero value (empty token = default auth)

2. Add config file permission check in `pkg/config/config.go`:
   - After reading the config file, check file permissions with `os.Stat`
   - If file is world-readable (mode & 0004 != 0), log warning: `"config file is world-readable, consider: chmod 600 .dark-factory.yaml"`
   - Warning only — do not fail

3. Update `NewPRCreator` in `pkg/git/pr_creator.go`:
   - Change signature: `NewPRCreator(ghToken string) PRCreator`
   - Store token in `prCreator` struct
   - Before `cmd.Run()` in `Create`: if token non-empty, `cmd.Env = append(os.Environ(), "GH_TOKEN="+p.ghToken)`

4. Update `NewBrancher` in `pkg/git/brancher.go`:
   - Change signature: `NewBrancher(ghToken string) Brancher`
   - Only `DefaultBranch` uses `gh` — other methods use `git` and don't need the token
   - Before `cmd.Run()` in `DefaultBranch`: inject token if non-empty

5. Update `NewPRMerger` in `pkg/git/pr_merger.go` (from spec 013):
   - Change signature: `NewPRMerger(ghToken string) PRMerger`
   - Both `gh pr view` and `gh pr merge` calls get token injected

6. Update `pkg/factory/factory.go`:
   - Resolve token once: `ghToken := cfg.ResolvedGitHubToken()`
   - If `cfg.GitHub.Token != ""` but `ghToken == ""`: log warning "github.token configured but env var is empty, using default gh auth"
   - Pass `ghToken` to `git.NewBrancher`, `git.NewPRCreator`, `git.NewPRMerger`

7. Update docs:
   - `README.md` and `example/.dark-factory.yaml`: add `github.token: ${DARK_FACTORY_TOKEN}` example

8. Update tests in `pkg/config/config_test.go`:
   - Config without `github` section → `ResolvedGitHubToken()` returns empty
   - Config with `github.token: ${TEST_VAR}` + env var set → resolves correctly
   - Config with `github.token: ${TEST_VAR}` + env var unset → returns empty
   - Config with `github.token: literal-value` → returns literal
   - Existing configs without `github` field parse successfully (backward compat)

9. Update tests in `pkg/git/pr_creator_test.go`:
   - `NewPRCreator("")` → subprocess env not modified
   - `NewPRCreator("tok")` → subprocess env includes `GH_TOKEN=tok`

10. Update tests in `pkg/git/brancher_test.go`:
    - `DefaultBranch` with token → `GH_TOKEN` set in subprocess env

11. Regenerate counterfeiter mocks: `go generate ./...`
</requirements>

<constraints>
- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Backward compatible — existing configs without `github.token` must keep working
- Token must NEVER appear in log output or error messages
- Token must NEVER be passed as CLI argument (only via cmd.Env)
- Does NOT affect `git` commands (push, commit, etc.) — only `gh` commands
- Use Ginkgo v2 + Gomega for tests
- Use counterfeiter for mocks
- Follow existing patterns exactly
</constraints>

<verification>
Run `go generate ./...` -- must succeed.
Run `make test` -- must pass.
Run `make precommit` -- must pass.
</verification>

<success_criteria>
- github.token field parsed from config YAML
- ${VAR} env var resolution works
- gh CLI receives GH_TOKEN when configured (pr_creator, brancher, pr_merger)
- Warning logged for empty env var
- Warning logged for world-readable config file
- Existing configs without github section still work
- Token never appears in logs or error messages
- make precommit passes
</success_criteria>
