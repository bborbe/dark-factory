---
description: Walk a dark-factory spec through real end-to-end verification, then mark complete
argument-hint: <spec-id-or-path>
---

Invoke the spec-verifier agent to verify the dark-factory spec at $ARGUMENTS.

1. Parse spec path from $ARGUMENTS
   - If a number, find `specs/in-progress/0*<n>-*.md`
   - If no path prefix, prepend `specs/in-progress/`
   - If no `.md` extension, append it
2. Invoke spec-verifier agent with the spec path
3. Agent walks the Setup → Action → Expected scenario interactively, gathers evidence, applies anti-pattern rejection rules
4. Agent calls `dark-factory spec complete <id>` only after every Acceptance Criterion is matched to fresh observable evidence
