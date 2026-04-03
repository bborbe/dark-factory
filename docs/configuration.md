# Configuration Guide

All dark-factory configuration lives in `.dark-factory.yaml` in the project root. Only add fields you want to override — defaults work for most projects.

## Minimal Config

```yaml
pr: false
worktree: false
```

This is the default: commits directly to the current branch, no PRs, no clone isolation.

## Workflow

Two booleans control how dark-factory handles git:

| `pr` | `worktree` | Behavior |
|------|-----------|----------|
| `false` | `false` | Commit directly to current branch (default) |
| `true` | `true` | Clone repo, create feature branch, open PR |
| `false` | `true` | Clone repo, commit to branch, no PR |
| `true` | `false` | Commit in-place, open PR |

```yaml
pr: true
worktree: true
```

**Legacy:** `workflow: direct` and `workflow: pr` still work but log a deprecation warning. Don't mix old and new fields — dark-factory rejects configs with both `workflow` and `pr`/`worktree` set.

### Auto-merge and Auto-release

```yaml
pr: true
worktree: true
autoMerge: true
autoRelease: true
```

| Field | Default | Purpose |
|-------|---------|---------|
| `autoMerge` | `false` | Merge PR automatically after checks pass (requires `pr: true`) |
| `autoRelease` | `false` | Tag and push a release after each prompt completes. When `false` (default), changes are committed with changelog under `## Unreleased` but not tagged or pushed. When `true`, tags and pushes after completion. Works in all workflows (direct, PR, branch). |

## Validation

Two complementary validation mechanisms run after each prompt completes:

### validationCommand (machine-judged)

A shell command whose exit code determines success or failure. Runs first.

```yaml
validationCommand: "make precommit"
```

Default: `make precommit`. Set to empty string to disable.

### validationPrompt (AI-judged)

Quality criteria that the AI agent reviews its own work against. This is **not** a command — it's text injected into the agent's prompt so it can self-evaluate the quality and completeness of its implementation.

**How it works:**

1. The agent finishes implementing the prompt
2. `validationCommand` runs (e.g., `make precommit`) — if it fails, the prompt fails
3. If `validationCommand` passes, the agent reads the `validationPrompt` criteria
4. The agent checks each criterion against its own changes (logic, coverage, docs)
5. Unmet criteria → `partial` status (work done, but quality not fully met)

**What goes in the criteria:** Things the AI should verify about its own work that linters and tests can't catch — did it update docs, does the logic handle edge cases, is test coverage sufficient, are error messages clear. Think of it as a code review checklist the AI applies to itself.

The value can be:

- **File path** — loads criteria from file (any format Claude Code understands, `.md` recommended)
- **Inline text** — used directly as criteria

```yaml
# File path (recommended for detailed criteria)
validationPrompt: docs/dod.md

# Inline text (for simple checks)
validationPrompt: "README.md is updated"
```

If the file doesn't exist, dark-factory logs a warning and continues without evaluation.

See `docs/dod.md` for a starting point.

**Constraints:** Must be a relative path. Absolute paths and `..` traversal are rejected.

## Code Review

Automated PR review handling:

```yaml
pr: true
autoReview: true
maxReviewRetries: 3
allowedReviewers:
  - "username1"
  - "username2"
useCollaborators: false
pollIntervalSec: 60
```

| Field | Default | Purpose |
|-------|---------|---------|
| `autoReview` | `false` | Poll PR for reviews and auto-fix requested changes |
| `maxReviewRetries` | `3` | Max fix attempts before notifying human |
| `allowedReviewers` | (empty) | Only act on reviews from these users |
| `useCollaborators` | `false` | Fetch allowed reviewers from repo collaborators |
| `pollIntervalSec` | `60` | How often to poll for new reviews (seconds) |
| `verificationGate` | `false` | Block until CI checks pass before proceeding |

## Git Provider

Default provider is GitHub (uses `gh` CLI). Bitbucket Server is also supported.

### GitHub (default)

```yaml
provider: github
github:
  token: ""  # optional, uses gh CLI auth by default
```

### Bitbucket Server

```yaml
provider: bitbucket-server
defaultBranch: master
bitbucket:
  baseURL: https://bitbucket.example.com
  tokenEnv: BITBUCKET_TOKEN
```

| Field | Default | Purpose |
|-------|---------|---------|
| `provider` | `github` | Git provider: `github` or `bitbucket-server` |
| `defaultBranch` | (auto-detected) | Required for Bitbucket (no auto-detection) |
| `bitbucket.baseURL` | (empty) | Bitbucket Server URL (required when provider is `bitbucket-server`) |
| `bitbucket.tokenEnv` | `BITBUCKET_TOKEN` | Env var name containing the API token |

**Note:** `tokenEnv` stores the env var *name*, not the token itself — config stays safe to commit.

## Notifications

Dark-factory notifies when human attention is needed (failures, stuck containers, specs ready for verification). Both channels can fire simultaneously.

```yaml
notifications:
  telegram:
    botTokenEnv: TELEGRAM_BOT_TOKEN
    chatIDEnv: TELEGRAM_CHAT_ID
  discord:
    webhookEnv: DISCORD_WEBHOOK_URL
```

A channel is active if its env var resolves to a non-empty value. No env var = no notifications from that channel.

| Field | Purpose |
|-------|---------|
| `telegram.botTokenEnv` | Env var name for Telegram bot token |
| `telegram.chatIDEnv` | Env var name for Telegram chat ID |
| `discord.webhookEnv` | Env var name for Discord webhook URL (must be HTTPS) |

**Note:** Config stores env var *names*, not secrets. Failed delivery logs a warning but never blocks processing.

## Container

```yaml
containerImage: "docker.io/bborbe/claude-yolo:v0.5.1"
model: "claude-sonnet-4-6"
```

| Field | Default | Purpose |
|-------|---------|---------|
| `containerImage` | `docker.io/bborbe/claude-yolo:v0.5.1` | Docker image for YOLO execution |
| `model` | `claude-sonnet-4-6` | Claude model used inside the container |

## Per-Project Container Limit

Override the global `maxContainers` limit for a specific project by adding `maxContainers` to `.dark-factory.yaml`:

```yaml
# Priority project: allow up to 5 containers
maxContainers: 5

# Background project: restrict to 1 container at a time
maxContainers: 1
```

| Field | Default | Purpose |
|-------|---------|---------|
| `maxContainers` | (global limit) | Override the system-wide container limit for this project. Missing or 0 falls back to the global limit from `~/.dark-factory/config.yaml` (default: 3). Must be ≥ 1 if set. |

Counting remains system-wide (all running dark-factory containers across all projects). Only the threshold is per-project. Two projects both set to 5 can together exceed any single limit — this is intentional.

### CLI Override

Override the limit for a single run without editing config:

```bash
dark-factory run --max-containers 5
dark-factory daemon --max-containers 1
```

Priority: CLI arg > project config > global config > default (3).

## Private Go Modules

For projects with private dependencies:

```yaml
gitconfigFile: ~/.claude-yolo/.gitconfig
netrcFile: ~/.claude-yolo/.netrc
env:
  GOPRIVATE: "your.private.host/*"
  GONOSUMCHECK: "your.private.host/*"
```

| Field | Default | Purpose |
|-------|---------|---------|
| `gitconfigFile` | (empty) | `.gitconfig` mounted into the container |
| `netrcFile` | (empty) | `.netrc` mounted into the container |
| `env` | (empty) | Env vars passed to the container |
| `extraMounts` | (empty) | Additional volume mounts injected into the container |
| `additionalInstructions` | (empty) | Text prepended to every prompt and spec generation command |

## Extra Mounts

Share documentation or config directories across repos without duplicating them:

```yaml
extraMounts:
  - src: ../docs/howto
    dst: /docs
  - src: ~/Documents/workspaces/coding/docs
    dst: /coding-docs
    readonly: true
```

| Field | Required | Default | Purpose |
|-------|----------|---------|---------|
| `src` | yes | — | Host path. Environment variables (`$VAR`, `${VAR}`) are expanded. `~/` expanded to home. Relative paths resolved from project root. |
| `dst` | yes | — | Container path where `src` is mounted. |
| `readonly` | no | `true` | Mount read-only (`:ro`). Set `false` for writable access. |

Missing `src` paths at execution time are logged as a warning and skipped — they do not abort the run.

Examples using environment variables:

```yaml
# Go module cache (uses GOPATH env var)
extraMounts:
  - src: ${GOPATH}/pkg
    dst: /home/node/go/pkg

# Python uv cache
extraMounts:
  - src: ~/.cache/uv
    dst: /home/node/.cache/uv
```

## Additional Instructions

Inject project-level context into every prompt without repeating it in each prompt's `<context>` section:

```yaml
additionalInstructions: |
  Read shared documentation at /docs for coding guidelines.
  Follow conventions in /docs/go-testing-guide.md for all test code.
```

The text is prepended before all other prompt content and before any spec generation command. When empty or absent, no content is injected.

Combine with `extraMounts` to mount and reference shared documentation:

```yaml
extraMounts:
  - src: ~/Documents/workspaces/coding/docs
    dst: /docs
additionalInstructions: |
  Read /docs/go-testing-guide.md before writing any tests.
```

## Directory Layout

Customizable but rarely needed:

```yaml
prompts:
  inboxDir: prompts
  inProgressDir: prompts/in-progress
  completedDir: prompts/completed
  logDir: prompts/log
specs:
  inboxDir: specs
  inProgressDir: specs/in-progress
  completedDir: specs/completed
  logDir: specs/log
```

## Advanced

| Field | Default | Purpose |
|-------|---------|---------|
| `projectName` | (auto-detected) | Override project name in notifications and logs |
| `debounceMs` | `500` | File watcher debounce in milliseconds |
| `serverPort` | `0` | REST API port (0 = disabled) |

## Full Example

```yaml
pr: true
worktree: true
autoMerge: true
autoReview: true
maxReviewRetries: 3
defaultBranch: master
validationCommand: "make precommit"
validationPrompt: docs/dod.md
provider: github
containerImage: "docker.io/bborbe/claude-yolo:v0.5.1"
model: "claude-sonnet-4-6"
maxContainers: 5
notifications:
  telegram:
    botTokenEnv: TELEGRAM_BOT_TOKEN
    chatIDEnv: TELEGRAM_CHAT_ID
env:
  GOPRIVATE: "github.com/myorg/*"
extraMounts:
  - src: ~/Documents/workspaces/coding/docs
    dst: /coding-docs
additionalInstructions: |
  Read /coding-docs/go-testing-guide.md before writing tests.
```
