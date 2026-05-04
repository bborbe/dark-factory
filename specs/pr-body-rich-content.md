---
status: idea
---

# Rich PR body content — surface prompt summary, spec link, and agent report to reviewers

## Summary

Today every PR opened by dark-factory has the body string `"Automated by dark-factory"` (optionally followed by `\n\nIssue: <ref>` when the prompt frontmatter has an `issue:` field). This is the originally-specified behavior — the PR-creation feature was scoped to a hardcoded provenance string and an optional issue reference. PR-body content was never part of any earlier spec.

This spec adds the missing capability: render the prompt's `<summary>` block, a link to the originating spec (when one exists), and the agent's `DARK-FACTORY-REPORT.summary` into the PR body. The reviewer landing on the PR sees what the change does, why it was made, and what the agent claims it implemented — without context-switching to the repo to find the prompt file.

This is a feature addition, not a regression fix. The current "Automated by dark-factory" line was the spec; we're extending the spec to deliver more value to the reviewer.

## Motivation

The headline value of the prompt/spec flow per `docs/architecture-flow.md` is "documentation — committed prompts and specs survive long after the implementation context is gone." The PR is the most leveraged moment that documentation should reach the reviewer. Today's hardcoded string voids that value at the review point: a reviewer either skims the diff blind or context-switches to the repo to find the prompt and spec files.

The autoReview path benefits the most. autoReview is only useful if the reviewer can review *informed*. A reviewer skimming the diff with no summary in the PR body either approves blind or routes the PR to manual review unnecessarily — the prompt and spec content already exist; the only gap is making them reach the reviewer's screen.

The change is purely additive on the reviewer-facing surface: existing PRs and existing prompts continue to merge cleanly. The PR body becomes richer, not smaller. The agent's behavior, the merge logic, the tag/release path all stay unchanged.

## Today's behavior (baseline)

- `pkg/processor/workflow_helpers.go:34` — `buildPRBody(issue string) string` returns either `"Automated by dark-factory"` or `"Automated by dark-factory\n\nIssue: <ref>"`. No prompt content, no spec content, no agent report.
- `pkg/cmd/prompt_complete.go:162` — `c.prCreator.Create(gitCtx, title, "Automated by dark-factory", branch)`. Same hardcoded string from the manual `prompt complete` path.
- The agent already writes a `DARK-FACTORY-REPORT` JSON block to the log with `status` and `summary` fields. The summary is also persisted to the prompt frontmatter as `summary:`. Both are available host-side at PR-creation time. Neither is consulted today.

This is the originally-specified behavior — see `pkg/processor/workflow_helpers.go` history. No regression to fix; a feature gap to fill.

## How a reviewer experiences the change today

A reviewer opening a dark-factory-created PR sees the diff plus a single line "Automated by dark-factory." To make an informed decision they navigate to the repo, find the prompt file (the filename includes the prompt number), open it, read the `<summary>`, optionally cross-reference the linked spec. Three or four extra clicks per review. After this spec lands, the reviewer reads the PR body in place.

## Code pointers

- `pkg/processor/workflow_helpers.go:33-39` — `buildPRBody` is the function to expand.
- `pkg/processor/workflow_helpers.go:88` — call site in `findOrCreatePR`. Currently passes only `issue` to `buildPRBody`. To produce a real body, this site needs access to the prompt frontmatter (`pf *prompt.PromptFile`) — already in scope as a parameter to `handleAfterIsolatedCommit`.
- `pkg/cmd/prompt_complete.go:162` — second call site, same hardcoded string. Must use the same body-construction logic.
- `pkg/prompt/prompt_file.go` — `PromptFile` struct exposes `Summary()`, `Body()`, `Spec()`, frontmatter fields. The summary is stored verbatim from the prompt file (`<summary>` block content) when the agent reports completion. The originating spec ID is in `spec: [...]` frontmatter.
- `pkg/review/poller.go` — when ChangesRequested, the poller already extracts the review body and feeds it into a fix prompt (`reviewBody` parameter to `handleChangesRequested`). The reverse direction — promoting prompt content into the PR body — is not implemented.

## Failure Modes

| Trigger | Expected behavior | Recovery / verification |
|---------|-------------------|--------------------------|
| Prompt has `<summary>` block; PR opens | PR body contains the summary text, formatted for GitHub markdown rendering | Visit PR, body shows the summary; not just "Automated by dark-factory" |
| Prompt has `spec: [065-foo-bar]` frontmatter | PR body links to or quotes the spec (URL to GitHub raw / repo path / inlined first paragraph) | PR body shows "Spec: 065-foo-bar" with a link or inline excerpt |
| Prompt has `issue: PROJ-123` | PR body still includes the issue reference (existing behavior preserved) | PR body shows both summary AND `Issue: PROJ-123` |
| Agent's `DARK-FACTORY-REPORT.summary` is set | PR body includes a "What the agent did" section with that summary | PR body has both "What was requested" (prompt summary) and "What was implemented" (agent report summary) |
| Prompt has no `<summary>` block (legacy or malformed) | Body falls back to a sensible default — at minimum the prompt filename and a link to the file at HEAD | PR body shows the filename, not an empty placeholder |
| Body exceeds GitHub's PR-body character limit (65536) | Body is truncated with a `...truncated` marker and a link to the full prompt file in the merged commit | PR body is non-empty, parseable, and visibly truncated |
| Prompt is a fix prompt generated by `ReviewPoller` from a previous ChangesRequested | Body includes the original review body that triggered the fix, so the same reviewer sees what they asked for and what was changed | PR body has a "Responding to feedback" section quoting the prior review body |

## Goal

After this fix, every PR opened by dark-factory has a body that includes:
1. The prompt's `<summary>` (or fallback if missing),
2. A reference to the originating spec (with link to the file at the merge commit) when one exists,
3. The agent's own `DARK-FACTORY-REPORT.summary` describing what was implemented,
4. The existing `Issue: <ref>` line when the prompt frontmatter has it.

A reviewer can decide whether to approve based on the PR alone, without context-switching to the repo.

## Constraints

- Do NOT change the PR title — `title` is already constructed from the prompt's H1 heading and that's working.
- Do NOT change the `Brancher`/`PRCreator` interface signatures beyond what's strictly necessary to thread the prompt content through. If `prCreator.Create(ctx, title, body, branch)` is the right shape, the only change is what fills `body`.
- Do NOT include the entire prompt body verbatim — that's the diff. Only the structured metadata sections (`<summary>`, `spec:`, agent report).
- Do NOT include the agent's full DARK-FACTORY-REPORT JSON — only its `summary` field, formatted for human reading.
- Do NOT regress the `Issue:` reference — existing prompts using `issue:` frontmatter must continue to render that line.
- Do NOT exceed GitHub's PR-body character limit; truncate with a clear marker and link to the merged file.
- Do NOT include any pre-merge artefact paths (`prompts/in-progress/...`) — link to `prompts/completed/<NNN>-...md` at the merge commit, since by the time the reviewer reads the PR the file may already be in flight to `completed/`.
- Reuse existing prompt-file accessors (`pf.Summary()`, etc.); do not re-parse the prompt file separately in the PR-body builder.

## Verification

A reviewer-facing rendering change must be verified against the built binary by reading the actual PR body GitHub renders, not just by unit-testing the body-builder function. Unit tests prove the string shape; runtime replay proves the markdown actually renders well in GitHub.

**Repro replay:**

```bash
# In jira-task-creator with pr: true:
cd ~/Documents/workspaces/jira-task-creator
# Drop a prompt with a rich <summary> block and a spec: [...] frontmatter
dark-factory prompt approve <name>
dark-factory daemon &
DAEMON_PID=$!

# Wait for PR to open (status: in_review or open)
PR_URL=$(grep "url=" .dark-factory.log | tail -1 | sed 's/.*url=//')
gh pr view "$PR_URL" --json body -q .body > /tmp/pr-body.txt

# Expected:
#   /tmp/pr-body.txt contains the prompt's <summary> text (≥1 line match)
#   /tmp/pr-body.txt contains "spec:" or links to the spec file
#   /tmp/pr-body.txt contains agent summary if present in prompt frontmatter
#   /tmp/pr-body.txt does NOT contain ONLY "Automated by dark-factory"

# Confirm:
grep -c "<actual summary text from prompt>" /tmp/pr-body.txt          # ≥1
grep -c "Automated by dark-factory" /tmp/pr-body.txt                  # at most a small footer line, NOT the only content
[ "$(wc -l < /tmp/pr-body.txt)" -gt 5 ]                               # body has structure, not a single line

kill $DAEMON_PID
```

**Negative-control replays:**

1. Prompt without `<summary>` block. Expected: body still has structure (filename, fallback explanation). NOT empty.
2. Prompt with `issue:` frontmatter. Expected: body retains the `Issue: <ref>` line in addition to the new content. Existing behavior preserved.
3. Very long prompt (`<summary>` > 60K chars). Expected: body is truncated with marker, contains a link to the prompt file at the merge commit.

**Acceptable evidence for `verifying → completed`:**

| Evidence | Acceptable? |
|----------|-------------|
| `gh pr view <url> --json body` shows multi-section body with summary | Yes |
| Visiting the PR on GitHub shows rendered markdown sections | Yes |
| Reviewer can decide approve/changes-requested from PR alone, no repo context-switch needed | Yes (subjective but operationally meaningful) |
| Unit test asserting `buildPRBody` includes `pf.Summary()` | Necessary but not sufficient |
| "All tests pass" without runtime replay | No |

## Open Questions

1. Should the body also include a "How to review" hint pointing to the prompt's `<verification>` block? Triage decision — could be useful for humans, could be redundant for committed prompts.
2. What's the canonical link target for the originating spec? Options: GitHub blob URL at the merge commit (stable), repo-relative path (renders as a link in the GitHub UI but not always portable), or both. Triage before approval.
3. The agent's `DARK-FACTORY-REPORT.summary` is sometimes a one-liner, sometimes a paragraph. Should the body section that quotes it have a fixed header, or adapt to length? Likely fixed header for consistency; defer to fix-prompt generation.
4. `pkg/cmd/prompt_complete.go:162` (the manual-completion path) doesn't have access to the agent's `DARK-FACTORY-REPORT` (because the agent ran outside the daemon's lifecycle). It does have the prompt file. Body for that path should fall back to "what the prompt asked for" only, no agent report. Document this divergence.
5. Should the body footer still say "Automated by dark-factory" as a single line, for transparency? Probably yes — it's a useful provenance signal — but it should be one line in a footer, not the entire body.

## See also

- Spec 065 (`bug-pr-create-missing-head-flag-in-isolated-workflows`) — fixed PR creation; that spec addressed *that the PR opens*; this spec addresses *what the PR says*.
- Spec for `bug-autoreview-skips-postmerge-actions-no-tag-no-release` — sibling autoReview-path work.
- `docs/architecture-flow.md` — the "specs/prompts as documentation" promise this feature delivers on.
- `pkg/processor/workflow_helpers.go:33-39, 88` — implementation site.
- `pkg/cmd/prompt_complete.go:162` — second call site.
- `pkg/prompt/prompt_file.go` — accessors for the data the body needs.
