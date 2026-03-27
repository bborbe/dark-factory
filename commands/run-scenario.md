---
description: Run dark-factory scenario(s) as manual verification checklists
argument-hint: [scenario-name-or-number]
allowed-tools: [Read, Glob, Bash, AskUserQuestion]
---

Run dark-factory scenarios from `scenarios/` directory.

1. If $ARGUMENTS is provided:
   - Find matching scenario file (by number prefix or name)
   - If no path prefix, search `scenarios/`
   - If no `.md` extension, append it
2. If no $ARGUMENTS:
   - List all scenarios with `status: active` from `scenarios/`
   - Show summary table (number, title, description)
   - Ask user which to run (or "all")
3. For each selected scenario:
   - Read the scenario file
   - Walk through Setup steps — ask user to confirm preconditions
   - Show Action steps — ask user to execute each
   - Show Expected checklist — ask user to verify each outcome
   - Report pass/fail per scenario
4. If running multiple: show summary table at the end (scenario → pass/fail)
