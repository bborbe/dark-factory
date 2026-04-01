---
description: Audit dark-factory scenario file against Scenario Writing Guide
argument-hint: <scenario-file-path>
---

Invoke the scenario-auditor agent to audit the dark-factory scenario at $ARGUMENTS.

1. Parse scenario path from $ARGUMENTS
   - If no path prefix, prepend `scenarios/`
   - If no `.md` extension, append it
2. Invoke scenario-auditor agent with the scenario path
3. Agent evaluates structure, quality, observability
4. Review findings with severity levels, scores, and recommendations
