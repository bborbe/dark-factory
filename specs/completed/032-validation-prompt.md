---
status: completed
approved: "2026-03-16T11:51:54Z"
prompted: "2026-03-16T11:54:44Z"
verifying: "2026-03-16T12:25:52Z"
completed: "2026-03-16T12:37:54Z"
branch: dark-factory/validation-prompt
---

## Summary

- Projects can define quality criteria that the AI agent evaluates after completing work
- The `validationPrompt` config field accepts either inline text or a file path
- If the value resolves to an existing file, its content is loaded as criteria (any format Claude Code can read, `.md` recommended)
- Otherwise the value is used directly as inline criteria text
- Inline: `validationPrompt: "readme.md is updated"` — simple one-liner
- File path: `validationPrompt: docs/dod.md` — detailed checklist loaded from file
- The agent checks each criterion against its changes and reports failures as blockers
- Complements `validationCommand` (machine-judged) with AI-judged evaluation
- Failure results in `partial` status — work was done but quality criteria not fully met

## Problem

`validationCommand` catches build failures and lint errors, but many project conventions cannot be expressed as shell commands: naming patterns, documentation quality, architectural rules, error handling style. Today these are either enforced manually after each prompt completes or missed entirely. With 10+ prompts per spec, manual review of each for convention compliance is slow and error-prone.

## Goal

After this work, every prompt execution evaluates project-specific quality criteria. Criteria can be inline text (for simple checks like "readme.md is updated") or loaded from a file (any format Claude Code can read, `.md` recommended). The agent reads the criteria, inspects its own changes, and reports which items pass or fail. Failed criteria result in `partial` status with clear blockers, allowing the human to decide whether to fix or accept.

## Non-Goals

- Replacing `validationCommand` — both coexist, `validationCommand` is deterministic, `validationPrompt` is AI-judged
- Enforcing a specific format for the criteria file — plain markdown, checklist recommended but not required
- Automatic retry or fix when criteria fail — human decides next step
- Per-prompt criteria overrides — one file applies to all prompts in the project

## Desired Behavior

1. When `validationPrompt` is configured, the value is resolved: if it points to an existing file, the file content is loaded; otherwise the value is used as inline text
2. The agent evaluates each criterion against its changes after `validationCommand` passes — a build failure skips criteria evaluation
3. If any criterion is not met, the completion report shows `partial` status with unmet criteria listed as blockers
4. If all criteria are met, no effect on status — `success` stays `success`
5. If the value is a file path and the file does not exist at execution time, a warning is logged and execution continues without evaluation
6. If `validationPrompt` is not set, no evaluation occurs — zero overhead

## Constraints

- Config field name is `validationPrompt` — parallel to existing `validationCommand`
- If the value resolves to an existing file (relative to project root), its content is loaded; otherwise the value is inline text
- File can be any format Claude Code understands (`.md` recommended)
- Criteria evaluation runs within the same agent turn — no additional API calls or external requests beyond the completion report
- `partial` status (not `failed`) — the work was done, validation passed, but criteria not fully met
- Assumes the AI agent can reliably evaluate clearly written, binary criteria — vague criteria (e.g. "code should be clean") may produce unreliable results
- The criteria file is expected to be version-controlled alongside the project

## Security

- The criteria file is read from the local filesystem — no external fetch
- File path must be within the project directory — no traversal outside project root
- Criteria file content is injected into the agent prompt — a malicious criteria file could attempt prompt injection; mitigated by the file being project-local and version-controlled

## Failure Modes

| Trigger | Expected Behavior | Recovery |
|---------|-------------------|----------|
| Criteria file does not exist | Log warning, continue without evaluation | Create the file or remove config |
| Criteria file is empty | No criteria to evaluate, continue normally | Add criteria to file |
| Criteria too vague for AI | Agent may misjudge — reports partial with unclear blockers | Rewrite criteria to be more specific |
| Agent ignores criteria entirely | Completion report shows success but criteria were not checked | Improve prompt suffix instructions |
| File path outside project root | Reject at config validation | Fix path in config |

## Do-Nothing Option

Project conventions continue to be enforced manually. Each completed prompt requires human review for naming, documentation, and architectural patterns. With 10+ prompts per feature, this adds 30+ minutes of review time and conventions are frequently missed, requiring follow-up fix prompts. As prompt count grows, the review burden scales linearly — this is not sustainable.

## Acceptance Criteria

- [ ] Value resolving to an existing file loads its content as criteria (`validationPrompt: docs/dod.md`)
- [ ] Value not resolving to a file is used directly as inline criteria text (`validationPrompt: "readme.md is updated"`)
- [ ] Unmet criteria result in `partial` status with blockers listing the failures
- [ ] Met criteria have no effect on status
- [ ] No config field means no criteria evaluation and zero overhead
- [ ] Absolute paths and paths traversing outside the project root are rejected at config validation with an error message
- [ ] Works alongside `validationCommand` — both can be active simultaneously
- [ ] `make precommit` passes

## Verification

Run `make precommit` — must pass.

Manual verification:
1. Set `validationPrompt: "readme.md is updated"` — run a prompt → expected: completion report includes criteria evaluation, `partial` if readme not updated
2. Set `validationPrompt: docs/dod.md` with a checklist file — run a prompt → expected: same behavior, criteria loaded from file
3. Delete `docs/dod.md`, keep config pointing to it — run a prompt → expected: warning in logs, prompt completes normally (`success` not `partial`)
4. Set `validationPrompt: /etc/passwd` — expected: config validation rejects with error before execution
