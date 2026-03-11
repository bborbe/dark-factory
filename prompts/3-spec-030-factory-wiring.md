---
status: created
spec: ["030"]
created: "2026-03-11T10:00:00Z"
branch: dark-factory/bitbucket-server-pr-workflow
---
<summary>
- `provider: github` (default) uses exactly the same `gh` CLI implementations as today — zero behavior change for existing users
- `provider: bitbucket-server` wires the new Bitbucket REST API implementations into the processor, review poller, and prompt verify command
- Provider selection is a single factory helper — all call sites (`CreateRunner`, `CreateOneShotRunner`, `CreateReviewPoller`, `CreatePromptVerifyCommand`) use the same logic
- For Bitbucket, project and repo are parsed from the git remote URL at startup — no manual configuration needed
- For Bitbucket, the default branch comes from `config.defaultBranch` (required field for Bitbucket, optional for GitHub)
- Default reviewers for new Bitbucket PRs are fetched from the default-reviewers plugin; if unavailable, PRs are created without reviewers
- All existing tests pass unchanged; new tests validate the factory selection logic
</summary>

<objective>
Wire the Bitbucket Server implementations from prompt 2 into the factory layer so that `provider: bitbucket-server` in `.dark-factory.yaml` activates the Bitbucket client for all PR operations (create, merge, review fetch, collaborator fetch). The factory selects the right implementation at startup; the processor and review poller are unchanged.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-factory-pattern.md` — `Create*` prefix, zero business logic in factories, constructor pattern.
Read `/home/node/.claude/docs/go-composition.md` — inject interfaces, never call package functions from business logic.
Read `/home/node/.claude/docs/go-testing.md` — Ginkgo/Gomega testing, external test packages.

**Preconditions from prompts 1 and 2 of this spec** (must already exist):
- `pkg/config/provider.go` — `Provider`, `ProviderGitHub`, `ProviderBitbucketServer`
- `pkg/config/config.go` — `Config.Provider`, `Config.Bitbucket`, `Config.ResolvedBitbucketToken()`
- `pkg/git/bitbucket_remote.go` — `BitbucketRemoteCoords`, `ParseBitbucketRemoteFromGit`
- `pkg/git/bitbucket_pr_creator.go` — `NewBitbucketPRCreator`
- `pkg/git/bitbucket_pr_merger.go` — `NewBitbucketPRMerger`
- `pkg/git/bitbucket_review_fetcher.go` — `NewBitbucketReviewFetcher`
- `pkg/git/bitbucket_collaborator_fetcher.go` — `NewBitbucketCollaboratorFetcher`

If any of these are missing, add them before proceeding — see prompts 1 and 2 for exact definitions.

Read these files before making any changes:
- `pkg/factory/factory.go` — `CreateProcessor`, `CreateRunner`, `CreateOneShotRunner`, `CreateReviewPoller`, `CreatePromptVerifyCommand`
- `pkg/config/config.go` — `Config` struct, `ResolvedGitHubToken()`, `ResolvedBitbucketToken()`, `DefaultBranch`
- `pkg/git/pr_creator.go` — `PRCreator` interface
- `pkg/git/pr_merger.go` — `PRMerger` interface
- `pkg/git/review_fetcher.go` — `ReviewFetcher` interface
- `pkg/git/collaborator_fetcher.go` — `CollaboratorFetcher` interface
- `pkg/git/brancher.go` — `Brancher` interface, `NewBrancher`, `WithDefaultBranch` option
- `pkg/factory/factory_suite_test.go` — test suite setup
</context>

<requirements>
**Step 1: Add a `createProviderDeps` helper to `pkg/factory/factory.go`**

Add a new helper function that returns the provider-specific implementations based on `cfg.Provider`. This function encapsulates all provider selection logic — it is the ONLY place in the factory where `cfg.Provider` is checked.

Add this function to `pkg/factory/factory.go`:

```go
// providerDeps holds the provider-specific git operation implementations.
type providerDeps struct {
    prCreator    git.PRCreator
    prMerger     git.PRMerger
    reviewFetcher git.ReviewFetcher
    collaboratorFetcher git.CollaboratorFetcher
    brancher     git.Brancher
}

// createProviderDeps returns the git provider implementations based on cfg.Provider.
// For github (or empty): uses gh CLI implementations (existing behavior).
// For bitbucket-server: uses Bitbucket Server REST API implementations.
func createProviderDeps(
    cfg config.Config,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) providerDeps {
    if cfg.Provider == config.ProviderBitbucketServer {
        return createBitbucketProviderDeps(cfg, currentDateTimeGetter)
    }
    return createGitHubProviderDeps(cfg, currentDateTimeGetter)
}

// createGitHubProviderDeps returns GitHub gh-CLI-backed implementations.
func createGitHubProviderDeps(
    cfg config.Config,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) providerDeps {
    ghToken := cfg.ResolvedGitHubToken()
    repoNameFetcher := git.NewGHRepoNameFetcher(ghToken)
    collaboratorLister := git.NewGHCollaboratorLister(ghToken)
    collaboratorFetcher := git.NewCollaboratorFetcher(
        repoNameFetcher,
        collaboratorLister,
        cfg.UseCollaborators,
        cfg.AllowedReviewers,
    )
    return providerDeps{
        prCreator:           git.NewPRCreator(ghToken),
        prMerger:            git.NewPRMerger(ghToken, currentDateTimeGetter),
        reviewFetcher:       git.NewReviewFetcher(ghToken),
        collaboratorFetcher: collaboratorFetcher,
        brancher:            git.NewBrancher(git.WithDefaultBranch(cfg.DefaultBranch)),
    }
}

// createBitbucketProviderDeps returns Bitbucket Server REST API-backed implementations.
// Parses project and repo from the current git remote URL.
// On error (e.g. unparseable remote URL), logs a warning and returns non-nil structs that
// will fail at operation time with a clear error — startup is not blocked.
func createBitbucketProviderDeps(
    cfg config.Config,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) providerDeps {
    ctx := context.Background()
    token := cfg.ResolvedBitbucketToken()
    baseURL := cfg.Bitbucket.BaseURL

    coords, err := git.ParseBitbucketRemoteFromGit(ctx, "origin")
    if err != nil {
        slog.Warn("bitbucket: failed to parse git remote URL; PR operations will fail", "error", err)
        coords = &git.BitbucketRemoteCoords{Project: "", Repo: ""}
    }

    // Fetch current user (for excluding from reviewers) — best-effort
    currentUser := fetchBitbucketCurrentUser(ctx, baseURL, token)

    // Build collaborator fetcher (default reviewers plugin) with current user excluded
    collaboratorFetcher := git.NewBitbucketCollaboratorFetcher(
        baseURL, token, coords.Project, coords.Repo, cfg.DefaultBranch, currentUser,
    )

    // Fetch reviewers now (same pattern as GitHub provider in CreateReviewPoller)
    reviewers := collaboratorFetcher.Fetch(ctx)
    if len(cfg.AllowedReviewers) > 0 {
        reviewers = cfg.AllowedReviewers // explicit config overrides plugin
    }

    return providerDeps{
        prCreator:    git.NewBitbucketPRCreator(
            baseURL, token, coords.Project, coords.Repo, cfg.DefaultBranch, reviewers,
        ),
        prMerger:     git.NewBitbucketPRMerger(baseURL, token, coords.Project, coords.Repo),
        reviewFetcher: git.NewBitbucketReviewFetcher(baseURL, token, coords.Project, coords.Repo),
        collaboratorFetcher: collaboratorFetcher,
        // Bitbucket uses defaultBranch from config — no gh CLI fallback
        brancher:     git.NewBrancher(git.WithDefaultBranch(cfg.DefaultBranch)),
    }
}
```

**Step 2: Add `fetchBitbucketCurrentUser` helper**

Add to `pkg/factory/factory.go`:

```go
// fetchBitbucketCurrentUser fetches the current Bitbucket Server username via the whoami endpoint.
// Returns empty string on error (graceful degradation — reviewer exclusion will not apply).
func fetchBitbucketCurrentUser(ctx context.Context, baseURL, token string) string {
    req, err := http.NewRequestWithContext(
        ctx, "GET",
        strings.TrimRight(baseURL, "/")+"/plugins/servlet/applinks/whoami",
        nil,
    )
    if err != nil {
        slog.Warn("bitbucket: failed to create whoami request", "error", err)
        return ""
    }
    req.Header.Set("Authorization", "Bearer "+token)
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        slog.Warn("bitbucket: whoami request failed", "error", err)
        return ""
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        slog.Warn("bitbucket: whoami returned non-200", "status", resp.StatusCode)
        return ""
    }
    body, _ := io.ReadAll(resp.Body)
    return strings.TrimSpace(string(body))
}
```

Add these imports to `pkg/factory/factory.go`:
```go
"context"
"io"
"log/slog"
"net/http"
"strings"
```

**Step 3: Refactor `CreateProcessor` to accept provider interfaces**

Currently `CreateProcessor` takes `ghToken string` and internally creates `git.NewPRCreator`, `git.NewPRMerger`, `git.NewBrancher`. Change it to accept these as parameters so the factory can inject the right provider.

Update `CreateProcessor` signature in `pkg/factory/factory.go`:

```go
// CreateProcessor creates a Processor that executes queued prompts.
func CreateProcessor(
    inProgressDir string,
    completedDir string,
    logDir string,
    projectName string,
    promptManager prompt.Manager,
    releaser git.Releaser,
    versionGetter version.Getter,
    ready <-chan struct{},
    containerImage string,
    model string,
    netrcFile string,
    gitconfigFile string,
    pr bool,
    worktree bool,
    brancher git.Brancher,         // NEW: replaces ghToken for branch operations
    prCreator git.PRCreator,        // NEW: replaces ghToken for PR creation
    prMerger git.PRMerger,          // NEW: replaces ghToken for PR merge
    autoMerge bool,
    autoRelease bool,
    autoReview bool,
    validationCommand string,
    specsInboxDir string,
    specsInProgressDir string,
    specsCompletedDir string,
    verificationGate bool,
    env map[string]string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) processor.Processor {
    return processor.NewProcessor(
        inProgressDir,
        completedDir,
        logDir,
        projectName,
        executor.NewDockerExecutor(
            containerImage,
            projectName,
            model,
            netrcFile,
            gitconfigFile,
            env,
        ),
        promptManager,
        releaser,
        versionGetter,
        ready,
        pr,
        worktree,
        brancher,
        prCreator,
        git.NewCloner(),
        prMerger,
        autoMerge,
        autoRelease,
        autoReview,
        spec.NewAutoCompleter(
            inProgressDir,
            completedDir,
            specsInboxDir,
            specsInProgressDir,
            specsCompletedDir,
            currentDateTimeGetter,
        ),
        spec.NewLister(specsInboxDir, specsInProgressDir, specsCompletedDir),
        validationCommand,
        verificationGate,
    )
}
```

Note: `defaultBranch` parameter is REMOVED from `CreateProcessor` — it's now handled inside `createProviderDeps` via the injected `brancher`. Remove the corresponding argument from all callers.

**Step 4: Update `CreateRunner` to use `createProviderDeps`**

In `CreateRunner`, replace the `ghToken` variable and the provider-specific constructor calls with `createProviderDeps`:

```go
func CreateRunner(cfg config.Config, ver string) runner.Runner {
    inboxDir := cfg.Prompts.InboxDir
    inProgressDir := cfg.Prompts.InProgressDir
    completedDir := cfg.Prompts.CompletedDir
    currentDateTimeGetter := libtime.NewCurrentDateTime()
    promptManager, releaser := createPromptManager(
        inboxDir, inProgressDir, completedDir, currentDateTimeGetter,
    )
    versionGetter := version.NewGetter(ver)
    projectName := project.Name(cfg.ProjectName)
    ready := make(chan struct{}, 10)
    specGen := CreateSpecGenerator(cfg, cfg.ContainerImage, currentDateTimeGetter)
    deps := createProviderDeps(cfg, currentDateTimeGetter)

    return runner.NewRunner(
        inboxDir, inProgressDir, completedDir, cfg.Prompts.LogDir,
        cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir, cfg.Specs.LogDir,
        promptManager,
        CreateLocker("."),
        CreateWatcher(inProgressDir, inboxDir, promptManager, ready,
            time.Duration(cfg.DebounceMs)*time.Millisecond, currentDateTimeGetter),
        CreateProcessor(
            inProgressDir, completedDir, cfg.Prompts.LogDir, projectName,
            promptManager, releaser, versionGetter, ready,
            cfg.ContainerImage, cfg.Model, cfg.NetrcFile, cfg.GitconfigFile,
            cfg.PR, cfg.Worktree,
            deps.brancher, deps.prCreator, deps.prMerger,
            cfg.AutoMerge, cfg.AutoRelease, cfg.AutoReview,
            cfg.ValidationCommand,
            cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir,
            cfg.VerificationGate, cfg.Env, currentDateTimeGetter,
        ),
        createOptionalServer(cfg, inboxDir, inProgressDir, completedDir, promptManager),
        createOptionalReviewPoller(cfg, promptManager),
        CreateSpecWatcher(cfg, specGen, currentDateTimeGetter),
    )
}
```

**Step 5: Update `CreateOneShotRunner`** using the same pattern as step 4 — replace `ghToken` with `deps := createProviderDeps(cfg, currentDateTimeGetter)` and pass `deps.brancher`, `deps.prCreator`, `deps.prMerger` to `CreateProcessor`.

**Step 6: Update `CreateReviewPoller`**

Replace the existing GitHub-specific implementation with provider-aware logic:

```go
// CreateReviewPoller creates a ReviewPoller that watches in_review prompts.
func CreateReviewPoller(cfg config.Config, promptManager prompt.Manager) review.ReviewPoller {
    currentDateTimeGetter := libtime.NewCurrentDateTime()
    deps := createProviderDeps(cfg, currentDateTimeGetter)
    allowedReviewers := deps.collaboratorFetcher.Fetch(context.Background())

    return review.NewReviewPoller(
        cfg.Prompts.InProgressDir,
        cfg.Prompts.InboxDir,
        allowedReviewers,
        cfg.MaxReviewRetries,
        time.Duration(cfg.PollIntervalSec)*time.Second,
        deps.reviewFetcher,
        git.NewPRMerger(cfg.ResolvedGitHubToken(), currentDateTimeGetter), // merger reused from GitHub for review poller
        promptManager,
        review.NewFixPromptGenerator(),
    )
}
```

Wait — this is wrong. For Bitbucket, the review poller should also use `deps.prMerger`. Update to:

```go
func CreateReviewPoller(cfg config.Config, promptManager prompt.Manager) review.ReviewPoller {
    currentDateTimeGetter := libtime.NewCurrentDateTime()
    deps := createProviderDeps(cfg, currentDateTimeGetter)
    allowedReviewers := deps.collaboratorFetcher.Fetch(context.Background())

    return review.NewReviewPoller(
        cfg.Prompts.InProgressDir,
        cfg.Prompts.InboxDir,
        allowedReviewers,
        cfg.MaxReviewRetries,
        time.Duration(cfg.PollIntervalSec)*time.Second,
        deps.reviewFetcher,
        deps.prMerger,
        promptManager,
        review.NewFixPromptGenerator(),
    )
}
```

**Step 7: Update `CreatePromptVerifyCommand`**

Replace the `ghToken` variable with provider-based deps:

```go
func CreatePromptVerifyCommand(cfg config.Config) cmd.PromptVerifyCommand {
    currentDateTimeGetter := libtime.NewCurrentDateTime()
    promptManager, releaser := createPromptManager(
        cfg.Prompts.InboxDir, cfg.Prompts.InProgressDir, cfg.Prompts.CompletedDir,
        currentDateTimeGetter,
    )
    deps := createProviderDeps(cfg, currentDateTimeGetter)
    return cmd.NewPromptVerifyCommand(
        cfg.Prompts.InProgressDir,
        cfg.Prompts.CompletedDir,
        promptManager,
        releaser,
        cfg.PR,
        deps.brancher,
        deps.prCreator,
        currentDateTimeGetter,
    )
}
```

**Step 8: Remove `createOptionalReviewPoller` duplication**

The `createOptionalReviewPoller` already calls `CreateReviewPoller` — no changes needed there. Verify it still compiles with the updated `CreateReviewPoller`.

**Step 9: Tests for factory wiring**

Add tests to `pkg/factory/factory_test.go` (create if it doesn't exist):

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    libtime "github.com/bborbe/time"

    "github.com/bborbe/dark-factory/pkg/config"
    "github.com/bborbe/dark-factory/pkg/factory"
)

var _ = Describe("CreateProcessor", func() {
    It("compiles with GitHub provider (default)", func() {
        cfg := config.Defaults()
        cfg.Provider = config.ProviderGitHub
        // Just verify no panic — processor construction is tested elsewhere
        Expect(func() {
            factory.CreateRunner(cfg, "v0.0.0")
        }).NotTo(Panic())
    })
})
```

Note: Since `createProviderDeps` is unexported and requires a real git repo for the Bitbucket path, keep factory tests minimal — focus on validating that the exported `CreateRunner` and `CreateOneShotRunner` accept GitHub provider without error. The Bitbucket provider is validated through config validation tests in prompt 1 and client tests in prompt 2.

If `pkg/factory/factory_test.go` already exists, add the new test case to the existing `Describe` block.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT change any interface definitions in `pkg/git/` — `PRCreator`, `PRMerger`, `ReviewFetcher`, `CollaboratorFetcher`, `Brancher` are unchanged
- `provider: github` (or omitted, defaulting to github): absolutely identical behavior to today — no regression
- The `defaultBranch` parameter is REMOVED from `CreateProcessor` because the brancher now carries it — ensure all callers are updated
- `createProviderDeps` is the ONLY function that checks `cfg.Provider` — no other factory function should branch on provider
- `fetchBitbucketCurrentUser` uses best-effort (returns `""` on error) — it must not block startup or cause startup failure
- `ParseBitbucketRemoteFromGit` failure in `createBitbucketProviderDeps` logs a warning and continues with empty project/repo — operation-time errors are clearer than startup panics
- Follow existing error wrapping: `errors.Wrap(ctx, err, "message")`
- All existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
```bash
# createProviderDeps exists and is the single provider selection point
grep -n "createProviderDeps\|createGitHubProviderDeps\|createBitbucketProviderDeps" pkg/factory/factory.go

# CreateProcessor no longer takes ghToken or defaultBranch — takes brancher/prCreator/prMerger
grep -n "func CreateProcessor" pkg/factory/factory.go

# CreateRunner uses deps.brancher, deps.prCreator, deps.prMerger
grep -n "deps\." pkg/factory/factory.go | head -20

# CreateReviewPoller uses deps.reviewFetcher and deps.prMerger
grep -n "deps\.reviewFetcher\|deps\.prMerger" pkg/factory/factory.go

# CreatePromptVerifyCommand uses deps.brancher and deps.prCreator
grep -n "deps\.brancher\|deps\.prCreator" pkg/factory/factory.go

make precommit
```
Must pass with no errors.
</verification>
