---
description: Create a dark-factory spec file for a feature or change
argument-hint: <feature-description>
allowed-tools: [Read, Write, Glob, Bash, AskUserQuestion]
---

Invoke the spec-creator agent to create a dark-factory spec.

1. Agent gathers requirements from $ARGUMENTS (or interactively if empty)
2. Agent determines next spec number from existing `specs/` files
3. Agent writes spec file following the dark-factory spec template
4. Agent runs preflight checklist validation

Pass $ARGUMENTS to the spec-creator agent.
