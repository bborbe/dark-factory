# dark-factory

Autonomous coding pipeline — drop prompts in, get PRs out.

One factory per project, sequential prompt processing, zero human intervention between spec and PR.

## Status

Under Development

## How It Works

```
You (fast)                         Factory (slow, unattended)
├── create prompt 1  ──→  prompts/ ┌─ worktree → YOLO → commit
├── create prompt 2  ──→  prompts/ │  YOLO → commit
├── create prompt 3  ──→  prompts/ │  YOLO → commit → push → PR
└── go AFK                         └─ idle, watching prompts/
                                              ↓
You come back                  ←── PR waiting for review
```

## Usage

```bash
cd ~/Documents/workspaces/vault-cli
dark-factory
```

No arguments needed. Watches `prompts/` in current directory.

## Prompt Lifecycle

Status tracked in YAML frontmatter:

| Status | Who | What |
|--------|-----|------|
| (none) | Human | Drafting |
| `queued` | Factory | Ready for execution |
| `executing` | Factory | YOLO working |
| `waiting` | Human | PR exists, review needed |
| `completed` | Terminal | Done, merged |
| `failed` | Human | Needs manual fix |

## Design Principles

- **YOLO has NO git access** — all git ops happen in the host
- **Sequential processing** — prompts build on each other
- **Stop on failure** — never skip, human fixes and resumes
- **One factory per project** — parallelism at the project level
- **Fresh context per prompt** — no context rot (Ralph Loop principle)
- **Frontmatter = state** — no database, no config files

## License

BSD-2-Clause
