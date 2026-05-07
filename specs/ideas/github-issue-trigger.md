---
tags:
  - dark-factory
  - spec
status: idea
---

## Summary

- Dark-factory service watches GitHub issues with a trigger label (e.g. `decompose` / `dark-factory`)
- The issue body IS the spec — no separate `specs/foo.md` file
- `audit-spec` runs on the issue body as a quality gate
- On pass: prompts are generated and executed
- On halt: dark-factory comments on the issue with what's missing
- Output options: A) file N child issues with `claude` label for Seibert vibe-ci to take over; B) direct PR back to consumer repo
- Trigger model mirrors Seibert vibe-ci's `claude` label primitive — same UX for the team

## Problem

Today, dark-factory ingest is filesystem-based: write a markdown file to `specs/` or `prompts/`, run a CLI to approve. That's fine for a single developer on a local machine, but wrong for a team-shared cloud service:

- Requires a synced git repo with structured directories
- Requires a CLI approval step for every prompt
- Doesn't compose with how teams already file work (GitHub issues, label-based triggers)
- Editing an approved spec/prompt requires manual file moves + status edits

Seibert vibe-ci already proves the right primitive: a GitHub issue with a state label is the trigger. The team knows how to file issues, how labels work, how to remove + re-apply for a retry. Dark-factory should reuse that primitive instead of inventing a parallel approval flow.

## Goal

After this work, dark-factory runs as a continuously-running cloud service that watches a Seibert-internal GitHub org for issues with a trigger label. When an issue lands in that state, dark-factory treats the issue body as a spec, runs `audit-spec`, and either halts with a comment (on missing constraints / failure modes / etc.) or proceeds to generate and execute prompts. No filesystem, no CLI approval — same UX shape the team already uses for vibe-ci.

## Architecture

```
GitHub Issue (label: `decompose`)
       │
       ▼  webhook OR poll loop
┌──────────────────────────────────┐
│  Dark-factory service (k8s)      │
│                                  │
│  1. Detect labeled issues        │
│  2. Read issue body              │
│  3. audit-spec on issue body     │
│      ├── halt → gh issue comment │
│      └── pass ↓                  │
│  4. generate-prompts (N)         │
│  5. execute via Executor         │
│      (k8s Job per prompt)        │
└──────────────────┬───────────────┘
                   │
       ┌───────────┴────────────┐
       ▼                        ▼
  Option A:                 Option B:
  N child issues with       Direct PRs back to
  `claude` label →          consumer repo
  vibe-ci takes over
  (parse → plan →
  implement → PR-chain)
```

## Non-goals

- Replacing the local Docker + filesystem trigger (both coexist)
- Custom GitHub App (use webhooks or polling with a PAT)
- Multi-repo orchestration in one trigger event (one issue → one decomposition run)
- Comment-driven retrigger (only label state changes matter — same as vibe-ci)
- Replacing vibe-ci (option A is hand-off, not replacement)

## Desired Behavior

1. **Trigger label is configurable.** Default `decompose` (or `dark-factory`). Service watches a configured GitHub org/repo for issues that gain this label.

2. **Actor gate.** Only repo writers can trigger — same model as vibe-ci. Triage-only labelers get a no-op + explanatory comment.

3. **Issue body = spec.** No separate spec file written or expected. The audit and prompt generation run against the issue body verbatim. Editing the issue and removing+re-applying the label = clean restart.

4. **Audit-spec halt-gate.** Audit runs against the issue body. Halts on missing required sections (constraints / failure modes / acceptance criteria / do-nothing analysis), out-of-scope content, or low confidence. Halt path posts a structured comment listing what's missing and how to fix it. Pass path proceeds to prompt generation.

5. **Prompt generation.** On pass, generate N prompts using the existing decomposition logic. Prompts are produced as transient artifacts in the service's working dir (or as content for child issues — see option A).

6. **Output mode A: hand-off to vibe-ci.** For each generated prompt, file a new GitHub issue with the `claude` label and a body that reads as a vibe-ci-compatible PRD. Vibe-ci's existing pipeline takes over (parse → plan → implement → PR). The original `decompose` issue gets a comment linking to all child issues.

7. **Output mode B: direct execution.** For each generated prompt, dark-factory executes via the existing k8s Job executor. Each prompt produces its own PR back to the consumer repo. Output mode is per-trigger configurable (label suffix, comment directive, or service config).

8. **Idempotent on repeated label apply.** If the label is removed and re-applied, the service treats it as a fresh attempt: prior child issues / PRs from the previous attempt remain (for diff/comparison), the new attempt creates a fresh batch. Comment on issue links to the new attempt's outputs.

9. **Concurrency control.** The same `maxContainers` global limit from k8s-execution applies. Multiple simultaneous trigger events queue cleanly; per-issue concurrency group prevents the same issue from running twice in parallel.

10. **Status surfaced via issue comments.** No separate dashboard. Each major step (audit pass, prompts generated, execution started, child issues filed, attempt complete) posts a comment on the trigger issue. Same UX shape as vibe-ci's parser/planner comments.

## Constraints

- **Reuse `audit-spec` logic** — the audit runs on the issue body without modification; no parallel "issue auditor" implementation.
- **Reuse the existing Executor interface** — k8s Job execution from `k8s-execution.md` is the runtime; this spec only adds the trigger surface.
- **No new approval CLI for the team** — applying the label IS the approval, removing+re-applying is the retry, just like vibe-ci.
- **Issue body remains the canonical source of truth.** No separate `specs/` markdown is written or read for triggered runs. (Local filesystem trigger keeps using `specs/` — the two coexist.)
- **No comments are read as authoritative input.** Only the issue title + body. Same rule as vibe-ci's parser. Comments may surface from the service to the user, but never the other way.
- **Local Docker executor + filesystem trigger continue working** — this spec is additive.
- **All existing tests pass, `make precommit` passes.**

## Failure Modes

| Trigger                                              | Expected behavior                                                                            | Recovery                                                  |
| ---------------------------------------------------- | -------------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| Audit-spec halts (missing constraints / etc.)        | Comment posted listing what's missing; no execution                                          | User edits issue + re-applies label                       |
| Audit-spec halts (out-of-scope / low confidence)     | Comment posted explaining halt; no execution                                                 | User refines issue scope + re-applies label               |
| Webhook missed (network blip, service restart)       | Poll loop catches up on next interval                                                        | Automatic                                                 |
| Polling rate limit hit                               | Exponential backoff, daemon continues                                                        | Automatic                                                 |
| Trigger label applied by non-writer                  | No-op + comment "only writers can trigger"                                                   | Repo writer applies label                                 |
| Concurrent label-apply on same issue                 | Per-issue concurrency group serializes; second event waits for first to complete             | Automatic                                                 |
| Output mode A: child-issue creation fails            | Comment on trigger issue with error; no rollback (some child issues may already exist)       | User cleans up partial child issues + re-applies label    |
| Output mode B: prompt execution fails                | Existing Executor failure path: prompt marked failed, comment on trigger issue               | User retries via re-apply label                           |
| GitHub API token expired                             | Daemon health-check fails; pod restarts; alert raised                                        | Operator rotates token                                    |
| Issue body too large (> some cap)                    | Audit halts with "issue body exceeds cap"; suggests splitting                                | User decomposes manually before re-applying label         |
| Trigger issue deleted mid-execution                  | Service detects 404, marks attempt aborted; child issues / PRs already created remain        | None needed — clean abort                                 |

## Security / Abuse Cases

- **Actor gate** is the primary defense — same model vibe-ci uses. Only repo writers can apply the trigger label. Triage-only contributors see a no-op + comment.
- **Untrusted-content boundary**: issue title + body are user-controlled. The audit-spec prompt and prompt-generation prompt MUST treat them as data, not instructions (same `<untrusted_user_content>` wrapping pattern vibe-ci uses for the parser).
- **API tokens via K8S Secrets** — Anthropic API key, GitHub PAT, claude-yolo config. Never logged, never in ConfigMaps.
- **GitHub PAT scope** — minimal: read+write on the configured repos for issues, labels, comments. No org-admin, no workflow-edit.
- **No webhook authentication bypass** — if using webhooks, validate the GitHub-Hub-Signature-256 header against a shared secret.
- **Audit comment injection** — when posting status comments, escape any user-controlled content from the issue body to prevent comment-injection attacks against downstream readers (other tools that parse the issue thread).
- **Output mode A child issues** inherit the actor of the trigger event in their body's "@-mention" but are filed by the bot account. Vibe-ci's actor gate gates the downstream `claude` label apply on the bot, not the original user.
- **Resource limits on Jobs** — same as k8s-execution spec. NetworkPolicy: outbound HTTPS to Anthropic API + GitHub API only.

## Acceptance Criteria

- [ ] Service polls (or receives webhooks for) GitHub issues with the configured trigger label
- [ ] Actor gate: non-writers get a no-op + explanatory comment
- [ ] `audit-spec` runs against issue body verbatim
- [ ] Halt path: structured comment posted listing what's missing; no prompt generation
- [ ] Pass path: prompts generated and executed via existing Executor
- [ ] Output mode A (hand-off): N child issues filed with `claude` label, parent issue gets a summary comment
- [ ] Output mode B (direct): N PRs opened back to consumer repo
- [ ] Idempotent on remove+re-apply of trigger label (clean restart, fresh batch)
- [ ] Per-issue concurrency group prevents parallel runs of the same issue
- [ ] Composes with `k8s-execution.md` (depends on it for runtime)
- [ ] Local Docker + filesystem trigger continue working unchanged
- [ ] `make precommit` passes
- [ ] All existing tests pass, plus new tests for the trigger surface

## Verification

```bash
make precommit
```

Manual verification:

1. Deploy dark-factory + k8s-execution + this trigger surface to a test cluster
2. Configure service to watch a test GitHub repo for `decompose` label
3. File a well-formed issue (constraints + failure modes + AC), apply `decompose` label
   → Observe: audit pass, N child issues filed (mode A) or N PRs opened (mode B)
4. File a malformed issue (missing constraints), apply label
   → Observe: audit halt, comment posted listing missing sections, no prompts generated
5. Edit the malformed issue to add the missing sections, remove + re-apply label
   → Observe: fresh attempt runs, succeeds
6. Apply `decompose` label as a triage-only user
   → Observe: no-op + comment "only writers can trigger"
7. Apply `decompose` to the same issue twice in rapid succession
   → Observe: per-issue concurrency group serializes; second event waits or no-ops cleanly

## Do-Nothing Option

Keep dark-factory as filesystem-trigger-only. Even with k8s-execution shipped, the trigger remains a synced `specs/` + `prompts/` directory and a CLI approval. Cost: no team adoption path; the workflow stays single-developer-shaped even when the runtime is server-side. Anyone wanting to run dark-factory at Seibert (or any team setting) hits the "I don't want to learn a new approval CLI" wall on day one.

## Open Questions

- Webhook vs poll-loop: which is the v1? Webhooks are lower-latency but require a public endpoint; polling is simpler to deploy. Probably poll for v1, webhook as a follow-up.
- Trigger label name: `decompose`, `dark-factory`, `df`, project-configurable? Default + override.
- Output mode default: A (hand-off to vibe-ci) or B (direct PR)? Probably A, since it composes with the existing Seibert pipeline cleanly.
- Cap on issue body size for audit input — match vibe-ci's parser cap (50000 bytes) or pick our own?
- Should generated child issues (mode A) include a back-link to the parent `decompose` issue? Probably yes, in body and via a `decomposed-from-N` label.
- How to handle a trigger issue that already has child issues from a previous attempt? Auto-close them on retry, or leave for human comparison? (Probably leave; matches vibe-ci's "attempts stay open for comparison" pattern.)
