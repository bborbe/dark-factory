---
description: Read all dark-factory guides for context before working
allowed-tools: [Read, Glob]
---

Read all dark-factory documentation to build full context. Use this before writing specs, prompts, or making dark-factory decisions.

## Step 1: Read dark-factory docs

Glob `~/.claude/plugins/marketplaces/dark-factory/docs/*.md` and Read every file returned. These cover spec/prompt structure, lifecycle, running, scenarios, CLAUDE.md guidance, init, YOLO setup, configuration, DoD, and architecture.

## Step 2: Read project docs

Glob `docs/*.md` from the current working directory. If results returned, Read each. If empty, skip (project has no domain docs yet).

These contain project-specific domain knowledge (Kafka schemas, task formats, controller design) that specs/prompts should reference instead of inlining.

## Step 3: Index coding plugin docs (don't read all)

Glob `~/.claude/plugins/marketplaces/coding/docs/*.md` and list filenames only — do NOT read every file (50+ files, expensive). These are generic coding patterns (Go, Python, testing, etc.) available to YOLO agents in the container at `/home/node/.claude/plugins/marketplaces/coding/docs/`.

Read individual files on-demand when a spec/prompt needs the matching pattern.

## Step 4: Summarize

Report:
- **Three-flow decision pointer** — point the user at the canonical decision doc; this is the most load-bearing decision a user makes, so route them to one source instead of any restatement here.

  **Canonical source: [`docs/choosing-a-flow.md`](../docs/choosing-a-flow.md). Point users at it — do NOT restate the decision in this summary; restating is exactly what drove historic drift across CLAUDE.md, spec-writing.md, and prompt-writing.md.** Headline reason for prompts/specs is **safe unattended execution** in a YOLO Claude container; pick direct when ceremony adds no value (markdown / docs / configs / small scripts).
- **Dark-factory workflow rules** — key lifecycle/CLI rules learned
- **Project docs available** — filename + one-line topic per file
- **Coding plugin docs available** — filenames only (grouped by language if helpful)
- **Confirm readiness to work**
