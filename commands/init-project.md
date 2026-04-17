---
description: Initialize a project for dark-factory prompt execution
argument-hint: [direct|pr|worktree]
allowed-tools: [Read, Write, Edit, Bash, Glob, AskUserQuestion]
---

Set up the current project for dark-factory. Read `docs/init-project.md` and `docs/claude-md-guide.md` from the dark-factory installation first, then execute each step:

1. **Check prerequisites**:
   - `.dark-factory.yaml` must NOT already exist (if it does, inform user and stop)
   - Verify `dark-factory` is installed: `which dark-factory`
   - Verify Docker is running: `docker info >/dev/null 2>&1`

2. **Choose workflow** from $ARGUMENTS or ask user:
   - `direct` — small projects, no branch protection, commits to current branch
   - `pr` — PR-based projects, feature branch + PR
   - `worktree` — worktree isolation + PR (recommended for libraries/services)

3. **Create `.dark-factory.yaml`** with chosen workflow (only non-default values)

4. **Create directories**:
   ```bash
   mkdir -p prompts/in-progress prompts/completed prompts/log
   mkdir -p specs/in-progress specs/completed specs/log
   touch prompts/in-progress/.keep prompts/completed/.keep prompts/log/.keep
   touch specs/in-progress/.keep specs/completed/.keep specs/log/.keep
   ```

5. **Update `.gitignore`** — add if missing:
   ```
   /.dark-factory.lock
   /.dark-factory.log
   /prompts/log
   /specs/log
   ```

6. **Add Dark Factory Workflow section to CLAUDE.md** — follow the template from `docs/claude-md-guide.md`. Include both the workflow section (for Claude Code) and development standards (for YOLO container).

7. **Verify** setup:
   ```bash
   cat .dark-factory.yaml
   ls prompts/in-progress/.keep
   ls specs/in-progress/.keep
   grep dark-factory.lock .gitignore
   grep dark-factory.log .gitignore
   grep "Dark Factory" CLAUDE.md
   ```

8. **Commit** the setup files (ask user first).
