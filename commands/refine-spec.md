---
description: Refine a spec by narrowing scope and splitting adjacent concerns (idea→draft)
argument-hint: "<spec-file-path>"
allowed-tools: [Task]
---

Invoke the spec-refiner agent to refine the dark-factory spec at $ARGUMENTS.

1. Parse spec path from $ARGUMENTS
   - If no path prefix, look in `specs/` then `specs/ideas/`
   - If no `.md` extension, append it
2. Invoke spec-refiner agent with the resolved spec path
3. Agent runs interactive narrowing dialogue, rewrites the spec, splits adjacent concerns into `specs/ideas/` stubs, and transitions status idea→draft
4. Present the agent's output as-is
