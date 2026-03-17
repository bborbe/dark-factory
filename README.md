# Dark Factory

[![CI](https://github.com/bborbe/dark-factory/actions/workflows/ci.yml/badge.svg)](https://github.com/bborbe/dark-factory/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/bborbe/dark-factory.svg)](https://pkg.go.dev/github.com/bborbe/dark-factory)
[![Go Report Card](https://goreportcard.com/badge/github.com/bborbe/dark-factory)](https://goreportcard.com/report/github.com/bborbe/dark-factory)

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

## Prerequisites

- **Go 1.26+** — to build dark-factory
- **Docker** — to run claude-yolo containers
- **claude-yolo image** — `docker pull docker.io/bborbe/claude-yolo:v0.3.0`
- **~/.claude-yolo/** — Claude Code config for the YOLO agent (see [YOLO Container Setup](docs/yolo-container-setup.md))
- **DARK_FACTORY_CLAUDE_CONFIG_DIR** — set to `~/.claude-yolo` (otherwise defaults to `~/.claude`)

## Installation

```bash
go install github.com/bborbe/dark-factory@latest
```

Or build from source:

```bash
git clone https://github.com/bborbe/dark-factory.git
cd dark-factory
make install
```

## Quick Start

```bash
# 1. Initialize project
cd ~/your-project
mkdir -p prompts/in-progress prompts/completed prompts/log
cat > .dark-factory.yaml <<'EOF'
pr: false
worktree: false
EOF

# 2. Write a prompt
cat > prompts/my-feature.md << 'EOF'
<summary>
- Adds a /health endpoint returning 200 OK
- No authentication required
- Existing endpoints unchanged
</summary>

<objective>
Add a health check endpoint for load balancer probes.
</objective>

<context>
Read CLAUDE.md for project conventions.
</context>

<requirements>
1. Add GET /health handler returning 200 with body "ok"
2. Register in the router
3. Add test
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
EOF

# 3. Approve and run
dark-factory prompt approve my-feature
dark-factory daemon
```

## Commands

| Command | Purpose |
|---------|---------|
| `dark-factory daemon` | Watch for prompts and process continuously |
| `dark-factory run` | Process all queued prompts and exit |
| `dark-factory status` | Show combined status |
| `dark-factory prompt list` | List prompts with status |
| `dark-factory prompt approve <name>` | Queue a prompt for execution |
| `dark-factory prompt retry` | Re-queue failed prompts |
| `dark-factory spec list` | List specs with status |
| `dark-factory spec approve <name>` | Approve a spec (triggers prompt generation) |
| `dark-factory spec complete <name>` | Mark a verified spec as completed |

## Claude Code Plugin

Dark-factory includes a Claude Code plugin for writing and auditing specs and prompts.

```bash
claude plugin marketplace add bborbe/dark-factory
claude plugin install dark-factory
```

| Command | Description |
|---------|-------------|
| `/dark-factory:create-spec` | Create a spec file interactively |
| `/dark-factory:create-prompt` | Create a prompt from spec or description |
| `/dark-factory:audit-spec` | Audit spec against quality criteria |
| `/dark-factory:audit-prompt` | Audit prompt against Definition of Done |
| `/dark-factory:daemon` | Start daemon in background |
| `/dark-factory:watch` | Monitor daemon with sound alerts (auto-detects project) |
| `/dark-factory:run` | One-shot: generate prompts from specs, execute queue |
| `/dark-factory:init-project` | Initialize project for dark-factory |
| `/dark-factory:read-guides` | Load all dark-factory guides into context |
| `/dark-factory:generate-code-review-prompts` | Review service against guidelines, generate fix prompts |

## Configuration

Optional `.dark-factory.yaml` in project root:

```yaml
pr: false                                            # create PRs (default: false)
worktree: false                                      # clone repo for isolation (default: false)
validationCommand: "make precommit"                  # shell command run after each prompt
validationPrompt: docs/dod.md                        # AI-judged quality criteria (file or inline)
containerImage: docker.io/bborbe/claude-yolo:v0.3.0  # YOLO Docker image
model: claude-sonnet-4-6                             # Claude model
```

See [docs/configuration.md](docs/configuration.md) for all config fields, validation, notifications, and providers.

## YOLO Container Configuration

Dark-factory executes prompts inside a [claude-yolo](https://github.com/bborbe/claude-yolo) Docker container. The container needs a Claude Code config directory on the host, mounted into the container.

**Quick setup** (use bborbe's config):

```bash
git clone https://github.com/bborbe/claude-yolo.git ~/.claude-yolo
cd ~/.claude-yolo && claude login
export DARK_FACTORY_CLAUDE_CONFIG_DIR=~/.claude-yolo
```

**Minimal setup** (create your own): You need three files — OAuth credentials (`claude login`), `settings.json`, and a `CLAUDE.md` with agent instructions.

See [docs/yolo-container-setup.md](docs/yolo-container-setup.md) for both options in detail.

## Directory Structure

```
your-project/
├── .dark-factory.yaml        # config
├── prompts/                  # inbox (drop prompts here)
│   ├── in-progress/          # queued/executing
│   ├── completed/            # archived
│   └── log/                  # execution logs
├── specs/                    # spec inbox
│   ├── in-progress/          # approved specs
│   └── completed/            # done specs
└── scenarios/                # end-to-end verification
```

## Documentation

| Guide | Purpose |
|-------|---------|
| [Architecture & Flow](docs/architecture-flow.md) | End-to-end execution flow, what runs where |
| [Configuration](docs/configuration.md) | All config fields, validation, notifications, providers |
| [Init Project](docs/init-project.md) | Set up a new project for dark-factory |
| [Spec Writing](docs/spec-writing.md) | Write behavioral specs for multi-prompt features |
| [Prompt Writing](docs/prompt-writing.md) | Write effective prompts for the YOLO agent |
| [Running](docs/running.md) | Start, monitor, and troubleshoot the pipeline |
| [Scenario Writing](docs/scenario-writing.md) | Write end-to-end verification checklists |
| [YOLO Container Setup](docs/yolo-container-setup.md) | Set up `~/.claude-yolo/` config directory |
| [CLAUDE.md Guide](docs/claude-md-guide.md) | Write project CLAUDE.md files for dark-factory |
| [Definition of Done](docs/dod.md) | Quality criteria for validationPrompt |

## Design Principles

- **YOLO has NO git access** — all git ops happen on the host
- **Sequential processing** — prompts execute in number order
- **Frontmatter = state** — no database
- **Fresh context per prompt** — no context rot
- **Stop on failure** — never skip failed prompts
- **Instance lock** — one dark-factory per project

## License

BSD-2-Clause
