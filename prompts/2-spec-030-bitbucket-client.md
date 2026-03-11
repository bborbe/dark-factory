---
status: created
spec: ["030"]
created: "2026-03-11T10:00:00Z"
---
<summary>
- A new Bitbucket Server HTTP client implements the same `PRCreator`, `PRMerger`, and `ReviewFetcher` interfaces the GitHub implementation uses — no interface changes required
- PRs are created on Bitbucket Server via `POST /rest/api/1.0/projects/{project}/repos/{repo}/pull-requests`
- Open PRs are detected by branch name via `GET .../pull-requests?state=OPEN&at=refs/heads/{branch}`, enabling idempotent PR creation
- PRs are merged via `POST .../pull-requests/{id}/merge` with the required version field fetched from the PR detail endpoint
- Review status (approved/changes-requested) and PR state (open/merged/declined) are read from the Bitbucket PR detail endpoint's `reviewers` array
- Default reviewers are fetched from the Bitbucket default-reviewers plugin and set on new PRs; if the plugin is unavailable the PR is created without reviewers (graceful degradation, `Fetch` returns nil)
- The bearer token never appears in log output or error messages — it is redacted before any logging
- A 401 response from the API surfaces as a clear error (prompt marked failed, recoverable by refreshing token)
</summary>

<objective>
Implement Bitbucket Server implementations of `PRCreator`, `PRMerger`, and `ReviewFetcher` in the `pkg/git` package, using the Bitbucket Server REST API v1.0. Each implementation satisfies the existing interface so the factory can swap it in without changing the processor.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` — interface/constructor/struct pattern, error wrapping, counterfeiter.
Read `/home/node/.claude/docs/go-security-linting.md` — gosec rules, #nosec comments with reasons.
Read `/home/node/.claude/docs/go-testing.md` — Ginkgo/Gomega testing patterns.

**Preconditions from prompt 1 of this spec** (must already exist):
- `pkg/config/provider.go` — `Provider`, `ProviderBitbucketServer`
- `pkg/git/bitbucket_remote.go` — `BitbucketRemoteCoords`, `ParseBitbucketRemoteURL`

Read these files before making any changes:
- `pkg/git/pr_creator.go` — `PRCreator` interface (must implement: `Create`, `FindOpenPR`)
- `pkg/git/pr_merger.go` — `PRMerger` interface (must implement: `WaitAndMerge`)
- `pkg/git/review_fetcher.go` — `ReviewFetcher` interface (must implement: `FetchLatestReview`, `FetchPRState`), `ReviewVerdict` constants
- `pkg/git/collaborator_fetcher.go` — `CollaboratorFetcher` interface (must implement: `Fetch`)
- `pkg/git/bitbucket_remote.go` — `BitbucketRemoteCoords`
- `docs/bitbucket-server-api-reference.md` — REST API endpoints, request/response formats
- `pkg/git/git_suite_test.go` — test suite setup
</context>

<requirements>
**Step 1: Shared HTTP client helper in `pkg/git/bitbucket_http.go`**

Create `pkg/git/bitbucket_http.go` with a reusable helper for authenticated Bitbucket API calls:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"

    "github.com/bborbe/errors"
)

// bitbucketClient is a minimal HTTP client for Bitbucket Server REST API.
type bitbucketClient struct {
    baseURL string
    token   string // never logged
}

// newBitbucketClient creates a bitbucketClient.
func newBitbucketClient(baseURL, token string) *bitbucketClient {
    return &bitbucketClient{
        baseURL: strings.TrimRight(baseURL, "/"),
        token:   token,
    }
}

// do executes an authenticated HTTP request and decodes the JSON response into out (may be nil).
// Returns an error for non-2xx status codes, with the token redacted from any error messages.
func (c *bitbucketClient) do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
    var bodyReader io.Reader
    if body != nil {
        b, err := json.Marshal(body)
        if err != nil {
            return errors.Wrap(ctx, err, "marshal request body")
        }
        bodyReader = bytes.NewReader(b)
    }

    url := c.baseURL + path
    // #nosec G107 -- URL is constructed from config-provided baseURL, not user input
    req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
    if err != nil {
        return errors.Wrap(ctx, err, "create http request")
    }
    req.Header.Set("Authorization", "Bearer "+c.token)
    if body != nil {
        req.Header.Set("Content-Type", "application/json")
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return errors.Wrap(ctx, err, "execute http request")
    }
    defer resp.Body.Close()

    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        respBody, _ := io.ReadAll(resp.Body)
        // redact token from logged error — do NOT include c.token in the message
        return errors.Errorf(ctx, "bitbucket API %s %s returned %d: %s", method, path, resp.StatusCode, redactToken(string(respBody), c.token))
    }

    if out != nil {
        if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
            return errors.Wrap(ctx, err, "decode response body")
        }
    }
    return nil
}

// redactToken replaces any occurrence of the token in s with "[REDACTED]".
// This ensures tokens never appear in error messages or logs.
func redactToken(s, token string) string {
    if token == "" {
        return s
    }
    return strings.ReplaceAll(s, token, "[REDACTED]")
}
```

**Step 2: Bitbucket PR creator in `pkg/git/bitbucket_pr_creator.go`**

Create `pkg/git/bitbucket_pr_creator.go`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
    "context"
    "fmt"
    "log/slog"
    "net/url"
    "os/exec"
    "strings"

    "github.com/bborbe/errors"
)

// bitbucketPRCreator implements PRCreator using the Bitbucket Server REST API.
type bitbucketPRCreator struct {
    client        *bitbucketClient
    project       string // Bitbucket project key (uppercase)
    repo          string // repository slug (lowercase)
    defaultBranch string
    reviewers     []string // optional default reviewer slugs
}

// NewBitbucketPRCreator creates a PRCreator backed by Bitbucket Server REST API.
func NewBitbucketPRCreator(
    baseURL string,
    token string,
    project string,
    repo string,
    defaultBranch string,
    reviewers []string,
) PRCreator {
    return &bitbucketPRCreator{
        client:        newBitbucketClient(baseURL, token),
        project:       project,
        repo:          repo,
        defaultBranch: defaultBranch,
        reviewers:     reviewers,
    }
}

// Create creates a pull request on Bitbucket Server and returns the PR web URL.
func (p *bitbucketPRCreator) Create(ctx context.Context, title string, body string) (string, error) {
    type reviewer struct {
        User struct {
            Slug string `json:"slug"`
        } `json:"user"`
    }

    reviewerList := make([]reviewer, 0, len(p.reviewers))
    for _, slug := range p.reviewers {
        var r reviewer
        r.User.Slug = slug
        reviewerList = append(reviewerList, r)
    }

    // We need the current branch name from git to set fromRef.
    // #nosec G204 -- fixed git command, not user input
    branchBytes, err := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD").Output()
    if err != nil {
        return "", errors.Wrap(ctx, err, "get current branch")
    }
    currentBranch := strings.TrimSpace(string(branchBytes))

    payload := map[string]interface{}{
        "title":       title,
        "description": body,
        "fromRef": map[string]interface{}{
            "id": "refs/heads/" + currentBranch,
            "repository": map[string]interface{}{
                "slug":    p.repo,
                "project": map[string]string{"key": p.project},
            },
        },
        "toRef": map[string]interface{}{
            "id": "refs/heads/" + p.defaultBranch,
            "repository": map[string]interface{}{
                "slug":    p.repo,
                "project": map[string]string{"key": p.project},
            },
        },
        "reviewers": reviewerList,
    }

    var result struct {
        ID    int    `json:"id"`
        Links struct {
            Self []struct {
                Href string `json:"href"`
            } `json:"self"`
        } `json:"links"`
    }

    path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests", p.project, p.repo)
    if err := p.client.do(ctx, "POST", path, payload, &result); err != nil {
        return "", errors.Wrap(ctx, err, "create pull request")
    }

    if len(result.Links.Self) > 0 && result.Links.Self[0].Href != "" {
        slog.Info("created Bitbucket PR", "id", result.ID, "url", result.Links.Self[0].Href)
        return result.Links.Self[0].Href, nil
    }
    // Fallback: construct URL from PR ID
    prURL := fmt.Sprintf("%s/projects/%s/repos/%s/pull-requests/%d",
        p.client.baseURL, p.project, p.repo, result.ID)
    slog.Info("created Bitbucket PR", "id", result.ID, "url", prURL)
    return prURL, nil
}

// FindOpenPR returns the URL of an open PR for the given branch, or "" if none exists.
func (p *bitbucketPRCreator) FindOpenPR(ctx context.Context, branch string) (string, error) {
    var result struct {
        Values []struct {
            ID    int `json:"id"`
            Links struct {
                Self []struct {
                    Href string `json:"href"`
                } `json:"self"`
            } `json:"links"`
        } `json:"values"`
    }

    // Encode the branch ref as a query parameter
    branchRef := url.QueryEscape("refs/heads/" + branch)
    path := fmt.Sprintf(
        "/rest/api/1.0/projects/%s/repos/%s/pull-requests?state=OPEN&at=%s",
        p.project, p.repo, branchRef,
    )

    if err := p.client.do(ctx, "GET", path, nil, &result); err != nil {
        return "", errors.Wrap(ctx, err, "find open pull request")
    }

    if len(result.Values) == 0 {
        return "", nil
    }

    pr := result.Values[0]
    if len(pr.Links.Self) > 0 && pr.Links.Self[0].Href != "" {
        return pr.Links.Self[0].Href, nil
    }
    return fmt.Sprintf("%s/projects/%s/repos/%s/pull-requests/%d",
        p.client.baseURL, p.project, p.repo, pr.ID), nil
}
```

**Step 3: Bitbucket PR merger in `pkg/git/bitbucket_pr_merger.go`**

Create `pkg/git/bitbucket_pr_merger.go`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
    "context"
    "fmt"
    "log/slog"
    "strconv"
    "strings"

    "github.com/bborbe/errors"
)

// bitbucketPRMerger implements PRMerger using the Bitbucket Server REST API.
type bitbucketPRMerger struct {
    client  *bitbucketClient
    project string
    repo    string
}

// NewBitbucketPRMerger creates a PRMerger backed by Bitbucket Server REST API.
func NewBitbucketPRMerger(baseURL, token, project, repo string) PRMerger {
    return &bitbucketPRMerger{
        client:  newBitbucketClient(baseURL, token),
        project: project,
        repo:    repo,
    }
}

// WaitAndMerge merges the Bitbucket PR identified by prURL.
// Bitbucket does not require polling for mergeability in the same way as GitHub —
// it returns an error immediately if the PR cannot be merged.
func (m *bitbucketPRMerger) WaitAndMerge(ctx context.Context, prURL string) error {
    prID, err := parseBitbucketPRID(prURL)
    if err != nil {
        return errors.Wrap(ctx, err, "parse PR ID from URL")
    }

    // Fetch PR version (required by Bitbucket merge API to prevent races)
    version, err := m.fetchPRVersion(ctx, prID)
    if err != nil {
        return errors.Wrap(ctx, err, "fetch PR version")
    }

    path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/merge",
        m.project, m.repo, prID)
    payload := map[string]int{"version": version}

    if err := m.client.do(ctx, "POST", path, payload, nil); err != nil {
        return errors.Wrap(ctx, err, "merge pull request")
    }

    slog.Info("merged Bitbucket PR", "id", prID)
    return nil
}

// fetchPRVersion fetches the current version of the PR (needed for the merge request body).
func (m *bitbucketPRMerger) fetchPRVersion(ctx context.Context, prID int) (int, error) {
    var result struct {
        Version int `json:"version"`
    }
    path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d",
        m.project, m.repo, prID)
    if err := m.client.do(ctx, "GET", path, nil, &result); err != nil {
        return 0, errors.Wrap(ctx, err, "fetch PR detail")
    }
    return result.Version, nil
}

// parseBitbucketPRID extracts the numeric PR ID from a Bitbucket PR URL.
// Expected format: .../pull-requests/{id} or .../pull-requests/{id}/overview
func parseBitbucketPRID(prURL string) (int, error) {
    // Remove trailing path segments after the PR ID
    parts := strings.Split(strings.TrimRight(prURL, "/"), "/")
    for i, part := range parts {
        if part == "pull-requests" && i+1 < len(parts) {
            idStr := parts[i+1]
            id, err := strconv.Atoi(idStr)
            if err != nil {
                return 0, fmt.Errorf("invalid PR ID %q in URL %q", idStr, prURL)
            }
            return id, nil
        }
    }
    return 0, fmt.Errorf("could not extract PR ID from URL %q", prURL)
}
```

**Step 4: Bitbucket review fetcher in `pkg/git/bitbucket_review_fetcher.go`**

Create `pkg/git/bitbucket_review_fetcher.go`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
    "context"
    "fmt"
    "strings"

    "github.com/bborbe/errors"
)

// bitbucketReviewFetcher implements ReviewFetcher using the Bitbucket Server REST API.
type bitbucketReviewFetcher struct {
    client  *bitbucketClient
    project string
    repo    string
}

// NewBitbucketReviewFetcher creates a ReviewFetcher backed by Bitbucket Server REST API.
func NewBitbucketReviewFetcher(baseURL, token, project, repo string) ReviewFetcher {
    return &bitbucketReviewFetcher{
        client:  newBitbucketClient(baseURL, token),
        project: project,
        repo:    repo,
    }
}

// bitbucketPRDetail is the response from GET .../pull-requests/{id}
type bitbucketPRDetail struct {
    State     string `json:"state"` // OPEN, MERGED, DECLINED
    Reviewers []struct {
        User struct {
            Slug string `json:"slug"`
        } `json:"user"`
        Approved bool   `json:"approved"`
        Status   string `json:"status"` // APPROVED, NEEDS_WORK, UNAPPROVED
    } `json:"reviewers"`
}

// FetchLatestReview returns the latest review verdict from a trusted reviewer on the given PR.
func (f *bitbucketReviewFetcher) FetchLatestReview(
    ctx context.Context,
    prURL string,
    allowedReviewers []string,
) (*ReviewResult, error) {
    prID, err := parseBitbucketPRID(prURL)
    if err != nil {
        return nil, errors.Wrap(ctx, err, "parse PR ID from URL")
    }

    var detail bitbucketPRDetail
    path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d",
        f.project, f.repo, prID)
    if err := f.client.do(ctx, "GET", path, nil, &detail); err != nil {
        return nil, errors.Wrap(ctx, err, "fetch PR detail")
    }

    allowed := make(map[string]bool, len(allowedReviewers))
    for _, r := range allowedReviewers {
        allowed[r] = true
    }

    // Scan reviewers for a trusted verdict (last trusted reviewer wins)
    var last *ReviewResult
    for _, reviewer := range detail.Reviewers {
        if !allowed[reviewer.User.Slug] {
            continue
        }
        var verdict ReviewVerdict
        switch reviewer.Status {
        case "APPROVED":
            verdict = ReviewVerdictApproved
        case "NEEDS_WORK":
            verdict = ReviewVerdictChangesRequested
        default:
            verdict = ReviewVerdictNone
        }
        last = &ReviewResult{Verdict: verdict, Body: ""}
    }

    if last == nil {
        return &ReviewResult{Verdict: ReviewVerdictNone}, nil
    }
    return last, nil
}

// FetchPRState returns the PR state as a string: "OPEN", "MERGED", or "CLOSED".
// Bitbucket uses "DECLINED" — map it to "CLOSED" for compatibility with the review poller.
func (f *bitbucketReviewFetcher) FetchPRState(ctx context.Context, prURL string) (string, error) {
    prID, err := parseBitbucketPRID(prURL)
    if err != nil {
        return "", errors.Wrap(ctx, err, "parse PR ID from URL")
    }

    var detail bitbucketPRDetail
    path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d",
        f.project, f.repo, prID)
    if err := f.client.do(ctx, "GET", path, nil, &detail); err != nil {
        return "", errors.Wrap(ctx, err, "fetch PR state")
    }

    state := strings.ToUpper(detail.State)
    if state == "DECLINED" {
        return "CLOSED", nil
    }
    return state, nil
}
```

**Step 5: Bitbucket default reviewer fetcher in `pkg/git/bitbucket_collaborator_fetcher.go`**

Create `pkg/git/bitbucket_collaborator_fetcher.go` implementing `CollaboratorFetcher`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
    "context"
    "fmt"
    "log/slog"
    "net/url"
)

// bitbucketCollaboratorFetcher implements CollaboratorFetcher using Bitbucket default reviewers plugin.
// If the default-reviewers plugin is unavailable (404), it gracefully returns an empty list.
type bitbucketCollaboratorFetcher struct {
    client        *bitbucketClient
    project       string
    repo          string
    defaultBranch string
    currentUser   string // excluded from reviewer list (PR author)
}

// NewBitbucketCollaboratorFetcher creates a CollaboratorFetcher backed by Bitbucket Server.
// currentUser is the slug of the user creating PRs — they are excluded from the reviewer list.
func NewBitbucketCollaboratorFetcher(
    baseURL, token, project, repo, defaultBranch, currentUser string,
) CollaboratorFetcher {
    return &bitbucketCollaboratorFetcher{
        client:        newBitbucketClient(baseURL, token),
        project:       project,
        repo:          repo,
        defaultBranch: defaultBranch,
        currentUser:   currentUser,
    }
}

// Fetch returns the list of default reviewer slugs from the Bitbucket default-reviewers plugin.
// Returns nil on error or if the plugin is not installed — graceful degradation.
func (f *bitbucketCollaboratorFetcher) Fetch(ctx context.Context) []string {
    // Step 1: Get repo ID (required for default-reviewers query)
    repoID, err := f.fetchRepoID(ctx)
    if err != nil {
        slog.Warn("bitbucket: failed to fetch repo ID for default reviewers", "error", err)
        return nil
    }

    // Step 2: Query default-reviewers plugin
    // Use defaultBranch as sourceRef too — we only need a valid ref to query the default-reviewers
    // plugin for its configured reviewer rules. The actual source branch doesn't matter for
    // reviewer resolution; Bitbucket uses the target branch's reviewer configuration.
    sourceRef := url.QueryEscape("refs/heads/" + f.defaultBranch)
    targetRef := url.QueryEscape("refs/heads/" + f.defaultBranch)
    path := fmt.Sprintf(
        "/rest/default-reviewers/1.0/projects/%s/repos/%s/reviewers?sourceRepoId=%d&targetRepoId=%d&sourceRefId=%s&targetRefId=%s",
        f.project, f.repo, repoID, repoID, sourceRef, targetRef,
    )

    var result []struct {
        Slug string `json:"slug"`
    }
    if err := f.client.do(ctx, "GET", path, nil, &result); err != nil {
        slog.Warn("bitbucket: default-reviewers plugin unavailable or returned error — PR will be created without reviewers", "error", err)
        return nil
    }

    reviewers := make([]string, 0, len(result))
    for _, r := range result {
        if r.Slug == f.currentUser {
            continue // exclude PR author
        }
        reviewers = append(reviewers, r.Slug)
    }
    return reviewers
}

// fetchRepoID returns the numeric Bitbucket repo ID from the repo detail endpoint.
func (f *bitbucketCollaboratorFetcher) fetchRepoID(ctx context.Context) (int, error) {
    var result struct {
        ID int `json:"id"`
    }
    path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s", f.project, f.repo)
    if err := f.client.do(ctx, "GET", path, nil, &result); err != nil {
        return 0, err
    }
    return result.ID, nil
}
```

**Step 6: Tests**

Since `parseBitbucketPRID` is unexported, add these tests to `pkg/git/git_internal_test.go` (existing internal test file):

```go
var _ = Describe("parseBitbucketPRID", func() {
    DescribeTable("valid URLs",
        func(prURL string, expectedID int) {
            id, err := parseBitbucketPRID(prURL)
            Expect(err).NotTo(HaveOccurred())
            Expect(id).To(Equal(expectedID))
        },
        Entry("standard URL", "https://bitbucket.example.com/projects/BRO/repos/sentinel/pull-requests/42", 42),
        Entry("URL with trailing slash", "https://bitbucket.example.com/projects/BRO/repos/sentinel/pull-requests/42/", 42),
        Entry("URL with /overview suffix", "https://bitbucket.example.com/projects/BRO/repos/sentinel/pull-requests/42/overview", 42),
        Entry("PR ID 1", "https://bitbucket.example.com/projects/BRO/repos/sentinel/pull-requests/1", 1),
    )

    DescribeTable("invalid URLs return error",
        func(prURL string) {
            _, err := parseBitbucketPRID(prURL)
            Expect(err).To(HaveOccurred())
        },
        Entry("GitHub PR URL", "https://github.com/owner/repo/pull/42"),
        Entry("no pull-requests segment", "https://bitbucket.example.com/projects/BRO/repos/sentinel"),
        Entry("non-numeric ID", "https://bitbucket.example.com/projects/BRO/repos/sentinel/pull-requests/abc"),
        Entry("empty string", ""),
    )
})
```

Since `redactToken` is also unexported, add to `pkg/git/git_internal_test.go`:

```go
var _ = Describe("redactToken", func() {
    It("replaces token occurrences with [REDACTED]", func() {
        result := redactToken("error: Bearer mysecrettoken in response", "mysecrettoken")
        Expect(result).NotTo(ContainSubstring("mysecrettoken"))
        Expect(result).To(ContainSubstring("[REDACTED]"))
    })

    It("is a no-op when token is empty", func() {
        result := redactToken("some error message", "")
        Expect(result).To(Equal("some error message"))
    })

    It("is a no-op when token does not appear in string", func() {
        result := redactToken("some error message", "notpresent")
        Expect(result).To(Equal("some error message"))
    })
})
```
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT change `PRCreator`, `PRMerger`, `ReviewFetcher`, or `CollaboratorFetcher` interfaces — implement new types that satisfy them
- The bearer token must NEVER appear in log output or error messages — always call `redactToken` before including response bodies in errors
- `bitbucketCollaboratorFetcher.Fetch` must return `nil` (not error) when the default-reviewers plugin is unavailable (404 or connection error) — graceful degradation
- The `FetchPRState` method maps Bitbucket's `"DECLINED"` to `"CLOSED"` for compatibility with the existing review poller (which checks for `"MERGED"` and `"CLOSED"`)
- `parseBitbucketPRID` is a pure function — no network calls — test without mocks
- The `Create` method in `bitbucketPRCreator` reads the current branch via `git rev-parse --abbrev-ref HEAD` — this works because `Create` is called from the clone directory where the feature branch is checked out
- Follow existing error wrapping: `errors.Wrap(ctx, err, "message")`
- All existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
```bash
# Bitbucket HTTP client exists
grep -n "bitbucketClient\|newBitbucketClient\|redactToken" pkg/git/bitbucket_http.go

# Bitbucket PRCreator constructor
grep -n "NewBitbucketPRCreator" pkg/git/bitbucket_pr_creator.go

# Bitbucket PRMerger constructor
grep -n "NewBitbucketPRMerger" pkg/git/bitbucket_pr_merger.go

# Bitbucket ReviewFetcher constructor
grep -n "NewBitbucketReviewFetcher" pkg/git/bitbucket_review_fetcher.go

# Bitbucket CollaboratorFetcher constructor
grep -n "NewBitbucketCollaboratorFetcher" pkg/git/bitbucket_collaborator_fetcher.go

# parseBitbucketPRID tests exist
grep -n "parseBitbucketPRID" pkg/git/git_internal_test.go

# redactToken tests exist
grep -n "redactToken" pkg/git/git_internal_test.go

make precommit
```
Must pass with no errors.
</verification>
