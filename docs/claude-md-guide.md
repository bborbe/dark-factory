# Writing a CLAUDE.md for Dark Factory Projects

Guide for writing CLAUDE.md files in projects managed by the dark-factory pipeline. The CLAUDE.md is read by the agent inside the Docker container — it's the only context the agent has about your project.

## Complete Flow Overview

```
Spec → Audit → Approve → [auto-generate prompts] → Audit → Approve → [auto-execute] → Done
 ↑                                                    ↑
 /dark-factory:create-spec                            /dark-factory:create-prompt (or auto from spec)
```

### Flow in Detail

| Step | Who | Action | Command/Tool |
|------|-----|--------|-------------|
| 1. Create spec | Human + Claude | Write behavioral spec for multi-prompt features | `/dark-factory:create-spec` |
| 2. Audit spec | Claude | Validate spec against preflight checklist | `/dark-factory:audit-spec` |
| 3. Approve spec | **Human confirms first**, then Claude executes | Move spec from inbox to queue | `dark-factory spec approve <name>` |
| 4. Generate prompts | dark-factory (automatic) | Spec watcher generates prompts from approved spec | Automatic — daemon watches `specs/in-progress/` |
| 5. Create prompts (standalone) | Human + Claude | Write prompts for simple changes (no spec needed) | `/dark-factory:create-prompt` |
| 6. Audit prompts | Claude | Validate prompt against Definition of Done | `/dark-factory:audit-prompt` |
| 7. Approve prompts | **Human confirms first**, then Claude executes | Move prompts from inbox to queue | `dark-factory prompt approve <name>` |
| 8. Execute prompts | dark-factory (automatic) | YOLO container runs each prompt sequentially | Automatic — daemon watches `prompts/in-progress/` |
| 9. Start daemon | **Human confirms first**, then Claude executes | Start dark-factory to process queue | `dark-factory daemon` (background) |

### When to Use Specs vs Standalone Prompts

| Change | Action |
|--------|--------|
| Simple fix, config change, 1-2 files | Skip spec → write prompt directly with `/dark-factory:create-prompt` |
| Multi-prompt feature, unclear edges, shared interfaces | Write spec first with `/dark-factory:create-spec` |

## CLAUDE.md Structure

A dark-factory CLAUDE.md has these sections in order:

1. Project summary (1 line)
2. Dark Factory Workflow (copy from template — includes complete flow)
3. Development Standards (project-specific)
4. Architecture (package map)
5. Key Design Decisions (important constraints)

## 1. Project Summary

One sentence describing what the project is. The agent needs this to understand the domain.

```markdown
# CLAUDE.md

Receipt API service for Octopus.
```

## 2. Dark Factory Workflow

Copy this section verbatim into every dark-factory project. Adjust only the guide links if your vault structure differs.

```markdown
## Dark Factory Workflow

**Never code directly.** All code changes go through the dark-factory pipeline.

### Complete Flow

**Spec-based (multi-prompt features):**

1. Create spec → `/dark-factory:create-spec`
2. Audit spec → `/dark-factory:audit-spec`
3. User confirms → `dark-factory spec approve <name>`
4. dark-factory auto-generates prompts from spec
5. Audit prompts → `/dark-factory:audit-prompt`
6. User confirms → `dark-factory prompt approve <name>`
7. Start daemon → `dark-factory daemon` (use Bash `run_in_background: true`)
8. dark-factory executes prompts automatically

**Standalone prompts (simple changes):**

1. Create prompt → `/dark-factory:create-prompt`
2. Audit prompt → `/dark-factory:audit-prompt`
3. User confirms → `dark-factory prompt approve <name>`
4. Start daemon → `dark-factory daemon` (use Bash `run_in_background: true`)
5. dark-factory executes prompt automatically

### Assess the change size

| Change | Action |
|--------|--------|
| Simple fix, config change, 1-2 files | Write a prompt → `/dark-factory:create-prompt` |
| Multi-prompt feature, unclear edges, shared interfaces | Write a spec first → `/dark-factory:create-spec` |

### Read the relevant guide before starting — every time, not from memory

- Writing a spec → read [[Dark Factory - Write Spec]] and [[Dark Factory Guide#Specs What Makes a Good Spec]]
- Writing prompts → read [[Dark Factory - Write Prompts]] and [[Dark Factory Guide#Prompts What Makes a Good Prompt]]
- Running prompts → read [[Dark Factory - Run Prompt]]

### Claude Code Commands

| Command | Purpose |
|---------|---------|
| `/dark-factory:create-spec` | Create a spec file interactively |
| `/dark-factory:create-prompt` | Create a prompt file from spec or task description |
| `/dark-factory:audit-spec` | Audit spec against preflight checklist |
| `/dark-factory:audit-prompt` | Audit prompt against Definition of Done |

### CLI Commands

| Command | Purpose |
|---------|---------|
| `dark-factory spec approve <name>` | Approve spec (inbox → queue, triggers prompt generation) |
| `dark-factory prompt approve <name>` | Approve prompt (inbox → queue) |
| `dark-factory daemon` | Start daemon (watches queue, executes prompts) |
| `dark-factory run` | One-shot mode (process all queued, then exit) |
| `dark-factory status` | Show combined status of prompts and specs |
| `dark-factory prompt list` | List all prompts with status |
| `dark-factory spec list` | List all specs with status |
| `dark-factory prompt retry` | Re-queue failed prompts for retry |

### Key rules

- Prompts go to **`prompts/`** (inbox) — never to `prompts/in-progress/` or `prompts/completed/`
- Specs go to **`specs/`** (inbox) — never to `specs/in-progress/` or `specs/completed/`
- Never number filenames — dark-factory assigns numbers on approve
- Never manually edit frontmatter status — use CLI commands above
- Always audit before approving (`/dark-factory:audit-prompt`, `/dark-factory:audit-spec`)
- **BLOCKING: Never run `dark-factory prompt approve`, `dark-factory spec approve`, or `dark-factory daemon` without explicit user confirmation.** Write the prompt/spec, then STOP and ask the user to approve. Do not assume approval from prior context or task momentum.
- **Start daemon in background** — use Bash tool with `run_in_background: true` (not foreground, not detached with `&`)
```

## 3. Development Standards

Project-specific build, test, and environment setup. The agent needs this to run `make precommit` and understand the toolchain.

### What to include

- **Coding guidelines reference** — link to shared guidelines repo if applicable
- **Module path** — Go module name, vendor mode
- **Environment variables** — anything needed before `go mod tidy` or builds (e.g., `GOPRIVATE`)
- **Build commands** — `make precommit`, `make test`, direct alternatives
- **Test conventions** — framework (Ginkgo/Gomega), mock tool (Counterfeiter), test package style (`*_test`)
- **Key dependencies** — libraries the agent will encounter (error handling, HTTP, routing)

### Example: Go project with private modules

```markdown
## Development Standards

### Project layout

- Go service in `api/` subdirectory
- Module: `bitbucket.seibert.tools/oc/receipt/api`
- Vendor mode: all commands use `-mod=vendor`

### Environment

Before any Go commands that resolve modules:

\`\`\`bash
export GOPRIVATE=bitbucket.seibert.tools/*
export GONOSUMCHECK=bitbucket.seibert.tools/*
\`\`\`

### Build and test

- `make precommit` — lint + format + test
- `make test` — tests only

### Test conventions

- Ginkgo/Gomega test framework
- Counterfeiter for mocks (`mocks/` dir)
- External test packages (`*_test`)
- No real HTTP, subprocess, or network calls in tests

### Dependencies

- `github.com/bborbe/errors` — error handling
- `github.com/bborbe/http` — HTTP utilities
- `github.com/onsi/ginkgo/v2` / `github.com/onsi/gomega` — testing
- `github.com/maxbrunsfeld/counterfeiter/v6` — mock generation
```

### Example: Go project with shared guidelines

```markdown
## Development Standards

This project follows the [coding-guidelines](https://github.com/bborbe/coding-guidelines).

### Key Reference Guides

- **go-architecture-patterns.md** — Interface → Constructor → Struct → Method
- **go-testing-guide.md** — Ginkgo v2/Gomega testing
- **go-makefile-commands.md** — Build commands
```

## 4. Architecture

A flat list of packages with one-line descriptions. This is the agent's map of the codebase — it tells the agent where to look and what each package owns.

### Rules

- One line per package/directory
- Start with the entry point, then list packages alphabetically or by dependency order
- Describe what the package does, not how

### Example

```markdown
## Architecture

- `main.go` — CLI entry point, subcommands: `run`, `status`, `prompt approve/reset`
- `pkg/config/` — Configuration parsing (`.dark-factory.yaml`)
- `pkg/executor/` — Execute single prompt via Docker container
- `pkg/factory/` — Wire dependencies, create processor
- `pkg/git/` — Git operations: clone, branch, commit, push, PR
- `pkg/processor/` — Core orchestration: pick up prompts, execute, handle results
- `pkg/prompt/` — Parse/update YAML frontmatter in prompt files
```

## 5. Key Design Decisions

Constraints the agent must respect. Without these, the agent may make reasonable-looking changes that violate your architecture.

### What to include

- Architectural invariants (what talks to what, what has no access to what)
- State management approach (database, files, frontmatter)
- Processing model (sequential, parallel, retry policy)
- Workflow modes and their differences

### Example

```markdown
## Key Design Decisions

- **Frontmatter = state** — no database, prompt file frontmatter tracks status
- **Two workflow modes:** `direct` (commit to current branch) and `pr` (clone → feature branch → PR)
- **YOLO has NO git access** — all git ops on host; clone mounted read-write for code only
- **Sequential processing** — one prompt at a time per project
- **Stop on failure** — never skip failed prompts
- **Factory functions are pure composition** — no conditionals, no I/O, no context.Background()
```

## Common Mistakes

### Too little context

The agent runs in a Docker container with only the codebase and CLAUDE.md. It cannot ask questions, browse the web, or read external docs. If the agent needs to know something to avoid breaking your project, it must be in CLAUDE.md.

**Bad:** Omitting that `GOPRIVATE` is required → agent runs `go mod tidy` and fails on private modules.

**Bad:** Not mentioning vendor mode → agent runs `go test ./...` without `-mod=vendor`.

### Too much detail

CLAUDE.md is not documentation. It's operational context for an autonomous agent. Keep it scannable.

**Bad:** Explaining the history of why you chose Ginkgo over `testing`.

**Good:** `Ginkgo/Gomega test framework` — the agent knows what to do.

### Missing architectural constraints

The agent will happily refactor your factory to include business logic, add `context.Background()` calls, or put handlers in `main.go` — unless you tell it not to.

**Bad:** No mention of zero-business-logic factory rule → agent adds conditionals in factory functions.

**Good:** `Factory functions are pure composition — no conditionals, no I/O, no context.Background()`

### Stale information

If your CLAUDE.md says the struct is at line 37 but you refactored last week, the agent will waste time searching. Use function/type names, not line numbers. Keep CLAUDE.md updated when architecture changes.

### Manually editing frontmatter

Never change `status: created` to `status: approved` by editing the file. The CLI does more than change the status — it moves files, assigns numbers, and normalizes filenames. Always use `dark-factory prompt approve` or `dark-factory spec approve`.

### Starting daemon wrong

The daemon is long-running. Don't run it in the foreground (blocks your session) or detached with `&` (loses lifecycle tracking). Use Bash tool with `run_in_background: true` so Claude Code tracks it properly.

## Checklist

Before approving your CLAUDE.md:

- [ ] One-line project summary at the top
- [ ] Dark Factory Workflow section with complete flow (spec + standalone paths)
- [ ] Claude Code commands table (`/dark-factory:create-spec`, `/dark-factory:create-prompt`, `/dark-factory:audit-spec`, `/dark-factory:audit-prompt`)
- [ ] CLI commands table (`dark-factory spec approve`, `dark-factory prompt approve`, `dark-factory daemon`, etc.)
- [ ] Key rules (inbox placement, no manual status edits, audit before approve, user confirmation)
- [ ] Build command documented (`make precommit` or equivalent)
- [ ] Test framework and mock tool named
- [ ] Environment variables listed (if any)
- [ ] Every package has a one-line description in Architecture
- [ ] Key design decisions that an agent could violate are listed
- [ ] No line numbers — use function/type names instead
- [ ] No stale references to removed code
