---
status: approved
---

# Review-Fix Loop

## Problem

After dark-factory creates a PR, the pipeline stalls waiting for a human to read review comments, write a fix prompt, and re-queue it. Even when `autoMerge` is enabled, any `request-changes` review breaks autonomous operation. The gap between PR creation and merge requires too much manual intervention.

## Goal

Dark-factory watches PRs for review activity from trusted reviewers. On approval, it merges. On `request-changes`, it generates a fix prompt and drops it in the inbox for human inspection and approval. The human approves by moving the fix prompt to the queue — the fix runs on the same branch, the PR updates, and the cycle repeats until approved or the retry limit is reached.

## Non-goals

- No fully autonomous fix application (fix prompt always lands in inbox first)
- No Bitbucket Server support (GitHub only)
- No inline comment parsing (top-level PR review body only)

## Desired Behavior

1. After PR creation, if `autoReview` is enabled, the prompt transitions to `in_review` instead of completing immediately.
2. Dark-factory periodically polls all `in_review` prompts for new reviews from trusted reviewers.
3. On `APPROVED` from a trusted reviewer → merge (delegates to `autoMerge` behavior).
4. On `CHANGES_REQUESTED` from a trusted reviewer → generate a fix prompt containing the review feedback, drop it in the inbox.
5. The fix prompt targets the existing branch and PR (via `branch` and `pr-url` fields from spec 017).
6. Human approves the fix by moving it to the queue; it executes and pushes to the existing PR.
7. If `CHANGES_REQUESTED` arrives more times than `maxReviewRetries` (default: 3), the prompt is marked `failed`.
8. Reviews from non-trusted accounts are silently ignored.
9. If the PR is merged or closed externally, the prompt is marked `completed` or `failed` accordingly.

## Constraints

- Requires `autoMerge: true` (approve path needs a merge mechanism)
- Trusted reviewer source must be configured: explicit list or repo collaborators
- Existing processor and watcher behavior must not change

## Security / Abuse Cases

- A non-collaborator posting a review must not trigger any action — reviewer identity must be verified before acting.
- Generated fix prompts must only contain review text — no executable content injected into the prompt file.
- The polling mechanism must not block prompt execution.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| No trusted review yet | Skip, poll again next interval | Wait |
| Review from untrusted account | Silently ignored | Add reviewer to whitelist |
| Retry limit reached | Prompt marked `failed` | Fix manually, merge manually |
| PR closed/merged externally | Prompt marked `completed` or `failed` | Expected — no action needed |
| GitHub API unreachable | Log warning, skip cycle, retry next interval | Check network / auth |

## Acceptance Criteria

- [ ] `in_review` status introduced; prompt transitions to it after PR creation when `autoReview: true`
- [ ] `APPROVED` from trusted reviewer triggers merge
- [ ] `CHANGES_REQUESTED` from trusted reviewer generates fix prompt in inbox with review text
- [ ] Fix prompt has `branch` and `pr-url` set targeting the existing PR
- [ ] Reviews from non-trusted accounts are ignored
- [ ] Retry count tracked; prompt marked `failed` when limit exceeded (default: 3)
- [ ] Externally closed/merged PR resolves the `in_review` prompt
- [ ] Config validation: `autoReview` requires `autoMerge` and a reviewer source

## Verification

```
make precommit
```

## Do-Nothing Option

Humans read PR reviews, write fix prompts manually, and re-queue them. Works but requires attention after every review round and breaks unattended overnight runs.
