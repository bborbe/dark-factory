---
description: Create a dark-factory spec file for a feature or change
argument-hint: <feature-description>
allowed-tools: [Read, Write, Glob, Bash, AskUserQuestion]
---

Invoke the spec-creator agent to create a dark-factory spec.

1. Agent gathers requirements from $ARGUMENTS (or interactively if empty)
2. Agent writes spec file to `specs/<name>.md` (inbox, no number — dark-factory numbers on approve)
3. Agent runs preflight checklist validation

Pass $ARGUMENTS to the spec-creator agent.
