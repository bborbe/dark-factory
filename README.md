# 🏭 Dark Factory

Autonomous coding pipeline — drop prompts in, get commits out.

One factory per project, sequential prompt processing, zero human intervention between spec and commit.

## How It Works

```
You (fast)                                    Factory (slow, unattended)
├── write prompt    ──→  prompts/              (inbox, passive)
├── ready to go     ──→  prompts/in-progress/  ┌─ watcher renames, processor executes
├── write prompt 2  ──→  prompts/              │  YOLO → commit → tag → push
├── move to queue   ──→  prompts/in-progress/  │  YOLO → commit → tag → push
└── go AFK                                     └─ idle, watching prompts/in-progress/
                                                    ↓
You come back                       ←── changes committed and pushed
```

## Specs

| Spec | Problem |
|------|---------|
| [001-core-pipeline](specs/001-core-pipeline.md) | Running AI coding prompts requires babysitting: open terminal, paste prompt, wait, review, commit, repeat. |
| [002-configuration](specs/002-configuration.md) | All paths, image names, and settings are hardcoded. |
| [003-filename-normalization](specs/003-filename-normalization.md) | Users create prompt files with arbitrary names (`add-health-check.md`, `9-fix-bug.md`, `my task.md`). |
| [004-directory-separation](specs/004-directory-separation.md) | All prompts live in one directory. |
| [005-instance-locking](specs/005-instance-locking.md) | Running two dark-factory instances in the same project causes race conditions: both pick the same prompt, both try to commit, git conflicts. |
| [006-crash-recovery](specs/006-crash-recovery.md) | If dark-factory crashes mid-execution, a prompt is left with `status: executing` — it's stuck. |
| [007-git-direct-workflow](specs/007-git-direct-workflow.md) | After the Docker container makes changes, those changes need to be committed and pushed. |
| [008-http-api](specs/008-http-api.md) | When dark-factory runs unattended, there's no way to check progress without reading files directly. |
| [009-cli-commands](specs/009-cli-commands.md) | The only way to interact with dark-factory is to start the daemon. |
| [010-pr-workflow](specs/010-pr-workflow.md) | Direct workflow commits to the current branch. |
| [011-timestamp-frontmatter](specs/011-timestamp-frontmatter.md) | No visibility into when prompts were created, queued, started, or completed. |
| [012-duplicate-frontmatter-handling](specs/012-duplicate-frontmatter-handling.md) | Prompts created with empty frontmatter (`---\n---`) end up with a duplicate frontmatter block in the content body after the processor prepends real frontmatter. |
| [013-configurable-github-identity](specs/013-configurable-github-identity.md) | Dark-factory creates PRs using the current user's `gh` auth. |
| [014-pr-url-in-frontmatter](specs/014-pr-url-in-frontmatter.md) | When dark-factory creates a PR via the `pr` workflow, the PR URL is logged but not persisted in the prompt's frontmatter. |
| [015-prompt-status-frontmatter](specs/015-prompt-status-frontmatter.md) | Prompt lifecycle tracking relies solely on folder location (inbox/queue/completed). |
| [016-auto-merge-and-release](specs/016-auto-merge-and-release.md) | When `workflow: pr` or `workflow: worktree` is configured, dark-factory creates a PR and moves on. |

## Prerequisites

- **Go 1.24+** — to build dark-factory
- **Docker** — to run claude-yolo containers
- **claude-yolo image** — `docker pull docker.io/bborbe/claude-yolo:v0.2.5`
- **Anthropic API key** — set `ANTHROPIC_API_KEY` environment variable (passed to container)
- **~/.claude-yolo/** — Claude Code config for the YOLO agent (see [YOLO Container Configuration](#yolo-container-configuration))

## Installation

```bash
go install github.com/bborbe/dark-factory@latest
```

Or build from source:

```bash
git clone https://github.com/bborbe/dark-factory.git
cd dark-factory
make install   # installs to $GOPATH/bin
```

## Quick Start

```bash
# 1. Set up your project
cd ~/Documents/workspaces/your-project
mkdir -p prompts/in-progress prompts/completed

# 2. Write a prompt
cat > prompts/my-feature.md << 'EOF'
# Add health check endpoint

## Goal

Add a `/health` endpoint that returns 200 OK.

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push
EOF

# 3. Move to in-progress for execution
mv prompts/my-feature.md prompts/in-progress/

# 4. Start dark-factory (long-running, watches for new prompts)
dark-factory daemon

# 5. Watch it work — watcher renames to 001-my-feature.md, processor executes
# When done, prompt moves to prompts/completed/001-my-feature.md
# Logs at prompts/log/001-my-feature.log
```

## Directory Structure

```
your-project/
├── .dark-factory.yaml        # optional config
├── CHANGELOG.md              # optional — enables auto-versioning with tags
├── prompts/                  # inbox (passive, drop prompts here)
│   ├── my-new-feature.md     # draft, nothing happens
│   ├── in-progress/          # watcher watches here, processor executes
│   │   └── 001-my-task.md    # will be picked up and executed
│   ├── completed/            # done prompts archived here
│   │   └── 001-my-task.md
│   └── log/                  # execution logs
│       └── 001-my-task.log
└── ...
```

## Workflow

1. Write a prompt in `prompts/` (inbox — nothing happens automatically)
2. When ready, move it to `prompts/in-progress/`
3. Watcher detects the file, renames to `NNN-name.md`, sets `status: queued` in frontmatter
4. Processor picks the lowest-numbered queued prompt
5. Validates: correct number prefix, status is queued, all previous prompts completed
6. Sets `status: executing`, spins up a claude-yolo Docker container
7. On success: commits, tags, pushes, moves to `prompts/completed/`
8. On failure: sets `status: failed` in frontmatter

### Handling Failures

When a prompt fails (`status: failed`):

1. Check the log: `prompts/log/NNN-name.log`
2. Fix the prompt (clarify instructions, reduce scope, fix constraints)
3. Reset status to `queued` in the frontmatter and move back to `prompts/in-progress/`
4. Dark-factory will retry it

### Versioning

If your project has a `CHANGELOG.md`, dark-factory automatically:
- Determines version bump (patch/minor) from changes
- Updates CHANGELOG.md with new version
- Creates a git tag (e.g., `v0.3.4`)
- Pushes both commit and tag

Without `CHANGELOG.md`, dark-factory commits and pushes without tagging.

## Writing Good Prompts

A prompt is a markdown file that tells the YOLO agent what to build. Structure:

```markdown
# Short title describing the change

## Goal

One paragraph: what should change and why.

## Current Behavior (optional)

What happens now, if fixing a bug or changing existing code.

## Expected Behavior (optional)

What should happen after this prompt is executed.

## Implementation

Step-by-step plan. Be specific — the agent follows this literally:

### 1. Create/modify package X

Code examples, interface definitions, function signatures.

### 2. Update wiring

Where and how to connect the new code.

### 3. Tests

List specific test cases (valid input passes, invalid input fails, edge cases).

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Follow specific coding guides (reference by path)
- Coverage ≥80% for changed packages
```

Tips:
- **Be specific** — "add a Validate() method" beats "add validation"
- **Include code examples** — show interfaces, function signatures, expected behavior
- **Reference coding guides** — `~/.claude-yolo/docs/go-patterns.md` etc.
- **Always include constraints** — especially "do NOT commit" (dark-factory handles git)
- **One concern per prompt** — smaller prompts succeed more often than large ones

## Prompt Format

Frontmatter is managed by dark-factory (you don't need to add it):

| Status | Who | What |
|--------|-----|------|
| (none) | Human | Drafting in inbox |
| `queued` | Factory | Ready for execution |
| `executing` | Factory | YOLO container running |
| `completed` | Factory | Done, in completed/ |
| `failed` | Factory | Needs manual fix |

## Commands

```bash
dark-factory run          # process all queued prompts and exit (one-shot)
dark-factory daemon       # watch for prompts and process continuously (long-running)
dark-factory status       # show queue, running prompt, completed count
dark-factory status -json # JSON output
```

## REST API

Disabled by default (`serverPort: 0`). Set a port to enable:

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Health check |
| `GET /status` | Current processing status |
| `GET /queue` | Queued prompts |
| `GET /completed` | Completed prompts |

## Configuration

Optional `.dark-factory.yaml` in project root. Without it, dark-factory uses defaults.

```yaml
projectName: my-project                              # optional project identifier
github:
  token: ${DARK_FACTORY_GITHUB_TOKEN}                # default — reads from env var
workflow: direct                                     # "direct" (default), "pr", or "worktree"
autoMerge: false                                     # wait for PR approval and merge (requires workflow: pr or worktree)
autoRelease: false                                   # create release after merge (requires autoMerge: true)
prompts:
  inboxDir: prompts                                  # passive drop zone (default: prompts)
  inProgressDir: prompts/in-progress                 # watcher + processor dir (default: prompts/in-progress)
  completedDir: prompts/completed                    # archive dir (default: prompts/completed)
  logDir: prompts/log                                # execution logs (default: prompts/log)
specs:
  inboxDir: specs                                    # spec inbox (default: specs)
  inProgressDir: specs/in-progress                   # spec processing dir (default: specs/in-progress)
  completedDir: specs/completed                      # completed specs (default: specs/completed)
  logDir: specs/log                                  # spec logs (default: specs/log)
containerImage: docker.io/bborbe/claude-yolo:v0.2.5  # YOLO Docker image
model: claude-sonnet-4-6                             # Claude model (default: claude-sonnet-4-6)
debounceMs: 500                                      # watcher debounce in ms
serverPort: 0                                        # REST API port (0 = disabled)
```

### GitHub Token

The optional `github.token` field allows dark-factory to use a specific GitHub identity for `gh` CLI operations (creating PRs, checking PR status, etc.). This is useful when you want dark-factory to create PRs under a bot account instead of your personal account.

- Use `${VAR_NAME}` syntax to reference environment variables
- When set, all `gh` commands use this token via the `GH_TOKEN` environment variable
- When not set or empty, `gh` uses its default authentication (from `gh auth login`)
- Token must never be committed — use environment variable reference only
- For security, ensure `.dark-factory.yaml` is not world-readable: `chmod 600 .dark-factory.yaml`

The inbox/in-progress/completed separation works out of the box with these defaults. You can customize any of these paths via `.dark-factory.yaml`. See `example/` for a complete setup.

## YOLO Container Configuration

Dark-factory executes prompts inside a [claude-yolo](https://github.com/bborbe/claude-yolo) Docker container. The container gets its Claude Code configuration from `~/.claude-yolo/` on the host, mounted as `/home/node/.claude` inside the container.

```
~/.claude-yolo/
├── CLAUDE.md          # instructions for the YOLO agent
├── docs/              # coding guides (go-patterns.md, go-testing.md, ...)
├── commands/          # custom slash commands
└── agents/            # custom agents
```

Set up `~/.claude-yolo/` before first use:

1. Create the directory: `mkdir -p ~/.claude-yolo/docs`
2. Add a `CLAUDE.md` with instructions for your YOLO agent (workflow rules, git constraints, verification steps)
3. Add coding guides in `docs/` for project-specific conventions the AI wouldn't know

The docs teach YOLO conventions like custom library patterns, naming rules, and linter limits. See `~/.claude-yolo/docs/README.md` for details on writing effective guides.

## Claude Code Plugin

Dark-factory includes a Claude Code plugin with commands for writing and auditing specs and prompts. Works in any project — your Obsidian vault, a Go service, wherever you run Claude Code.

### Install

```bash
claude plugin marketplace add bborbe/dark-factory
claude plugin install dark-factory
```

### Commands

| Command | Description |
|---------|-------------|
| `/audit-prompt <file>` | Audit prompt against Prompt Definition of Done |
| `/audit-spec <file>` | Audit spec against preflight checklist and quality criteria |
| `/create-prompt <spec-or-description>` | Create prompt files from a spec or task description |
| `/create-spec <description>` | Create a spec file for a feature or change |

### Agents

| Agent | Purpose |
|-------|---------|
| `prompt-auditor` | Validates prompt structure, code references, quality scoring |
| `spec-auditor` | Validates spec sections, behavioral level, preflight checklist |
| `prompt-creator` | Decomposes specs into 2-6 executable prompts |
| `spec-creator` | Interactive spec creation with template and scope validation |

All agents are self-contained — no external file dependencies. Knowledge (Prompt DoD, spec template, preflight checklist) is inlined.

## Design Principles

- **YOLO has NO git access** — all git ops happen on the host
- **Sequential processing** — prompts execute in number order
- **Validation before execution** — prompt must have valid status, number prefix, and all predecessors completed
- **Instance lock** — only one dark-factory per project (flock + PID file)
- **Fresh context per prompt** — no context rot
- **Frontmatter = state** — no database
- **Version tracked** — each completed prompt records the dark-factory version

## License

BSD-2-Clause
