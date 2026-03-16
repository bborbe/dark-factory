# Architecture & Execution Flow

How dark-factory processes specs and prompts, what runs where, and how each component fits together.

## System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│  HOST (your machine)                                            │
│                                                                 │
│  ┌─────────────┐    ┌──────────────┐    ┌───────────────────┐  │
│  │ Claude Code  │───>│ dark-factory │───>│ Docker Container  │  │
│  │ (interactive)│    │   (daemon)   │    │  (YOLO agent)     │  │
│  └─────────────┘    └──────────────┘    └───────────────────┘  │
│                            │                      │             │
│  You write specs/prompts   │ Orchestrates:        │ Implements: │
│  and approve them          │ - git ops            │ - code      │
│  via Claude Code           │ - container lifecycle│ - tests     │
│                            │ - status tracking    │ - docs      │
│                            │ - notifications      │             │
└─────────────────────────────────────────────────────────────────┘
```

**Key boundary:** The YOLO container has NO git access. All git operations (branch, commit, push, PR) happen on the host via dark-factory.

## End-to-End Flow

### Phase 1: Spec → Prompts (Host, Interactive)

```
Human + Claude Code                    dark-factory daemon
       │                                      │
  1. Write spec                               │
  2. Audit spec (/audit-spec)                 │
  3. Approve spec ──────────────────────>  4. Auto-generate prompts
       │                                      │
  5. Review generated prompts                 │
  6. Audit prompts (/audit-prompt)            │
  7. Approve prompts ──────────────────>  8. Queue for execution
```

### Phase 2: Prompt Execution (Host + Container)

This is the core loop. For each queued prompt, dark-factory runs these steps:

```
 STEP  WHERE        WHAT HAPPENS
 ────  ─────        ────────────
  1    Host         Git fetch + merge origin/default
  2    Host         Load prompt file, read body content
  3    Host         Assemble final prompt (see "Prompt Assembly" below)
  4    Host         Setup workflow (branch switch or clone)
  5    Host         Start Docker container with assembled prompt
  6    Container    YOLO agent reads prompt and implements changes
  7    Container    Agent runs validationCommand (e.g., make precommit)
  8    Container    Agent self-evaluates against validationPrompt criteria
  9    Container    Agent writes DARK-FACTORY-REPORT completion marker
 10    Host         Parse completion report from container logs
 11    Host         Validate report (success/partial/failed)
 12    Host         Move prompt to completed/
 13    Host         Git commit + push (or PR creation)
 14    Host         Auto-complete linked spec if all prompts done
 15    Host         Notify (Telegram/Discord) if attention needed
```

### Phase 3: Post-Execution (Host)

```
Direct workflow:     commit → tag → push
Branch workflow:     commit → push branch → (last prompt?) → merge to default → release
PR workflow:         commit → push branch → create/update PR → (autoMerge?) → merge
```

## Prompt Assembly

Dark-factory assembles the final prompt the agent receives by appending sections to the original prompt body. This happens on the host **before** the container starts.

```
┌──────────────────────────────────┐
│  Original prompt body            │  ← What the human/generator wrote
│  (from prompts/in-progress/)     │
├──────────────────────────────────┤
│  Completion Report Suffix        │  ← Always appended
│  (DARK-FACTORY-REPORT format)    │    Tells agent how to report results
├──────────────────────────────────┤
│  Changelog Suffix                │  ← Only if CHANGELOG.md exists
│  (write ## Unreleased entry)     │    Tells agent to update changelog
├──────────────────────────────────┤
│  validationCommand Suffix        │  ← Only if validationCommand is set
│  (e.g., "run make precommit")   │    Overrides <verification> section
│                                  │    Agent runs this command
├──────────────────────────────────┤
│  validationPrompt Suffix         │  ← Only if validationPrompt is set
│  (AI quality criteria)           │    Agent self-reviews its own changes
│                                  │    against these criteria AFTER
│                                  │    validationCommand passes
└──────────────────────────────────┘
```

### Execution order inside the container

```
1. Agent implements all requirements from the prompt body
2. Agent runs validationCommand (e.g., make precommit)
   └── If exit code != 0 → report "failed", stop
3. Agent reads validationPrompt criteria
   └── Checks each criterion against its own changes
   └── Unmet criteria → report "partial" with blockers
   └── All met → report "success"
4. Agent writes DARK-FACTORY-REPORT with status + summary
```

**Important:** `validationCommand` is a shell command the agent runs (machine-judged, exit code). `validationPrompt` is text the agent reads and evaluates its work against (AI-judged, self-review). They are complementary — validationCommand catches build/lint failures, validationPrompt catches quality/completeness gaps.

## Status Lifecycle

### Prompt Status

```
created → approved → executing → completed
                         │            │
                         └── failed   └── (moved to completed/)
                         │
                         └── partial (validationPrompt criteria unmet)
```

### Spec Status

```
draft → approved → prompted → verifying → completed
                       │           │
                       │           └── (human runs dark-factory spec complete)
                       └── (all prompts auto-generated)
```

## Directory Structure

```
project/
├── .dark-factory.yaml          # Configuration
├── .dark-factory.lock          # Instance lock (flock-based)
├── CLAUDE.md                   # Agent context (read by YOLO container)
├── CHANGELOG.md                # Version history (optional)
├── docs/
│   └── dod.md                  # Definition of Done (validationPrompt target)
├── prompts/
│   ├── my-change.md            # Inbox (status: created)
│   ├── in-progress/
│   │   └── 001-my-change.md    # Queue (status: approved/executing)
│   ├── completed/
│   │   └── 001-my-change.md    # Done (status: completed)
│   └── log/
│       └── 001-my-change.log   # Execution log + completion report
├── specs/
│   ├── my-feature.md           # Inbox (status: draft)
│   ├── in-progress/
│   │   └── 033-my-feature.md   # Active (status: approved/prompted/verifying)
│   ├── completed/
│   │   └── 033-my-feature.md   # Done (status: completed)
│   └── log/
└── scenarios/                  # End-to-end verification scenarios
```

## What Runs Where

| Component | Runs on | Responsibility |
|-----------|---------|---------------|
| Claude Code | Host (interactive) | Write specs/prompts, audit, approve |
| dark-factory daemon | Host (background) | Orchestrate: watch queue, manage containers, git ops, notifications |
| dark-factory CLI | Host (interactive) | `approve`, `retry`, `status`, `list`, `complete` |
| YOLO container | Docker (isolated) | Implement code changes, run tests, self-evaluate quality |
| Git operations | Host only | Fetch, branch, commit, push, PR create/merge |
| validationCommand | Inside container | Agent runs shell command (e.g., `make precommit`) |
| validationPrompt | Inside container | Agent self-reviews work against quality criteria |
| Notifications | Host | Telegram/Discord via HTTPS from daemon |

## Workflow Modes

| `pr` | `worktree` | Git behavior | Container sees |
|------|-----------|-------------|----------------|
| `false` | `false` | Commit to current branch in-place | Original repo (read-write) |
| `false` | `true` | Clone repo, commit to branch | Clone directory (read-write) |
| `true` | `false` | Commit in-place, create PR | Original repo (read-write) |
| `true` | `true` | Clone repo, create PR | Clone directory (read-write) |

With `branch` set on prompt/spec: execution happens on that branch. Multiple prompts on the same branch see cumulative changes.

## Configuration Reference

See [configuration.md](configuration.md) for all config fields and examples.
