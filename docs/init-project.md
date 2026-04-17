# Initialize a Project for Dark Factory

Set up a new or existing project to use the dark-factory pipeline.

**Follow every step in order. Do not skip any step — including the smoke test.**

## Prerequisites

- **dark-factory** installed (`go install github.com/bborbe/dark-factory@latest`)
- **Docker** running
- **claude-yolo image** pulled (`docker pull docker.io/bborbe/claude-yolo:v0.3.1`)
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

## Step 3: Create Definition of Done

The config references `validationPrompt: docs/dod.md` — this file must exist or validation will fail.

```bash
mkdir -p docs
```

Create `docs/dod.md` with project-specific quality criteria. See [vault-cli/docs/dod.md](https://github.com/bborbe/vault-cli/blob/master/docs/dod.md) for a reference template covering:

- **Code Quality** — doc comments, error handling, factory patterns
- **Testing** — coverage targets, framework conventions
- **Install** — `go install` works, no `replace`/`exclude` in go.mod
- **Documentation** — README and CHANGELOG updated

Adapt each section to your project (e.g., remove CLI-specific checks for libraries, add service-specific checks for services).

## Step 4: Create Directories

```bash
mkdir -p prompts/in-progress prompts/completed prompts/log
mkdir -p specs/in-progress specs/completed specs/log
touch prompts/in-progress/.keep prompts/completed/.keep prompts/log/.keep
touch specs/in-progress/.keep specs/completed/.keep specs/log/.keep
```

`.keep` files ensure git tracks empty directories.

## Step 5: Update .gitignore

Add:

```
/.dark-factory.lock
/.dark-factory.log
/prompts/log
/specs/log
```

## Step 6: Add Dark Factory Section to CLAUDE.md

The project CLAUDE.md serves two audiences:
1. **Claude Code sessions** — humans working interactively, need workflow rules
2. **YOLO container** — autonomous agent executing prompts, needs project-specific dev context

See [claude-md-guide.md](claude-md-guide.md) for the full template and guidance.

At minimum, add:
- Dark Factory Workflow section (complete flow, commands, key rules)
- Development Standards section (build commands, test conventions, dependencies)
- Architecture section (package map)
- Key Design Decisions (constraints the agent must respect)

## Step 7: Commit

```bash
git add .dark-factory.yaml .gitignore prompts/ specs/ docs/dod.md CLAUDE.md
git commit -m "setup dark-factory config and directories"
```

## Step 8: Verify Setup

Run all checks — every line must succeed:

```bash
cat .dark-factory.yaml              # config exists
cat docs/dod.md                     # DoD exists
ls prompts/in-progress/.keep        # dirs tracked
ls specs/in-progress/.keep
grep dark-factory.lock .gitignore   # lock ignored
grep dark-factory.log .gitignore    # daemon log ignored
grep prompts/log .gitignore         # logs ignored
grep "Dark Factory" CLAUDE.md       # workflow section exists
```

**Do not proceed to step 9 until all checks pass.**

## Step 9: Smoke-Test Prompt (REQUIRED)

**This step is mandatory.** Run the smoke test before writing any real prompts. It confirms the pipeline works end-to-end (Docker, CLAUDE.md, build commands, YOLO container).

Copy the initial prompt template:

```bash
cp <dark-factory-docs>/init-prompt-fix-tests-and-dod.md prompts/fix-tests-and-dod.md
```

See [init-prompt-fix-tests-and-dod.md](init-prompt-fix-tests-and-dod.md) for the template. This prompt runs `make precommit`, fixes any failures, and checks the Definition of Done.

Approve and run it:

```bash
dark-factory prompt approve fix-tests-and-dod
dark-factory daemon
```

**Only write real prompts after the smoke test completes successfully.**

## Next Steps

- Configure your project: [configuration.md](configuration.md) — all config fields, validation, notifications, providers
- Write a spec: [spec-writing.md](spec-writing.md)
- Write a prompt: [prompt-writing.md](prompt-writing.md)
