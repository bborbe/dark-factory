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

Pass $ARGUMENTS to the prompt-creator agent.
