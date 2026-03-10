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
