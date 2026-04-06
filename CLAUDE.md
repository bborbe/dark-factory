# CLAUDE.md

## Dark Factory Workflow

**Never code directly.** All code changes go through the dark-factory pipeline.

### What to do

1. **Assess the change size:**

| Change | Action |
|--------|--------|
| Simple fix, config change, 1-2 files | Write a prompt → [docs/prompt-writing.md](docs/prompt-writing.md) |
| Multi-prompt feature, unclear edges, shared interfaces | Write a spec first → [docs/spec-writing.md](docs/spec-writing.md) |

2. **Read the relevant guide before starting** — every time, not from memory:
   - Architecture & flow → read [docs/architecture-flow.md](docs/architecture-flow.md)
   - Writing a spec → read [docs/spec-writing.md](docs/spec-writing.md)
   - Writing prompts → read [docs/prompt-writing.md](docs/prompt-writing.md)
   - Configuration → read [docs/configuration.md](docs/configuration.md)
   - Running prompts → read [docs/running.md](docs/running.md)
   - Running scenarios → read [docs/scenario-writing.md](docs/scenario-writing.md)

3. **Follow the guide step by step.** Do not skip audit steps.

4. **After completing a spec or major refactor**, run the relevant scenario to verify end-to-end behavior. Scenarios live in `scenarios/` (numbered by complexity). Always use a temp copy of the sandbox — never run against the original.

### Key rules (details in the guides)

- Prompts go to **`prompts/`** (inbox) — never to `prompts/in-progress/` or `prompts/completed/`
- Specs go to **`specs/`** (inbox) — never to `specs/in-progress/` or `specs/completed/`
- Never number filenames — dark-factory assigns numbers on approve
- Never manually edit frontmatter status — use CLI:
  - `dark-factory prompt approve <name>` — queue prompt for execution
  - `dark-factory prompt retry` — re-queue failed prompts
  - `dark-factory spec approve <name>` — approve spec (triggers prompt generation)
  - `dark-factory spec complete <name>` — mark verified spec as completed
  - `dark-factory status` — show combined status
  - `dark-factory prompt list` / `dark-factory spec list` — list with status
- Always audit before approving (`/audit-prompt`, `/audit-spec`)
- **Never approve or run dark-factory without explicit user confirmation** — present the prompt, wait for user to say "approve" or "run"
- **Start daemon in background** — use Bash tool with `run_in_background: true`: `dark-factory daemon` (not foreground, not detached with `&`)

## Plugin Release Checklist

**When to release:** Any change to `commands/`, `agents/`, `docs/`, or `skills/` requires a plugin version bump — these files ship as part of the plugin.

**How to release:**

1. Pick the next version: increment minor from the latest `CHANGELOG.md` entry (e.g. v0.103.0 → v0.104.0)
2. Update **all four files** — version string must be identical everywhere (without `v` prefix in JSON):
   - `CHANGELOG.md` — add new `## vX.Y.Z` section at top with all changes (binary + plugin)
   - `.claude-plugin/plugin.json` — `"version": "X.Y.Z"`
   - `.claude-plugin/marketplace.json` — `"version": "X.Y.Z"` in **both** `metadata` and `plugins[0]`
3. Commit: `release plugin vX.Y.Z: <summary>`
4. Push: `git push`

**Common mistakes:**
- Forgetting `.claude-plugin/` files (plugin stays at old version)
- Creating a separate "Plugin vX" changelog section (wrong — one version for everything)
- Using different versions in the 3 JSON fields (must all match)
- Not including binary changes (fetch timeout, lock fix) in the changelog when they're uncommitted

## Development Standards

This project follows the [coding-guidelines](https://github.com/bborbe/coding-guidelines).

### Key Reference Guides

- **[go-architecture-patterns.md](~/Documents/workspaces/coding-guidelines/go-architecture-patterns.md)** - Interface → Constructor → Struct → Method
- **[go-testing-guide.md](~/Documents/workspaces/coding-guidelines/go-testing-guide.md)** - Ginkgo v2/Gomega testing
- **[go-makefile-commands.md](~/Documents/workspaces/coding-guidelines/go-makefile-commands.md)** - Build commands
- **[git-commit-workflow.md](~/Documents/workspaces/coding-guidelines/git-commit-workflow.md)** - Commit process

## Architecture

- `main.go` — CLI entry point, subcommands: `run`, `status`, `prompt approve/reset`
- `pkg/cmd/` — CLI subcommands (daemon, run, status, prompt, spec)
- `pkg/config/` — Configuration parsing (`.dark-factory.yaml`)
- `pkg/executor/` — Execute single prompt via claude-yolo Docker container
- `pkg/factory/` — Wire dependencies, create processor and review poller
- `pkg/generator/` — Auto-generate prompts from approved specs
- `pkg/git/` — Git operations: clone, branch, commit, push, PR creation, merge
- `pkg/lock/` — Instance lock (one dark-factory per project)
- `pkg/processor/` — Core orchestration: pick up prompts, setup workflow, execute, handle post-execution
- `pkg/project/` — Project metadata detection
- `pkg/prompt/` — Parse/update YAML frontmatter in prompt markdown files
- `pkg/report/` — Parse completion reports from YOLO output
- `pkg/review/` — PR review polling for autoReview mode
- `pkg/runner/` — Docker container lifecycle management
- `pkg/server/` — REST API for status and control
- `pkg/spec/` — Spec file management and auto-completion
- `pkg/specnum/` — Spec number assignment
- `pkg/specwatcher/` — Watch specs/ directory for file changes
- `pkg/status/` — Combined status reporting
- `pkg/version/` — Version detection and management
- `pkg/watcher/` — Watch prompts/ directory for file changes

## Key Design Decisions

- **Frontmatter = state** — no database, prompt file frontmatter tracks status
- **Two workflow modes:** `direct` (commit to current branch) and `pr` (clone → feature branch → PR)
- **YOLO has NO git access** — all git ops on host; clone mounted read-write for code only
- **Sequential processing** — one prompt at a time per project
- **Stop on failure** — never skip failed prompts
- **PR workflow: prompt lifecycle in original repo** — clone contains only code changes; prompt move/status updates happen in the original working directory
- **Stale clone recovery** — cloner removes existing dest dir before cloning to handle crashes
