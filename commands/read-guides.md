---
description: Read all dark-factory guides for context before working
allowed-tools: [Read, Glob]
---

Read all dark-factory documentation to build full context. Use this before writing specs, prompts, or making dark-factory decisions.

## Step 1: Read dark-factory docs

Read all `docs/*.md` files from the dark-factory installation directory:

1. `docs/spec-writing.md` — Spec structure, lifecycle, preflight checklist
2. `docs/prompt-writing.md` — Prompt structure, writing rules, Definition of Done
3. `docs/documentation.md` — Where knowledge belongs (spec vs prompt vs project docs vs yolo docs)
4. `docs/running.md` — Daemon/run modes, monitoring, CLI reference
5. `docs/scenario-writing.md` — Scenario format, writing rules
6. `docs/claude-md-guide.md` — CLAUDE.md structure for dark-factory projects
7. `docs/init-project.md` — Project setup guide
8. `docs/yolo-container-setup.md` — YOLO container config

## Step 2: Read project docs

List and read all `docs/*.md` files in the current project directory (if `docs/` exists). These are project-specific domain docs that specs and prompts should reference.

```
Glob: docs/*.md
```

Read each file found. These docs contain domain knowledge (Kafka schemas, task file formats, controller design, etc.) that prompts should reference instead of inlining.

## Step 3: Read yolo docs index

Read `~/.claude-yolo/docs/README.md` to understand which generic coding pattern docs are available. These are mounted in the YOLO container at `/home/node/.claude/docs/` and prompts should reference them for reusable patterns.

```
Read: ~/.claude-yolo/docs/README.md
```

After reading all three sources, summarize:
- Dark-factory workflow rules learned
- Project docs available (list filenames and topics)
- Yolo docs available (list filenames and topics)
- Confirm readiness to work
