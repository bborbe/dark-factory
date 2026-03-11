# Bitbucket Server API Reference

Reference for implementing Bitbucket Server as a dark-factory git provider.

## Base URL

```
https://bitbucket.example.com
```

## Authentication

Bearer token in `Authorization` header.

```
Authorization: Bearer <token>
```

## API Endpoints

### Create PR

```
POST /rest/api/1.0/projects/{project}/repos/{repo}/pull-requests
```

Maps to: `PRCreator.Create()`

```json
{
  "title": "PR title",
  "description": "PR description",
  "fromRef": {
    "id": "refs/heads/feature-branch",
    "repository": {"slug": "repo", "project": {"key": "PROJECT"}}
  },
  "toRef": {
    "id": "refs/heads/master",
    "repository": {"slug": "repo", "project": {"key": "PROJECT"}}
  },
  "reviewers": [{"user": {"slug": "username"}}]
}
```

Response contains `id` (PR number), `state`, `author`, `reviewers`.

### Find Open PR

```
GET /rest/api/1.0/projects/{project}/repos/{repo}/pull-requests?state=OPEN&at=refs/heads/{branch}
```

Maps to: `PRCreator.FindOpenPR()`

### Merge PR

```
POST /rest/api/1.0/projects/{project}/repos/{repo}/pull-requests/{id}/merge
```

Maps to: `PRMerger.WaitAndMerge()`

Requires PR version in body to prevent race conditions:

```json
{"version": 1}
```

Get current version from `GET .../pull-requests/{id}` response.

### Get PR Status

```
GET /rest/api/1.0/projects/{project}/repos/{repo}/pull-requests/{id}
```

Maps to: `ReviewFetcher.FetchPRState()`

Response `state`: `OPEN`, `MERGED`, `DECLINED`.

### Get PR Reviews/Approvals

```
GET /rest/api/1.0/projects/{project}/repos/{repo}/pull-requests/{id}/activities?fromType=ACTIVITY
```

Maps to: `ReviewFetcher.FetchLatestReview()`

Alternatively, check `reviewers[].approved` and `reviewers[].status` (`APPROVED`, `NEEDS_WORK`, `UNAPPROVED`) from the PR detail endpoint.

### Get Default Reviewers

```
GET /rest/default-reviewers/1.0/projects/{project}/repos/{repo}/reviewers?sourceRepoId={id}&targetRepoId={id}&sourceRefId={ref}&targetRefId={ref}
```

Maps to: `CollaboratorFetcher.Fetch()`

Returns list of user objects. Exclude current user (PR author) from reviewers.

### Get Current User

```
GET /plugins/servlet/applinks/whoami
```

Returns plain text username (slug).

### Get Repo ID

```
GET /rest/api/1.0/projects/{project}/repos/{repo}
```

Returns repo metadata including `id` (needed for default reviewers query).

## Extracting Project/Repo from Git Remote

No API call needed — parse from remote URL:

```
ssh://bitbucket.example.com:7999/bro/sentinel.git
→ project: BRO, repo: sentinel

https://bitbucket.example.com/scm/bro/sentinel.git
→ project: BRO, repo: sentinel
```

Maps to: `RepoNameFetcher.Fetch()`

## Interface Mapping Summary

| dark-factory Interface | GitHub (current) | Bitbucket Server |
|------------------------|------------------|------------------|
| `PRCreator.Create` | `gh pr create` | `POST .../pull-requests` |
| `PRCreator.FindOpenPR` | `gh pr list --head` | `GET .../pull-requests?state=OPEN&at=` |
| `PRMerger.WaitAndMerge` | `gh pr merge --merge` | `POST .../pull-requests/{id}/merge` |
| `ReviewFetcher.FetchLatestReview` | `gh pr view --json reviews` | `GET .../pull-requests/{id}` (reviewers field) |
| `ReviewFetcher.FetchPRState` | `gh pr view --json state` | `GET .../pull-requests/{id}` (state field) |
| `CollaboratorFetcher.Fetch` | `gh api repos/.../collaborators` | `GET .../reviewers` (default reviewers API) |
| `RepoNameFetcher.Fetch` | `gh repo view --json nameWithOwner` | Parse git remote URL |
| `Brancher.DefaultBranch` | `gh repo view --json defaultBranchRef` | Config `defaultBranch` (required) |
