# Bug Workflow

How to file, triage, fix, and verify bugs in dark-factory.

**TL;DR:** Bugs are specs with `kind: bug`. Same lifecycle, same directory, same tooling — just one extra frontmatter field. No new constructs.

## Why specs, not a new artifact

A bug report is a behavioral contract: "the system DOES X, but it SHOULD do Y." That's exactly what specs already are (see [spec-writing.md](rules/spec-writing.md)). The lifecycle (idea → draft → approved → prompted → verifying → completed) maps cleanly onto bug triage:

| Lifecycle | Bug meaning |
|-----------|-------------|
| `idea` | Reported, not yet triaged |
| `draft` | Triaged, root cause hypothesis written, ready for review |
| `approved` | Fix scoped, daemon generates fix prompts |
| `prompted` | Fix prompts queued/executing |
| `verifying` | Fix landed, awaiting reproduction-cannot-reproduce confirmation |
| `completed` | Bug verified gone |

Don't conflate category (bug/feature) with lifecycle (`status:`). They're orthogonal — a bug should still progress through all six states.

## When to file as a bug

| Symptom | File a bug? |
|---------|-------------|
| Documented behavior doesn't match reality | Yes |
| Config flag silently ignored | Yes |
| Crash, panic, deadlock, infinite loop | Yes |
| Race condition or data corruption | Yes |
| User-visible error message that's wrong/misleading | Yes |
| "Could be cleaner" — no functional defect | No (feature/refactor spec) |
| New capability needed | No (feature spec) |
| Doc typo, broken link | No (PR directly, no spec) |

## Filing a bug

### Location

Same as feature specs:

| Status | Directory |
|--------|-----------|
| `idea` (rough report, not triaged) | `specs/ideas/` |
| `draft` (triaged, ready for approval) | `specs/` |

Lowercase-kebab-case filename. Optional `bug-` prefix for visual grouping (`bug-<short-description>.md`) — purely a sort convenience, not required.

### Frontmatter

```yaml
---
status: idea
kind: bug
---
```

`kind: bug` is the only category marker. Optional — defaults to `feature` when absent. Use it so spec-list/audit can filter bugs from feature backlog.

### Sections (bug-specific)

Standard spec sections (Summary/Problem/Goal/Constraints/Acceptance Criteria) all apply, plus:

#### `Reproduction` (mandatory)

Exact steps to reproduce. Must include:

- The smallest config that exhibits the bug
- The exact command sequence
- Observed evidence (log lines, git state, file contents) — copy verbatim, don't paraphrase
- The dark-factory version (`dark-factory --version`)

A bug spec without reproduction steps is unactionable. If you can't reproduce, file it as `idea` and ask in the Open Questions section for repro.

#### `Expected vs Actual`

Two columns or two paragraphs. State expected per documented behavior (cite the doc — `workflows.md:34`), then state actual.

#### `Workaround` (optional)

Temporary mitigation users can apply until the fix lands. Distinct from the fix itself — workarounds are user-side, fixes are code-side.

#### `Why this is a bug`

If non-obvious, explain why this contradicts documented behavior, an invariant, or a reasonable user expectation. Cite the source (doc line, code comment, prior spec).

### Sections to skip on bug specs

These belong to feature specs and don't apply (or apply trivially):

- `Non-goals` (a bug fix has no scope creep — it fixes the bug)
- `Do-Nothing Option` (cost of not fixing = bug stays; usually obvious)
- `Security / Abuse` (only if the bug has security implications)

## Triage (idea → draft)

Move from `idea` to `draft` when:

- Reproduction is confirmed by someone other than the reporter
- Root cause has a working hypothesis (named the suspect file/function or config path)
- Acceptance criteria are binary and testable
- Failure modes that the fix MUST cover are enumerated

Triage is a research step, not a fix step. Don't write the fix prompt yet — let the daemon generate it after approval.

## Approval

Same as feature specs:

```bash
dark-factory spec approve <bug-name>
```

The daemon generates fix prompts from the spec. Each prompt should be atomic — one observable behavior change per prompt — same rules as feature prompts ([prompt-writing.md](rules/prompt-writing.md)).

## Verification (the critical bug-specific step)

Bug verification is **not** the same as feature verification. The mandatory check:

> **Run the original reproduction steps. Confirm the bug no longer reproduces.**

Tests passing ≠ bug fixed. The repro from the spec must be replayed against the deployed/built artifact and produce the documented expected behavior.

Use `/dark-factory:verify-spec <id>` ([spec-verification.md](spec-verification.md)) — the agent walks Setup → Action → Expected and refuses completion on inspection-only evidence. Specifically for bug specs:

| Evidence | Acceptable? |
|----------|-------------|
| "Tests pass" | No — tests prove what the author thought, not the bug is gone |
| "Code looks right" | No — inspection is not verification |
| "Ran the repro from the spec; expected behavior observed" | Yes |
| Log line / metric / git state showing the new behavior at runtime | Yes |
| Before/after comparison: same input, old binary fails, new binary passes | Yes |

If the bug describes a runtime symptom (silent fail, wrong commit destination, missing PR), the verification MUST exercise the runtime path. Stub/mock-only tests do not satisfy this.

## Failed-fix handling

If verification fails:

1. Do NOT mark `completed`
2. Re-run reproduction and capture fresh evidence
3. If the bug still reproduces, the spec stays in `verifying` and a follow-up prompt is needed (re-open with a new prompt, do not edit the spec)
4. If the bug NOW reproduces in a different shape (regression introduced by the fix), file a new bug spec — don't reuse the old one

## Examples

### Good bug filename

```
specs/ideas/bug-autorelease-overrides-pr-workflow.md
specs/ideas/bug-stuck-container-not-killed-on-cancel.md
specs/bug-prompt-status-flips-completed-on-failure.md
```

### Anti-patterns

- `specs/bugs/<name>.md` — don't create a separate folder; daemon/CLI doesn't know about it
- `status: bug` in frontmatter — conflates category with lifecycle
- Filing without reproduction — unactionable, becomes stale
- Filing the **fix** as a prompt without a spec — skips triage, commits to one approach prematurely

## Quick checklist before filing

- [ ] Reproduction steps are verbatim, not summarized
- [ ] Expected behavior cites a documented source (doc line, prior spec, code comment)
- [ ] Actual behavior includes raw log/git/output evidence
- [ ] Frontmatter has `kind: bug`
- [ ] Filename is lowercase-kebab-case, optionally `bug-` prefixed
- [ ] Status is `idea` (rough) or `draft` (triaged + ready for approval)
- [ ] No fix details — that's the prompts' job after approval

## See also

- [spec-writing.md](rules/spec-writing.md) — full spec structure and rules
- [spec-verification.md](spec-verification.md) — verification procedure (applies to both feature and bug specs)
- [prompt-writing.md](rules/prompt-writing.md) — how fix prompts are structured
- [running.md](running.md) — daemon, CLI commands, status checks
