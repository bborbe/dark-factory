---
status: prompted
approved: "2026-03-11T09:44:40Z"
prompted: "2026-03-11T10:19:18Z"
branch: dark-factory/bitbucket-server-pr-workflow
---

## Summary

- Teams on Bitbucket Server can use `workflow: pr` with full PR creation, review polling, and auto-merge
- GitHub remains the default provider — no changes for existing users
- Provider selected via `provider` field in `.dark-factory.yaml`
- `workflow: direct` already works (fixed in v0.33.1 via `defaultBranch` config)

## Problem

`workflow: pr` is fully GitHub-dependent — 6+ operations use `gh` CLI: create PR, view PR, merge PR, fetch reviews, fetch collaborators, get repo name. Running dark-factory with `workflow: pr` on Bitbucket Server (or GitLab, Gitea, etc.) fails immediately.

Teams using Bitbucket Server are currently limited to `workflow: direct` which means no PR review loop, no auto-merge, no review polling.

## Goal

After this work, `workflow: pr` creates PRs on Bitbucket Server via its REST API. The PR lifecycle (create → review → merge) works the same as GitHub. Provider is configured in `.dark-factory.yaml`.

## Non-goals

- No GitLab support (add later using same interface)
- No Bitbucket Cloud support (different API from Bitbucket Server)
- No changes to `workflow: direct`
- No changes to Docker executor or prompt processing
- No migration of existing GitHub-specific config — GitHub remains default

## Assumptions

- Git remote URL follows standard Bitbucket Server formats: `ssh://host:port/project/repo.git` or `https://host/scm/project/repo.git`
- `defaultBranch` config field already exists (added in v0.33.1) — required for Bitbucket (no API equivalent)
- Bitbucket Server 7.x+ (REST API v1.0, default-reviewers plugin included by default)
- Default-reviewers plugin is installed (standard in Bitbucket Server, not a third-party addon)

## Desired Behavior

1. Config accepts `provider: bitbucket-server` (default: `github`)
2. Config accepts `bitbucket:` section with `baseURL` and `tokenEnv` (env var name, default: `BITBUCKET_TOKEN`) — config is git-safe, no secrets
3. PRs can be created on Bitbucket Server during pr workflow
4. PRs can be merged on Bitbucket Server when autoMerge is enabled
5. Review status can be polled from Bitbucket Server when autoReview is enabled
6. Default reviewers are assigned automatically (degrades gracefully if not available)
7. Project/repo are extracted from git remote URL — no user configuration needed
8. Default branch comes from config `defaultBranch` (required for Bitbucket)
9. All existing GitHub behavior unchanged when `provider` is `github` or omitted

Implementation guidance (not part of spec contract): see `docs/bitbucket-server-api-reference.md`

## Constraints

- Do NOT change existing git provider interfaces — implement new types behind them
- Do NOT remove `gh` CLI support — it remains the GitHub implementation
- Existing tests must pass unchanged
- Core PR operations use Bitbucket Server REST API v1.0 only; reviewer suggestions use the default-reviewers plugin (standard in 7.x+, degrades gracefully if absent)
- Token auth only (no OAuth flow for Bitbucket Server)

## Security / Abuse

- **Token in logs**: bearer token must not appear in log output or error messages — redact before logging
- **Token scope**: requires project-write permission (create PR, merge PR); read-only tokens will fail at PR creation with 403
- **SSRF via `baseURL`**: user-supplied URL is trusted (config file is local, not user input) — no additional validation needed
- **Token storage**: config stores env var name (`tokenEnv`), not the token itself — config is safe to commit; token lives only in environment

## Failure Modes

| Trigger | Behavior | Recovery |
|---------|----------|----------|
| Invalid `provider` value | Config validation error at startup | Fix config |
| Bitbucket Server unreachable | PR creation fails, prompt marked failed | Retry after fixing network |
| Token expired/invalid | 401 from API, prompt marked failed | Refresh token, retry |
| Token lacks write permission | 403 from API, prompt marked failed | Use token with project-write scope |
| PR merge conflict | Merge fails, prompt marked failed | Manual resolution |
| Missing `bitbucket.baseURL` when provider=bitbucket-server | Config validation error | Add required field |
| Default-reviewers plugin not installed | Reviewer fetch returns empty list, PR created without reviewers | Install plugin or configure `allowedReviewers` in config |
| Unparseable git remote URL | Config validation error at startup | Fix remote URL format |

## Acceptance Criteria

- [ ] `provider: github` (or omitted) works identically to current behavior
- [ ] `provider: bitbucket-server` with `bitbucket: { baseURL, token }` passes config validation
- [ ] `provider: invalid` fails config validation with clear error
- [ ] PR created on Bitbucket Server via REST API in pr workflow
- [ ] PR merged on Bitbucket Server via REST API when autoMerge enabled
- [ ] Review status fetched from Bitbucket Server when autoReview enabled
- [ ] `defaultBranch` config used (no `gh repo view` fallback attempted)
- [ ] All existing tests pass
- [ ] Config validation test: `provider=invalid` returns error
- [ ] API error test: 401 response marks prompt failed
- [ ] Token is not logged in any output
- [ ] PR created successfully without reviewers when default-reviewers plugin unavailable

## Verification

```
make precommit
```

Manual integration test with real Bitbucket Server instance:
```yaml
# .dark-factory.yaml
workflow: pr
provider: bitbucket-server
defaultBranch: master
bitbucket:
  baseURL: https://bitbucket.example.com
  tokenEnv: BITBUCKET_TOKEN
```

## Do-Nothing Option

Stay on `workflow: direct` for Bitbucket projects. Acceptable short-term — no PR review loop, but code still gets committed and pushed. Over time, teams lose the review-fix loop and auto-merge, requiring manual branch management and PR creation for every prompt.
