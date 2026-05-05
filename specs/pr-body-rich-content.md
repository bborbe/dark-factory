---
status: draft
kind: feature
---

# PR body explains why the PR was created

## Summary

- Today every dark-factory PR has the body `"Automated by dark-factory"` (optionally followed by `Issue: <ref>`).
- The reviewer cannot tell from the PR alone what the change is for or which prompt produced it.
- This spec replaces the static body with the prompt's `<summary>` block plus a `Spec: <slug>` line when the prompt links a spec.
- The existing `Issue: <ref>` line and a single-line `Automated by dark-factory` provenance footer are preserved.
- All other PR mechanics (title, branch, merge, tag) are unchanged.

## Problem

`buildPRBody` in `pkg/processor/workflow_helpers.go` returns a fixed string. The prompt's authored intent — already present in the prompt file's `<summary>` block — never reaches the reviewer. To decide whether to approve, the reviewer must navigate the repo, find the prompt file by number, open it, and read the summary. That defeats the "specs/prompts as committed documentation" promise at exactly the moment it matters most.

## Goal

A reviewer opening a dark-factory PR sees the prompt's authored summary in the PR body, plus a reference to the originating spec when one exists. No repo navigation needed to understand why the PR was opened. The body explains intent, not correctness — the diff is still the source of truth.

## Non-goals

- Including the agent's `DARK-FACTORY-REPORT.summary` (what the agent claims it did).
- Truncation handling for oversized bodies — defer until a real prompt hits the limit.
- A fallback story for malformed prompts missing a `<summary>` block — body may be near-empty in that case; not catastrophic.
- Updating the manual completion path in `pkg/cmd/prompt_complete.go:162`.
- Including review feedback bodies in fix-prompt PRs (`ReviewPoller` path).

## Desired Behavior

1. When the prompt's `<summary>` block is non-empty, the PR body's first section contains its content rendered as markdown.
2. When the prompt frontmatter has `spec: [<slug>]`, the PR body contains a line `Spec: <slug>` (plain text, no URL).
3. When the prompt frontmatter has `issue: <ref>`, the PR body contains a line `Issue: <ref>` exactly as today.

## Constraints

- Reuse `pf.Summary()` from `pkg/prompt/prompt_file.go`; do not re-parse the prompt file.
- Do not change `PRCreator.Create(ctx, title, body, branch)` interface.
- Do not include the prompt body verbatim — that's the diff.
- Do not include any pre-merge artifact paths (`prompts/in-progress/...`).
- The `Issue: <ref>` line behavior must not regress.
- The PR body ends with a single-line footer `Automated by dark-factory` for provenance (preserved).
- PR title, branch name, merge behavior, and tag/release behavior are unchanged.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Prompt has non-empty `<summary>` | Body contains the summary text | N/A |
| Prompt has `spec: [065-foo]` | Body contains `Spec: 065-foo` | N/A |
| Prompt has `issue: PROJ-123` | Body contains `Issue: PROJ-123` (preserved) | N/A |
| Prompt has empty/missing `<summary>` | Body contains only `Spec:`/`Issue:` lines (if present) and the footer | None — body is best-effort; PR still opens |
| All sections empty | Body contains only the footer `Automated by dark-factory` | None — PR still opens; reviewer reads diff |

## Do-Nothing Option

Reviewers continue to context-switch to the repo for every dark-factory PR. The `autoReview` workflow remains weak because the AI reviewer also has no context. Documentation value of committed prompts is realized only in retrospect, never at review time. Cost: ongoing per-PR friction; no broken behavior, just sustained drag.

## Acceptance Criteria

- [ ] PR body contains the prompt's `<summary>` content when present.
- [ ] PR body contains `Spec: <slug>` when the prompt frontmatter links a spec.
- [ ] PR body contains `Issue: <ref>` when the prompt frontmatter has `issue:`.
- [ ] PR body ends with `Automated by dark-factory` footer.
- [ ] All four combinations of (summary present/absent × spec present/absent) produce a well-formed body.
- [ ] Scenario added under `scenarios/` (number assigned at scenario-write time): approves a prompt with a known summary, runs daemon with `pr: true`, asserts `gh pr view --json body` contains the summary text.
- [ ] CHANGELOG.md `## Unreleased` entry added.

## Verification

```bash
cd ~/Documents/workspaces/dark-factory
make precommit
bash scenarios/helper/run-NNN-pr-body.sh   # NNN assigned at scenario-write time; or walk the markdown scenario manually
```

The scenario is the load-bearing check — `make precommit` proves the builder shape, the scenario proves GitHub actually renders what the reviewer sees.

## See also

- `pkg/processor/workflow_helpers.go:33-39, 88` — `buildPRBody` and its call site.
- `pkg/prompt/prompt_file.go` — `Summary()`, `Spec()`, frontmatter accessors.
- `docs/architecture-flow.md` — "specs/prompts as committed documentation" rationale.
- Spec 065 (`bug-pr-create-missing-head-flag-in-isolated-workflows`) — fixed *that* the PR opens; this spec fixes *what it says*.
