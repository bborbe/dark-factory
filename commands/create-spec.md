---
description: Create a dark-factory spec file for a feature or change
argument-hint: <feature-description>
allowed-tools: [Task, Read, Write, Glob, Bash, AskUserQuestion]
---

Invoke the `spec-creator` **agent** through the Task tool (`subagent_type: spec-creator`). It is an agent, not a skill: a Skill-tool call for it fails with `Unknown skill: dark-factory:spec-creator`. `Task` is in `allowed-tools` for exactly this reason.

1. Agent gathers requirements from $ARGUMENTS (or interactively if empty)
2. Agent writes spec file to `specs/<name>.md` (inbox, no number — dark-factory numbers on approve)
3. Agent runs preflight checklist validation

Pass $ARGUMENTS to the spec-creator agent.
