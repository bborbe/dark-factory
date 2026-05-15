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

1. Read `docs/prompt-writing.md` "Detail Levels" section (5-level spectrum from very-detailed to very-rough).
2. Run pattern-discovery searches for each surface the prompt will touch:
   - `rg -l 'errors\.Wrapf|fmt\.Errorf' pkg/ internal/` — error wrapping style
   - `rg -l 'http\.NewRequestWithContext' pkg/` — HTTP client construction
   - `rg -l 'counterfeiter:generate' pkg/` — mocking pattern
   - `rg -l 'ginkgo\.' pkg/` — test framework
3. **Pick a detail level deliberately** based on what the searches found:
   - Patterns exist (≥5 matches, consistent) → **Level 3 (Medium)** — reference exemplars, do NOT inline.
   - Patterns missing or inconsistent → **Level 2 (Detailed)** — spelled-out signatures, hinted bodies (no exemplar to point to).
   - Novel structure / spike → Level 4 or 5.
   - External-API reproduction → Level 1.
4. **Default to Level 3** when patterns exist; never silently slide into Level 1 just because full inlining feels safer — that's how author-logic bugs ship.

The prompt's `<context>` MUST list the pattern-reference files (the exemplars discovered in step 2) for every project convention the new code will follow. The auditor's "Pattern collision" check will fail otherwise.

Pass $ARGUMENTS to the prompt-creator agent.
