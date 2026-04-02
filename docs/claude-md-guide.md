# Writing a CLAUDE.md for Dark Factory Projects

Extension of the [general CLAUDE.md guide](https://github.com/bborbe/coding-guidelines/blob/master/docs/claude-md-guide.md). Read the general guide first — this doc only covers dark-factory-specific additions.

The CLAUDE.md is read by the YOLO agent inside the Docker container — it's the only context the agent has about your project.

## Structure

A dark-factory CLAUDE.md follows the general structure plus a mandatory Dark Factory Workflow section:

```markdown
# CLAUDE.md

One-line project summary.

## Dark Factory Workflow          <-- dark-factory addition
[copy from template below]

## Release Checklist              <-- if project has plugin
[multi-file version bumps]

## Development Standards
[build, test, env, toolchain]

## Architecture
[package map, entry points]

## Key Design Decisions
[constraints]
```

## Dark Factory Workflow Section

Copy this section verbatim into every dark-factory project. Adjust only the guide links if your vault structure differs.

```markdown
## Dark Factory Workflow

**Never code directly.** All code changes go through the dark-factory pipeline.

### Complete Flow

**Spec-based (multi-prompt features):**

1. Create spec -> `/dark-factory:create-spec`
2. Audit spec -> `/dark-factory:audit-spec`
3. User confirms -> `dark-factory spec approve <name>`
4. dark-factory auto-generates prompts from spec
5. Audit prompts -> `/dark-factory:audit-prompt`
6. User confirms -> `dark-factory prompt approve <name>`
7. Start daemon -> `dark-factory daemon` (use Bash `run_in_background: true`)
8. dark-factory executes prompts automatically

**Standalone prompts (simple changes):**

1. Create prompt -> `/dark-factory:create-prompt`
2. Audit prompt -> `/dark-factory:audit-prompt`
3. User confirms -> `dark-factory prompt approve <name>`
4. Start daemon -> `dark-factory daemon` (use Bash `run_in_background: true`)
5. dark-factory executes prompt automatically

### Assess the change size

| Change | Action |
|--------|--------|
| Simple fix, config change, 1-2 files | Write a prompt -> `/dark-factory:create-prompt` |
| Multi-prompt feature, unclear edges, shared interfaces | Write a spec first -> `/dark-factory:create-spec` |

### Read the relevant guide before starting -- every time, not from memory

- Writing a spec -> read [[Dark Factory - Write Spec]] and [[Dark Factory Guide#Specs What Makes a Good Spec]]
- Writing prompts -> read [[Dark Factory - Write Prompts]] and [[Dark Factory Guide#Prompts What Makes a Good Prompt]]
- Running prompts -> read [[Dark Factory - Run Prompt]]

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
| `dark-factory spec approve <name>` | Approve spec (inbox -> queue, triggers prompt generation) |
| `dark-factory prompt approve <name>` | Approve prompt (inbox -> queue) |
| `dark-factory daemon` | Start daemon (watches queue, executes prompts) |
| `dark-factory run` | One-shot mode (process all queued, then exit) |
| `dark-factory status` | Show combined status of prompts and specs |
| `dark-factory prompt list` | List all prompts with status |
| `dark-factory spec list` | List all specs with status |
| `dark-factory prompt retry` | Re-queue failed prompts for retry |
| `dark-factory prompt cancel <name>` | Cancel a running or queued prompt (never use `docker kill`) |

### Key rules

- Prompts go to **`prompts/`** (inbox) -- never to `prompts/in-progress/` or `prompts/completed/`
- Specs go to **`specs/`** (inbox) -- never to `specs/in-progress/` or `specs/completed/`
- Never number filenames -- dark-factory assigns numbers on approve
- Never manually edit frontmatter status -- use CLI commands above
- Always audit before approving (`/dark-factory:audit-prompt`, `/dark-factory:audit-spec`)
- **BLOCKING: Never run `dark-factory prompt approve`, `dark-factory spec approve`, or `dark-factory daemon` without explicit user confirmation.** Write the prompt/spec, then STOP and ask the user to approve.
- **Before starting daemon** -- run `dark-factory status` first to check if one is already running.
- **Start daemon in background** -- use Bash tool with `run_in_background: true` (not foreground, not detached with `&`)
```

## YOLO Agent Constraints

The YOLO agent runs in a Docker container with only the codebase and CLAUDE.md. It cannot:
- Ask questions or interact with the user
- Browse the web or read external docs
- Access MCP servers or Obsidian vault

If the agent needs to know something to avoid breaking the project, it must be in CLAUDE.md.

## Common Mistakes (Dark Factory Specific)

### Too little context

**Bad:** Omitting that `GOPRIVATE` is required -> agent runs `go mod tidy` and fails on private modules.

**Bad:** Not mentioning vendor mode -> agent runs `go test ./...` without `-mod=vendor`.

### Manually editing frontmatter

Never change `status: draft` to `status: approved` by editing the file. The CLI does more than change the status — it moves files, assigns numbers, and normalizes filenames. Always use `dark-factory prompt approve` or `dark-factory spec approve`.

### Killing containers with docker kill

Never use `docker kill` to stop a running prompt. It leaves dark-factory in an inconsistent state (stale lock, no proper cleanup). Use `dark-factory prompt cancel <name>` instead — it stops the container and updates the prompt status cleanly.

### Starting daemon wrong

The daemon is long-running. Don't run it in the foreground (blocks your session) or detached with `&` (loses lifecycle tracking). Use Bash tool with `run_in_background: true` so Claude Code tracks it properly.

## Checklist (Dark Factory Additions)

In addition to the [general CLAUDE.md checklist](https://github.com/bborbe/coding-guidelines/blob/master/docs/claude-md-guide.md#checklist):

- [ ] Dark Factory Workflow section with complete flow (spec + standalone paths)
- [ ] Claude Code commands table
- [ ] CLI commands table
- [ ] Key rules (inbox placement, no manual status edits, audit before approve, user confirmation)
- [ ] Release Checklist if project ships a plugin
