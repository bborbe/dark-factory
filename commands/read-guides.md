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
- **Three-flow decision table** — emit verbatim, this is the most load-bearing decision a user makes. The headline reason to use prompts/specs is **safe unattended execution** in a YOLO Claude container with permission checks disabled — queue work and step away. The split is about *what artifact deserves to be committed alongside the change*, not size or complexity:

  | Kind of change | Flow | What gets committed |
  |----------------|------|---------------------|
  | Doc / config / yaml — no code | **Direct** — edit + commit yourself | Just the diff |
  | Code change of any size | **Prompt** — write a prompt, audit, approve, daemon executes | Prompt + diff (technical "how" record) |
  | Feature delivering business value | **Spec → prompts** — write spec, audit, approve, daemon auto-generates prompts, audit each, approve, daemon executes | Spec + prompts + diff (business "why" + technical "how") |

  Decide by: **Is code changing?** No → direct. Yes → prompt or spec. **Is there a business-level "why" that deserves its own document?** No → prompt. Yes → spec.

  The split between prompt and spec is **business-why vs technical-how**, NOT big vs small. A 50-prompt mechanical refactor stays prompts. A 1-prompt user-visible feature may still warrant a spec.

  See `architecture-flow.md` "Choosing a Flow" for the canonical version.
- **Dark-factory workflow rules** — key lifecycle/CLI rules learned
- **Project docs available** — filename + one-line topic per file
- **Coding plugin docs available** — filenames only (grouped by language if helpful)
- **Confirm readiness to work**
