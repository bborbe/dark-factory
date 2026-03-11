# Initialize a Project for Dark Factory

Set up a new or existing project to use the dark-factory pipeline.

## Prerequisites

- **dark-factory** installed (`go install github.com/bborbe/dark-factory@latest`)
- **Docker** running
- **claude-yolo image** pulled (`docker pull docker.io/bborbe/claude-yolo:v0.2.5`)
- **~/.claude-yolo/** configured (see main README)

## Step 1: Choose Workflow

| Workflow | When to use | Git behavior |
|----------|-------------|-------------|
| `direct` | Small projects, no branch protection | Commits to current branch |
| `pr` | PR-based projects, libraries | Feature branch + PR |

## Step 2: Create Config

Create `.dark-factory.yaml` in the project root. Only add non-default values.

**Direct workflow (default):**

```yaml
workflow: direct
```

**PR workflow:**

```yaml
workflow: pr
autoMerge: true
```

**With private Go modules:**

```yaml
workflow: direct
defaultBranch: master
gitconfigFile: ~/.claude-yolo/.gitconfig
netrcFile: ~/.claude-yolo/.netrc
env:
  GOPRIVATE: "your.private.host/*"
  GONOSUMCHECK: "your.private.host/*"
```

## Step 3: Create Directories

```bash
mkdir -p prompts/in-progress prompts/completed prompts/log
mkdir -p specs/in-progress specs/completed specs/log
touch prompts/in-progress/.keep prompts/completed/.keep prompts/log/.keep
touch specs/in-progress/.keep specs/completed/.keep specs/log/.keep
```

`.keep` files ensure git tracks empty directories.

## Step 4: Update .gitignore

Add:

```
/.dark-factory.lock
/prompts/log
/specs/log
```

## Step 5: Add Dark Factory Section to CLAUDE.md

The project CLAUDE.md serves two audiences:
1. **Claude Code sessions** — humans working interactively, need workflow rules
2. **YOLO container** — autonomous agent executing prompts, needs project-specific dev context

See [claude-md-guide.md](claude-md-guide.md) for the full template and guidance.

At minimum, add:
- Dark Factory Workflow section (complete flow, commands, key rules)
- Development Standards section (build commands, test conventions, dependencies)
- Architecture section (package map)
- Key Design Decisions (constraints the agent must respect)

## Step 6: Commit

```bash
git add .dark-factory.yaml .gitignore prompts/ specs/ CLAUDE.md
git commit -m "setup dark-factory config and directories"
```

## Verification

```bash
cat .dark-factory.yaml              # config exists
ls prompts/in-progress/.keep        # dirs tracked
ls specs/in-progress/.keep
grep dark-factory.lock .gitignore   # lock ignored
grep prompts/log .gitignore         # logs ignored
grep "Dark Factory" CLAUDE.md       # workflow section exists
```

## Config Reference

| Field | Default | Purpose |
|-------|---------|---------|
| `workflow` | `direct` | `direct` or `pr` |
| `defaultBranch` | (auto-detected) | Required for non-GitHub repos |
| `autoMerge` | `false` | Auto-merge PR after checks (requires `pr`) |
| `autoRelease` | `false` | Create release after merge (requires `autoMerge`) |
| `containerImage` | `docker.io/bborbe/claude-yolo:v0.2.5` | YOLO Docker image |
| `model` | `claude-sonnet-4-6` | Claude model |
| `debounceMs` | `500` | Watcher debounce in ms |
| `serverPort` | `0` | REST API port (0 = disabled) |
| `gitconfigFile` | (empty) | .gitconfig mounted into container |
| `netrcFile` | (empty) | .netrc mounted into container |
| `env` | (empty) | Env vars passed to container |

## Next Steps

- Write a spec: [spec-writing.md](spec-writing.md)
- Write a prompt: [prompt-writing.md](prompt-writing.md)
