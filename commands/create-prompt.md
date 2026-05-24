---
description: Create dark-factory prompt files from a spec or task description
argument-hint: <spec-file-or-task-description>
allowed-tools: [Read, Write, Glob, Bash, AskUserQuestion]
---

Invoke the prompt-creator agent to create dark-factory prompts.

If $ARGUMENTS points to a spec file (contains `.md` or starts with `specs/`):
- Agent reads the spec and decomposes into 2-6 prompts
- Prompts are written to `prompts/` directory

If $ARGUMENTS is a task description:
- Agent gathers requirements interactively
- Creates 1-3 focused prompts

**Before writing requirements, the agent MUST:**

1. Read `docs/rules/prompt-writing.md` "Detail Levels" section (5-level spectrum from very-detailed to very-rough).
2. Run pattern-discovery searches for each surface the prompt will touch:
   - `rg -l 'errors\.Wrapf|fmt\.Errorf' pkg/ internal/` — error wrapping style
   - `rg -l 'http\.NewRequestWithContext' pkg/` — HTTP client construction
   - `rg -l 'counterfeiter:generate' pkg/` — mocking pattern
   - `rg -l 'ginkgo\.' pkg/` — test framework
3. **Pick a detail level deliberately based on a 2-axis decision** — *do patterns exist?* × *is the structure novel?*:
   
   |  | **Patterns exist (≥5 consistent matches)** | **Patterns missing / inconsistent** |
   |---|---|---|
   | **Translation work** (apply existing shape to new feature) | **Level 3** — reference exemplars, no inlining | **Level 2** — spelled-out signatures, hinted bodies (no exemplar to point to). Promote to Level 3 in a future prompt once the pattern is documented. |
   | **Novel structure** (agent must invent shape) | **Level 3 or 2** — exemplars constrain style; agent invents structure | **Level 4 or 5** — let the agent explore. Promote to Level 3 once the pattern is proven AND documented in `project/docs/`. |
   
   Special case: **External-API reproduction** (must match a published reference line-for-line) → **Level 1**, link the source.
4. **Default to Level 3 only when patterns exist.** Never silently fall through to Level 3 because it's "the default" — that's the trap that produces fake references to files that don't demonstrate the claimed pattern. If Step 2 returned no matches, you're at Level 2 (translation) or 4/5 (novel), never Level 3.
5. **Never silently slide into Level 1** just because full inlining feels safer — that's how author-logic bugs ship.

The prompt's `<context>` MUST list the pattern-reference files (the exemplars discovered in step 2) for every project convention the new code will follow. The auditor's "Pattern collision" check will fail otherwise.

Pass $ARGUMENTS to the prompt-creator agent.
