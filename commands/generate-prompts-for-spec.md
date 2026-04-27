---
description: Generate dark-factory prompt files from an approved spec (non-interactive)
argument-hint: <spec-file>
allowed-tools: [Read, Write, Glob, Bash, Task]
---

Read the spec file at `/workspace/$ARGUMENTS`.

Extract the spec number from the filename (e.g. `020` from `020-auto-prompt-generation.md`).

Read 3-5 recent completed prompts from `/workspace/prompts/completed/` (pick the highest-numbered ones) to understand the prompt style, XML tag structure, and level of detail expected.

Read the spec carefully. Identify:
- Desired Behaviors (numbered list) — these drive decomposition
- Constraints — must be repeated in every prompt
- Acceptance Criteria — used in verification sections
- Failure Modes table — each trigger must map to a requirement step in at least one prompt (error handling, timeout, fallback, recovery)
- Security section — include relevant checks in requirements where applicable

**Discover relevant coding guides** using the coding-guidelines-finder and project-docs-finder agents:

Spawn both agents in parallel using Task:
1. `coding:coding-guidelines-finder` — finds relevant guides from `~/.claude/plugins/marketplaces/coding/docs/`
2. `coding:project-docs-finder` — finds relevant docs from `/workspace/docs/`

Pass the spec's objective and key behaviors as the task description.

If either agent finds no results, that's fine — continue with results from the other.
If the coding plugin docs directory doesn't exist, STOP and report: "coding plugin not installed. Install it before generating prompts."

For each matching guide, reference it in the prompt's `<context>` section:
```
Read `go-http-handler-refactoring-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
```
Do NOT inline patterns that are already documented — the YOLO agent will read the guide itself.

Decompose the spec into 2–6 prompt files. Group coupled behaviors that cannot be verified independently into the same prompt. Sequence them so each prompt's postconditions are the next prompt's preconditions.

For each prompt, write a file to `/workspace/prompts/<slug>.md`. Do NOT add number prefixes — dark-factory assigns numbers on approve. If prompts must be executed in a specific order, prefix with `1-`, `2-`, `3-` for alphabetical sorting (e.g. `1-spec-020-model.md`, `2-spec-020-routing.md`).

Each file must start with this exact frontmatter:
```
---
spec: ["NNN"]
status: draft
created: "<current UTC timestamp in ISO8601 format>"
---
```
where `NNN` is the zero-padded spec number (e.g. `"020"`).

After the frontmatter, write the prompt body using XML tags:

```xml
<summary>
TL;DR — 5-10 bullet points describing WHAT this prompt achieves, not HOW.
Written for the human reviewer, not the agent. No file paths, no struct names, no function signatures.
Each bullet should describe an observable outcome or behavior change.

BAD (too technical — describes implementation):
- Adds `validationCommand` field to `Config` in `pkg/config/config.go`
- Adds `ValidationSuffix(cmd string) string` to `pkg/report/suffix.go`
- Threads `validationCommand` through `CreateProcessor` in `pkg/factory/factory.go`

GOOD (describes what changes for the user/system):
- Projects can configure a validation command that applies to all prompts
- The configured command is injected into every prompt before execution
- The agent uses the command's exit code as the authoritative success/failure signal
- Empty config disables injection — prompts fall back to their own verification
- Existing prompts continue to work unchanged
</summary>

<objective>
WHAT to build and WHY (1-3 sentences). State the end state, not the steps.
</objective>

<context>
Read CLAUDE.md for project conventions.
List specific files to read before making changes.
</context>

<requirements>
Numbered, specific, unambiguous steps. Include exact file paths, function signatures, and import paths.
</requirements>

<constraints>
- Repeat relevant constraints from the spec — the agent has no memory between prompts
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
Any additional verification commands.
</verification>
```

Rules:
- Specificity over brevity — longer prompts are almost always better
- Always include exact file paths and function signatures
- Always copy constraints from the spec into each prompt
- Do NOT add frontmatter fields beyond spec/status/created — dark-factory adds the rest
- Do NOT place prompts in `/workspace/prompts/in-progress/` — inbox only
- The `spec` field must be a YAML array: `spec: ["020"]` not `spec: "020"`
