---
description: Audit dark-factory prompt file against Prompt Definition of Done
argument-hint: <prompt-file-path>
---

Invoke the prompt-auditor agent to audit the dark-factory prompt at $ARGUMENTS.

1. Parse prompt path from $ARGUMENTS
   - If no path prefix, prepend `prompts/`
   - If no `.md` extension, append it
2. Invoke prompt-auditor agent with the prompt path
3. Agent evaluates structure, code references, quality
4. Review findings with severity levels, scores, and recommendations
5. Check the project's declared pre-approve gate
   - Read the project `CLAUDE.md` for a "Release Checklist" section or any
     "before `dark-factory prompt approve`" instruction, plus any doc it links
   - Report gate status next to the audit verdict: **satisfied**, **not yet
     walked**, or **none declared**
   - If a gate is declared and not yet walked, say so explicitly — a clean
     audit verdict is NOT approval readiness. On `autoRelease: true` projects
     approval ships the change with no second checkpoint, so the gate is the
     last opportunity to catch an unvalidated build
