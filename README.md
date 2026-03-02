# 🏭 Dark Factory

Autonomous coding pipeline — drop prompts in, get commits out.

One factory per project, sequential prompt processing, zero human intervention between spec and commit.

## How It Works

```
You (fast)                              Factory (slow, unattended)
├── write prompt    ──→  prompts/       (inbox, passive)
├── ready to go     ──→  prompts/queue/ ┌─ watcher renames, processor executes
├── write prompt 2  ──→  prompts/       │  YOLO → commit → tag → push
├── move to queue   ──→  prompts/queue/ │  YOLO → commit → tag → push
└── go AFK                              └─ idle, watching prompts/queue/
                                                    ↓
You come back                       ←── changes committed and pushed
```

## Prerequisites

- **Go 1.24+** — to build dark-factory
- **Docker** — to run claude-yolo containers
- **claude-yolo image** — `docker pull docker.io/bborbe/claude-yolo:v0.0.7`
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
mkdir -p prompts/queue prompts/completed

# 2. Write a prompt
cat > prompts/my-feature.md << 'EOF'
# Add health check endpoint

## Goal

Add a `/health` endpoint that returns 200 OK.

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push
EOF

# 3. Queue it for execution
mv prompts/my-feature.md prompts/queue/

# 4. Start dark-factory
dark-factory

# 5. Watch it work — watcher renames to 001-my-feature.md, processor executes
# When done, prompt moves to prompts/completed/001-my-feature.md
# Logs at prompts/log/001-my-feature.log
```

## Directory Structure

```
your-project/
├── .dark-factory.yaml      # optional config
├── CHANGELOG.md            # optional — enables auto-versioning with tags
├── prompts/                # inbox (passive, drop prompts here)
│   ├── my-new-feature.md   # draft, nothing happens
│   ├── queue/              # watcher watches here, processor executes
│   │   └── 001-my-task.md  # will be picked up and executed
│   ├── completed/          # done prompts archived here
│   │   └── 001-my-task.md
│   └── log/                # execution logs
│       └── 001-my-task.log
└── ...
```

## Workflow

1. Write a prompt in `prompts/` (inbox — nothing happens automatically)
2. When ready, move it to `prompts/queue/`
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
3. Reset status to `queued` in the frontmatter and move back to `prompts/queue/`
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
dark-factory              # run (default) — watch and process prompts
dark-factory run          # same as above
dark-factory status       # show queue, running prompt, completed count
dark-factory status -json # JSON output
```

## REST API

Runs on port 8080 (configurable via `serverPort`):

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Health check |
| `GET /status` | Current processing status |
| `GET /queue` | Queued prompts |
| `GET /completed` | Completed prompts |

## Configuration

Optional `.dark-factory.yaml` in project root. Without it, dark-factory uses defaults.

```yaml
workflow: direct                                    # "direct" (default) or "pr"
inboxDir: prompts                                   # passive drop zone (default: prompts)
queueDir: prompts/queue                             # watcher + processor dir (default: prompts)
completedDir: prompts/completed                     # archive dir (default: prompts/completed)
containerImage: docker.io/bborbe/claude-yolo:v0.0.7 # YOLO Docker image
debounceMs: 500                                     # watcher debounce in ms
serverPort: 8080                                    # REST API port
```

**Important**: Without a config file, `queueDir` defaults to `prompts` (not `prompts/queue`). Create a `.dark-factory.yaml` to use the inbox/queue separation. See `example/` for a complete setup.

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
