# YOLO Container Setup

Dark-factory executes prompts inside a [claude-yolo](https://github.com/bborbe/claude-yolo) Docker container. The container needs a Claude Code configuration directory (`~/.claude-yolo/`) on the host, mounted as `/home/node/.claude` inside the container.

Two options: use bborbe's config (includes plugins with coding guides, agents, and slash commands) or create your own minimal config.

## Option 1: Use bborbe's Config

Clone the repo and authenticate:

```bash
git clone https://github.com/bborbe/claude-yolo.git ~/.claude-yolo
cd ~/.claude-yolo
claude login
```

This gives you:
- `CLAUDE.md` — workflow instructions (no attribution, verification rules, completion protocol)
- `plugins/marketplaces/coding/` — coding plugin with 40+ guides (Go patterns, testing, security, Python, etc.), agents, and slash commands

Install the coding plugin:

```bash
claude plugins install coding
```

Update periodically:

```bash
cd ~/.claude-yolo && git pull
claude plugins update coding
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

Install the coding plugin for Go/Python coding guides, agents, and slash commands:

```bash
CLAUDE_CONFIG_DIR=~/.claude-yolo claude plugins install coding
```

This installs to `~/.claude-yolo/plugins/marketplaces/coding/` with:
- `docs/` — 40+ coding guides (Go patterns, testing, error handling, Python, etc.)
- `agents/` — specialized agents (quality, security, coverage, etc.)
- `commands/` — slash commands (`/code-review`, etc.)
- `skills/` — reusable skills

The agent can reference these guides in prompts. Example: `"Read the go-testing-guide.md from coding plugin for Ginkgo/Gomega patterns"`

## How Dark-Factory Uses the Config

Dark-factory mounts `~/.claude-yolo/` as `/home/node/.claude` inside the container:

```
Host                                                    Container
~/.claude-yolo/CLAUDE.md                            →  /home/node/.claude/CLAUDE.md
~/.claude-yolo/plugins/marketplaces/coding/docs/    →  /home/node/.claude/plugins/marketplaces/coding/docs/
```

The mount path defaults to `~/.claude-yolo` and is configurable per project via `.dark-factory.yaml`:

```yaml
claudeDir: ~/my-custom-claude-config
```

## Pulling the Container Image

```bash
docker pull docker.io/bborbe/claude-yolo:v0.3.1
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

```
