---
status: completed
---

# Core Pipeline: Watch, Execute, Commit

## Problem

Running AI coding prompts requires babysitting: open terminal, paste prompt, wait, review, commit, repeat. For a queue of 10+ prompts this takes hours of human attention for work that is fundamentally sequential and unattended.

## Goal

A daemon that watches a directory for markdown prompt files, executes each one in an isolated Docker container, and commits the result to git. Human drops files, walks away, comes back to committed code.

## Non-goals

- No web UI or dashboard
- No parallel execution (one prompt at a time)
- No automatic PR creation (direct commit to current branch)
- No configuration file (hardcoded defaults for MVP)
- No crash recovery (clean restarts assumed)

## Desired Behavior

1. Daemon starts, watches a directory for `.md` files
2. Files are processed in alphabetical order (filename determines priority)
3. Each file has YAML frontmatter with a `status` field
4. Only files with `status: queued` are picked up
5. Before execution, status is set to `executing` in the file's frontmatter
6. The prompt content (everything after frontmatter) is passed to a Docker container
7. The Docker container runs with the project root mounted as `/workspace`
8. After successful execution, status is set to `completed`
9. Changes made by the container are committed to git with the prompt title as commit message
10. After failure (non-zero exit), status is set to `failed`
11. Processing continues with the next queued file
12. A prompt is only processed if all lower-numbered prompts are already completed

## Constraints

- Prompt files are standard markdown with YAML frontmatter
- Docker container has NO git access — all git operations happen on the host
- Prompt content passed via file mount (read-only), not environment variable
- Container gets network capabilities (NET_ADMIN, NET_RAW) for package installation
- Go module cache (`~/go/pkg`) mounted for build performance

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Docker container exits non-zero | Set status=failed, log error, continue to next prompt | Manual fix + reset status |
| Prompt file has empty content | Move to completed without execution (no-op) | None needed |
| Previous prompt not completed | Skip, log "previous prompt not completed" | Process when predecessor completes |
| Git commit fails | Log error, prompt stays as executing | Manual intervention |

## Acceptance Criteria

- [ ] Daemon watches directory and detects new/changed `.md` files
- [ ] Only `status: queued` files are processed
- [ ] Status transitions: queued -> executing -> completed/failed
- [ ] Docker container receives prompt content and project workspace
- [ ] Git commit happens after successful container execution
- [ ] Failed containers don't block subsequent prompts (after manual reset)
- [ ] Processing order follows alphabetical filename sort

## Verification

Run `make precommit` — must pass.

## Do-Nothing Option

Continue babysitting each prompt manually. Works for 1-3 prompts but doesn't scale to 10+ prompt sequences.
