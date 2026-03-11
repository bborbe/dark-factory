# YOLO Container Setup

Dark-factory executes prompts inside a [claude-yolo](https://github.com/bborbe/claude-yolo) Docker container. The container needs a Claude Code configuration directory (`~/.claude-yolo/`) on the host, mounted as `/home/node/.claude` inside the container.

Two options: use bborbe's config (includes Go/Python coding guides, agents, and slash commands) or create your own minimal config.

## Option 1: Use bborbe's Config

Clone the repo and authenticate:

```bash
git clone https://github.com/bborbe/claude-yolo.git ~/.claude-yolo
cd ~/.claude-yolo
claude login
```

This gives you:
- `CLAUDE.md` — workflow instructions (no attribution, verification rules, completion protocol)
- `docs/` — 25+ coding guides (Go patterns, testing, security, Python, etc.)
- `commands/` — slash commands (`/run-prompt`, `/create-prompt`, `/code-review`, etc.)
- `agents/` — specialized agents (quality, security, coverage, etc.)

Update periodically:

```bash
cd ~/.claude-yolo && git pull
```

## Option 2: Create Your Own Config

### Minimum Required Files

Dark-factory needs exactly three things in `~/.claude-yolo/`:

**1. OAuth credentials** (created by `claude login`):

```bash
mkdir -p ~/.claude-yolo
cd ~/.claude-yolo
CLAUDE_CONFIG_DIR=~/.claude-yolo claude login
```

This creates `.credentials.json` (or `.claude.json` on older versions). Dark-factory validates the OAuth token before starting a container.

**2. `settings.json`** — skip the dangerous-mode confirmation dialog:

```bash
echo '{"skipDangerousModePermissionPrompt": true}' > ~/.claude-yolo/settings.json
```

**3. `CLAUDE.md`** — instructions for the autonomous agent:

```bash
cat > ~/.claude-yolo/CLAUDE.md << 'YOLOEOF'
# YOLO Container - Autonomous Execution Mode

You are running in an isolated Docker container with auto-approve enabled.

## Rules

- **NO** Claude attribution in commits (no "Generated with Claude Code", no "Co-Authored-By")
- Use `cd path && git ...` (NEVER `git -C /path`)
- Read project CLAUDE.md before making changes
- Run `make precommit` or `make test` before declaring complete
- Tests must pass before declaring complete

## Workflow

1. Read the prompt carefully
2. Read project CLAUDE.md for conventions
3. Implement all requirements
4. Run verification command from the prompt
5. Report what was implemented and any blockers
YOLOEOF
```

### Result

```
~/.claude-yolo/
├── .credentials.json   # OAuth token (from claude login)
├── settings.json       # {"skipDangerousModePermissionPrompt": true}
└── CLAUDE.md           # Agent instructions
```

That's it. Dark-factory will work with just these three files.

### Optional Enhancements

Add these as needed:

```
~/.claude-yolo/
├── docs/               # Coding guides the agent can reference
│   ├── go-testing.md
│   └── go-patterns.md
├── commands/           # Slash commands available in container
│   └── run-prompt.md
└── agents/             # Specialized agents
    └── simple-bash-runner.md
```

- **`docs/`** — coding guidelines. The agent can read these if your `CLAUDE.md` references them. Example: `"For Go: Read docs/go-testing.md"`
- **`commands/`** — slash commands. Not required by dark-factory (it passes prompts directly via `-p` flag), but useful for interactive container sessions.
- **`agents/`** — specialized sub-agents. Same as commands — optional.

## How Dark-Factory Uses the Config

Dark-factory mounts `~/.claude-yolo/` as `/home/node/.claude` inside the container:

```
Host                          Container
~/.claude-yolo/CLAUDE.md  →  /home/node/.claude/CLAUDE.md
~/.claude-yolo/docs/       →  /home/node/.claude/docs/
```

The mount path is configurable via the `DARK_FACTORY_CLAUDE_CONFIG_DIR` environment variable:

```bash
# Default: ~/.claude (override to use separate YOLO config)
export DARK_FACTORY_CLAUDE_CONFIG_DIR=~/.claude-yolo
```

**Important:** If you don't set this variable, dark-factory uses `~/.claude` (your main Claude Code config). Setting it to `~/.claude-yolo` keeps YOLO config isolated from your interactive sessions.

## Pulling the Container Image

```bash
docker pull docker.io/bborbe/claude-yolo:v0.2.9
```

Or build from source:

```bash
git clone https://github.com/bborbe/claude-yolo.git
cd claude-yolo
make build
```

## Verification

```bash
# Config dir exists with credentials
ls ~/.claude-yolo/.credentials.json 2>/dev/null || ls ~/.claude-yolo/.claude.json
cat ~/.claude-yolo/settings.json
cat ~/.claude-yolo/CLAUDE.md

# Container image available
docker images | grep claude-yolo

# Env var set (add to ~/.zshrc or ~/.bashrc)
echo $DARK_FACTORY_CLAUDE_CONFIG_DIR
```
