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

## Scenarios — never default, never split

Scenarios are E2E tests at the top of the test pyramid: slow, brittle, expensive. Most specs are satisfied by unit + integration tests written as part of the implementation prompt — do NOT add scenario authoring as a default decomposition step.

**Rule 1: Do not add a scenario the spec did not request.**
- If the spec's Acceptance Criteria does NOT explicitly require a new scenario, do NOT generate scenario authoring requirements speculatively. The prompt-level tests already cover the spec's ACs.
- "Touches an integration seam" is NOT a trigger by itself. The spec author already applied the four-condition test from `docs/scenario-writing.md`. If they did not list a scenario AC, none is needed.

**Rule 2: When the spec DOES require a scenario, inline it — do NOT split into a separate prompt.**
- Writing a scenario is writing a markdown checklist. It does not need its own YOLO container run.
- Add the scenario authoring as a numbered requirement step in the implementation prompt that touches the relevant code (typically the last step, before the verification command).
- Example requirement: "Write `scenarios/NNN-<slug>.md` with status `draft`. Setup: ... Action: ... Expected: ..." — full scenario body inlined.
- One Docker container run produces both the code change AND the scenario file.

**Rule 3: Split a scenario into its own prompt only as a rare exception.**
- Justified only when the scenario's checklist genuinely cannot fit alongside the code change in one prompt without losing readability AND the scenario itself requires distinct setup the implementation prompt's container won't have.
- This is unusual. Default to inlining.

If unsure between inline and split, inline.

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
