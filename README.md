# dark-factory

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

## Quick Start

```bash
cd ~/Documents/workspaces/your-project
dark-factory
```

Watches `prompts/queue/` for prompts to execute. See `example/` for a complete project layout.

## Directory Structure

```
your-project/
├── .dark-factory.yaml      # optional config
├── prompts/                 # inbox (passive, drop prompts here)
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
3. Watcher detects the file, renames to `NNN-name.md`, sets frontmatter
4. Processor picks the lowest-numbered queued prompt
5. Spins up a claude-yolo Docker container
6. On success: commits, tags, pushes, moves to `prompts/completed/`
7. On failure: sets `status: failed` in frontmatter

## Prompt Format

Markdown with optional YAML frontmatter:

```markdown
# Add user authentication

## Goal

Add JWT-based authentication to the REST API.

## Implementation

...
```

Frontmatter is managed by dark-factory:

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

Optional `.dark-factory.yaml` in project root:

```yaml
workflow: direct                                    # "direct" (default) or "pr"
inboxDir: prompts                                   # passive drop zone (default: prompts)
queueDir: prompts/queue                             # watcher + processor dir (default: prompts)
completedDir: prompts/completed                     # archive dir (default: prompts/completed)
containerImage: docker.io/bborbe/claude-yolo:v0.0.7 # YOLO Docker image
debounceMs: 500                                     # watcher debounce in ms
serverPort: 8080                                    # REST API port
```

All fields optional. Missing file or fields use defaults.

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
