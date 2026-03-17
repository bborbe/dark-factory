# Initialize a Project for Dark Factory

Set up a new or existing project to use the dark-factory pipeline.

## Prerequisites

- **dark-factory** installed (`go install github.com/bborbe/dark-factory@latest`)
- **Docker** running
- **claude-yolo image** pulled (`docker pull docker.io/bborbe/claude-yolo:v0.3.0`)
- **~/.claude-yolo/** configured (see main README)

## Step 1: Choose Workflow

| `pr` | `worktree` | When to use | Git behavior |
|------|-----------|-------------|-------------|
| `false` | `false` | Small projects, no branch protection | Commits to current branch |
| `true` | `true` | PR-based projects, libraries | Clone repo, feature branch + PR |

## Step 2: Create Config

Create `.dark-factory.yaml` in the project root. Only add non-default values.

**Direct workflow (default):**

```yaml
pr: false
worktree: false
validationPrompt: docs/dod.md
```

**PR workflow:**

```yaml
pr: true
worktree: true
autoMerge: true
validationPrompt: docs/dod.md
```

**With private Go modules:**

```yaml
pr: false
worktree: false
validationPrompt: docs/dod.md
defaultBranch: master
gitconfigFile: ~/.claude-yolo/.gitconfig
netrcFile: ~/.claude-yolo/.netrc
env:
  GOPRIVATE: "your.private.host/*"
  GONOSUMCHECK: "your.private.host/*"
```

See [configuration.md](configuration.md) for all config fields.

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

## Step 7: Smoke-Test Prompt

Copy the initial prompt template to validate the setup works end-to-end:

```bash
cp <dark-factory-docs>/init-prompt-fix-tests-and-dod.md prompts/fix-tests-and-dod.md
```

See [init-prompt-fix-tests-and-dod.md](init-prompt-fix-tests-and-dod.md) for the template. This prompt runs `make precommit`, fixes any failures, and checks the Definition of Done — confirming the pipeline works before you write real prompts.

## Next Steps

- Configure your project: [configuration.md](configuration.md) — all config fields, validation, notifications, providers
- Write a spec: [spec-writing.md](spec-writing.md)
- Write a prompt: [prompt-writing.md](prompt-writing.md)
