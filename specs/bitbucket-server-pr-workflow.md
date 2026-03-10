---
status: draft
---

## Summary

- `workflow: pr` works with Bitbucket Server (and any git host) — not just GitHub
- Replaces all `gh` CLI calls with a pluggable git provider interface
- GitHub remains the default; Bitbucket Server added as second provider
- Provider selected via `provider` field in `.dark-factory.yaml`
- `workflow: direct` already works (fixed in v0.33.1 via `defaultBranch` config)

## Problem

`workflow: pr` is fully GitHub-dependent — 6+ operations use `gh` CLI: create PR, view PR, merge PR, fetch reviews, fetch collaborators, get repo name. Running dark-factory with `workflow: pr` on Bitbucket Server (or GitLab, Gitea, etc.) fails immediately.

The Octopus team uses Bitbucket Server. Currently limited to `workflow: direct` which means no PR review loop, no auto-merge, no review polling.

## Goal

After this work, `workflow: pr` creates PRs on Bitbucket Server via its REST API. The PR lifecycle (create → review → merge) works the same as GitHub. Provider is configured in `.dark-factory.yaml`.

## Non-goals

- No GitLab support (add later using same interface)
- No Bitbucket Cloud support (different API from Bitbucket Server)
- No changes to `workflow: direct`
- No changes to Docker executor or prompt processing
- No migration of existing GitHub-specific config — GitHub remains default

## Desired Behavior

1. New config field `provider: bitbucket-server` (default: `github`) in `.dark-factory.yaml`
2. New config section `bitbucket:` with `baseURL` and `token` (env var reference like GitHub)
3. `PRCreator` creates Bitbucket Server PRs via REST API (`POST /rest/api/1.0/projects/{project}/repos/{repo}/pull-requests`)
4. `PRMerger` merges PRs via REST API (`POST .../pull-requests/{id}/merge`)
5. `ReviewFetcher` fetches PR reviews/approvals via REST API
6. `CollaboratorFetcher` returns configured reviewers (Bitbucket Server has no direct collaborator API equivalent — use config list)
7. `RepoNameFetcher` extracts project/repo from git remote URL (no API call needed)
8. All existing GitHub behavior unchanged when `provider: github` (default)

## Constraints

- Do NOT change the `Brancher`, `PRCreator`, `PRMerger`, `ReviewFetcher` interfaces — implement new types behind them
- Do NOT add new dependencies for HTTP client — use stdlib `net/http`
- Do NOT remove `gh` CLI support — it remains the GitHub implementation
- Existing tests must pass unchanged
- Bitbucket Server API v1.0 (REST) — no plugins or extensions required
- Token auth only (no OAuth flow for Bitbucket Server)

## Failure Modes

| Trigger | Behavior | Recovery |
|---------|----------|----------|
| Invalid `provider` value | Config validation error at startup | Fix config |
| Bitbucket Server unreachable | PR creation fails, prompt marked failed | Retry after fixing network |
| Token expired/invalid | 401 from API, prompt marked failed | Refresh token, retry |
| PR merge conflict | Merge fails, prompt marked failed | Manual resolution |
| Missing `bitbucket.baseURL` when provider=bitbucket-server | Config validation error | Add required field |

## Acceptance Criteria

- [ ] `provider: github` (or omitted) works identically to current behavior
- [ ] `provider: bitbucket-server` with `bitbucket: { baseURL, token }` passes config validation
- [ ] `provider: invalid` fails config validation with clear error
- [ ] PR created on Bitbucket Server via REST API in pr workflow
- [ ] PR merged on Bitbucket Server via REST API when autoMerge enabled
- [ ] Review status fetched from Bitbucket Server when autoReview enabled
- [ ] `defaultBranch` config used (no `gh repo view` fallback attempted)
- [ ] All existing tests pass
- [ ] New tests cover Bitbucket provider creation, config validation, and API error paths

## Verification

```
make precommit
```

Integration test with real Bitbucket Server instance (manual, not automated):
```
# .dark-factory.yaml
workflow: pr
provider: bitbucket-server
defaultBranch: master
bitbucket:
  baseURL: https://bitbucket.seibert.cloud
  token: $BITBUCKET_TOKEN
```

## Do-Nothing Option

Stay on `workflow: direct` for Bitbucket projects. Acceptable short-term — no PR review loop, but code still gets committed and pushed. The cost is losing the review-fix loop and auto-merge capabilities for Octopus projects.
