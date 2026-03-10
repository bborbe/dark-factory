---
description: Audit dark-factory spec file against preflight checklist and quality criteria
argument-hint: <spec-file-path>
---

Invoke the spec-auditor agent to audit the dark-factory spec at $ARGUMENTS.

1. Parse spec path from $ARGUMENTS
   - If no path prefix, prepend `specs/`
   - If no `.md` extension, append it
2. Invoke spec-auditor agent with the spec path
3. Agent evaluates preflight checklist, behavioral level, section completeness
4. Review findings with severity levels, scores, and recommendations
