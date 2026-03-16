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

When using PRs:

```yaml
pr: true
worktree: true
autoMerge: true
autoRelease: true
```

| Field | Default | Purpose |
|-------|---------|---------|
| `autoMerge` | `false` | Merge PR automatically after checks pass |
| `autoRelease` | `false` | Create release after merge (requires `autoMerge`) |

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
containerImage: "docker.io/bborbe/claude-yolo:v0.3.0"
model: "claude-sonnet-4-6"
```

| Field | Default | Purpose |
|-------|---------|---------|
| `containerImage` | `docker.io/bborbe/claude-yolo:v0.3.0` | Docker image for YOLO execution |
| `model` | `claude-sonnet-4-6` | Claude model used inside the container |

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
containerImage: "docker.io/bborbe/claude-yolo:v0.3.0"
model: "claude-sonnet-4-6"
notifications:
  telegram:
    botTokenEnv: TELEGRAM_BOT_TOKEN
    chatIDEnv: TELEGRAM_CHAT_ID
env:
  GOPRIVATE: "github.com/myorg/*"
```
