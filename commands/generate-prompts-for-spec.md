---
description: Generate dark-factory prompt files from an approved spec (non-interactive)
argument-hint: "<spec-file>"
allowed-tools: Task
---

Invoke the `prompt-creator` agent to generate prompts from the approved spec.

**When to reach for this command** (vs. letting the daemon auto-generate): you have `autoGeneratePrompts: false` (the default) and want to trigger generation for a specific approved spec, or you want to re-generate prompts for a spec whose first attempt was rejected. See [docs/running.md § Two ways to generate prompts](../docs/running.md#two-ways-to-generate-prompts-from-an-approved-spec) for the auto-vs-manual tradeoffs.

**Context to pass to the agent:**

- Spec file path: `/workspace/$ARGUMENTS` (the repo root is mounted at `/workspace`).
- Output directory: `/workspace/prompts/` (inbox — not `prompts/in-progress/`, dark-factory moves files on approve).
- Mode: **non-interactive** (daemon-triggered). The agent MUST NOT call `AskUserQuestion`. Any ambiguity that would otherwise prompt the user must be resolved from the spec content alone — if the spec is genuinely under-specified, the agent should still produce the best-effort decomposition and surface the open questions as comments inside the generated prompts (so the human reviewer catches them at audit time).
- The agent has full access to `/workspace/` for reading source files to verify signatures, look up library APIs, and discover existing patterns. It MUST do so before writing requirements — see the agent's own rules.

**Expected output:** the agent decides how many prompts to produce based on the spec (see the agent's `<workflow>` step 7-10 and `Sizing Guide`). Typical: 1 for a config change, 2-3 for a single feature, 4-6 for a major feature. Multi-prompt outputs use `1-`, `2-`, `3-` filename prefixes for execution order. Do NOT post-process or merge the agent's output.

Pass `/workspace/$ARGUMENTS` to the prompt-creator agent and wait for its completion.

**Then run a single audit pass:** for each prompt file the creator wrote, invoke the `prompt-auditor` agent on that path. Collect each audit report. Do NOT retry, rewrite, or merge prompts based on findings — the human reviewer owns that decision.

Return: the creator's summary, followed by the per-prompt audit reports verbatim. Format:

```
## Generated Prompts
<creator's summary — files created, decisions, open questions>

## Audit Findings
### prompts/<filename-1>.md
<auditor's full report>

### prompts/<filename-2>.md
<auditor's full report>
```

This front-loads the audit so the human reviewer sees the findings alongside the prompts without running `/audit-prompt` manually.
