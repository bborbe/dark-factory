# Configuration Guide

All dark-factory configuration lives in `.dark-factory.yaml` in the project root. Only add fields you want to override — defaults work for most projects.

## Minimal Config

```yaml
workflow: direct
```

This is the default: commits directly to the current branch, no PRs, no isolation.

## Workflow

Two dimensions control git behavior: **separation** (`workflow:` enum) and **delivery** (`pr`, `autoMerge`, `autoRelease` booleans).

```yaml
workflow: branch
pr: true
autoMerge: true
autoRelease: true
```

| Field | Values | Purpose |
|-------|--------|---------|
| `workflow` | `direct` (default) \| `branch` \| `worktree` \| `clone` | Isolation level |
| `pr` | `false` (default) \| `true` | Push branch and open PR (incompatible with `workflow: direct`) |
| `autoMerge` | `false` (default) \| `true` | Merge PR automatically after checks pass (requires `pr: true`) |
| `autoRelease` | `false` (default) \| `true` | Push commits; tag release when `CHANGELOG.md` exists |

For the full matrix, container semantics, and choosing a mode, see [workflows.md](workflows.md).

**Legacy fields** still parse but log a deprecation warning:
- `worktree: bool` — mapped to a `workflow:` value (see [workflows.md](workflows.md#migration-from-legacy-config))
- `workflow: pr` — mapped to `workflow: clone` + `pr: true`

`autoRelease` semantics: When `false` (default), commits stay local (no push, no tag). When `true` and `CHANGELOG.md` exists, commits are pushed AND `## Unreleased` is bumped to `## vX.Y.Z` with a tag pushed. When `true` without `CHANGELOG.md`, commits are pushed but no tag is created. Works in all workflows.

`dark-factory prompt complete <id>` honours `autoRelease` and adds a branch-context safety default: on any non-`master` branch, completion commits but does NOT release, regardless of `autoRelease`, unless the operator passes `--release` explicitly. The flag overrides both the branch default and `autoRelease=false`. See [running.md § prompt complete --release](running.md#prompt-complete---release) for the operator-facing description.

## Validation

Two complementary validation mechanisms run after each prompt completes:

### testCommand (fast iteration feedback)

A shell command the YOLO agent runs repeatedly during development for fast build/test feedback. Unlike `validationCommand`, this is not authoritative — it is a development aid that runs frequently while the agent iterates on code changes.

```yaml
testCommand: "make test"
```

Default: `make test`. Set to empty string to disable injection entirely.

### validationCommand (machine-judged)

A shell command whose exit code determines success or failure. Runs exactly once at the end as the authoritative final gate — not during iteration.

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
backend: docker    # docker (default) | local
containerImage: "docker.io/bborbe/claude-yolo:v0.11.1"
model: "claude-sonnet-4-6"
```

| Field | Default | Purpose |
|-------|---------|---------|
| `backend` | `docker` | Execution backend. `docker` runs each LLM step in a claude-yolo container; `local` runs `claude` as an in-process subprocess in cwd — no docker daemon required, for already-isolated single-tenant pods only (`containerImage` is ignored). See [execution-backends.md](execution-backends.md#the-local-backend-backend-local). |
| `containerImage` | `docker.io/bborbe/claude-yolo:v0.11.1` | Docker image for YOLO execution (`backend: docker` only) |
| `model` | `claude-sonnet-4-6` | Claude model used by the agent |

## Global Config

Machine-wide user preferences live in `~/.config/dark-factory/config.yaml` (XDG). If that file does not exist and `~/.dark-factory/config.yaml` (legacy) is present, the legacy file is read as a fallback. This file is optional — when absent, all defaults apply and no behavior changes.

```yaml
# ~/.config/dark-factory/config.yaml  (XDG; ~/.dark-factory/config.yaml also read if XDG path absent)
maxContainers: 5
model: claude-opus-4-7
hideGit: true
autoRelease: true
dirtyFileThreshold: 20
```

### Precedence

For user-pref fields, the effective value follows this chain (last writer wins):

```
default  ←  global config  ←  project config  ←  CLI flag
```

- **Default**: hardcoded in `config.Defaults()` (e.g. `model: claude-sonnet-4-6`)
- **Global**: `~/.config/dark-factory/config.yaml` (XDG; falls back to `~/.dark-factory/config.yaml` if absent) — applies to all projects on this machine
- **Project**: `.dark-factory.yaml` in the repo root — overrides global for this project
- **CLI flag**: `--model NAME`, `--max-containers N`, or `--set <key>=<value>` — overrides yaml for this invocation only

Field absent at a layer means that layer is skipped — it never silently zeroes an upstream value.

For a full description of the 5-layer model and which fields belong at which layer, see [docs/config-layering.md](config-layering.md).

### Layered fields (phase 1)

The following fields are currently eligible for global config:

| Field | Default | Description |
|-------|---------|-------------|
| `maxContainers` | `3` | System-wide container concurrency limit |
| `model` | `claude-sonnet-4-6` | Claude model for all projects on this machine |
| `hideGit` | `false` | Hide git status from YOLO container by default |
| `autoRelease` | `false` | Auto-push commits and tag releases |
| `dirtyFileThreshold` | `0` (disabled) | Skip prompts when dirty file count exceeds this |
| `env` | (none) | Environment variables injected into the YOLO container; key-level merge with project `env:` (project wins on collision) |

All values in `~/.config/dark-factory/config.yaml` are optional (or `~/.dark-factory/config.yaml` if using the legacy path). Set only the ones you want to override.

### Validation

Global config is validated at startup. Invalid values fail startup with a clear error:

```
error: globalconfig: validate: globalconfig: dirtyFileThreshold must not be negative, got -1
```

The global config file itself is not validated before that — errors in the file (invalid YAML, unknown fields ignored by yaml.v3) surface at startup.

Model values must match `^[a-zA-Z0-9._:/-]{1,256}$`. Shell metacharacters (spaces, semicolons, pipes, dollar signs, etc.) are rejected because the model name flows to container args.

### Source tracing

The `effective config` log line emitted at startup includes a `*Source` field for each layered field:

```
msg="effective config" model=claude-opus-4-7 modelSource=global hideGit=true hideGitSource=global ...
```

Possible values: `default`, `global`, `project`, `arg`.

### Global env

Set machine-wide environment variables that every project on this machine will receive:

```yaml
# ~/.config/dark-factory/config.yaml  (XDG)
env:
  ANTHROPIC_BASE_URL: https://my-provider.example.com/v1
  ANTHROPIC_API_KEY: sk-ant-your-key-here   # only safe here — this file is not committed
```

**Key-level merge**: the project `.dark-factory.yaml` can add or override individual keys without replacing the whole map. Project values win on collision. Non-overlapping keys from both layers are passed to the container.

```yaml
# .dark-factory.yaml  (project file — committed to git)
env:
  GOPATH: /home/node/go   # adds a key; global keys still flow through
```

**Key name rules**: keys must match `^[A-Z_][A-Z0-9_]*$`. Any key that does not match causes config load to fail with an error naming the offending key.

**Secrets**: literal secret values (API keys, tokens) are permitted in the global home file because it is never committed. Never put literal secrets in `.dark-factory.yaml` — that file is tracked by git and may be read by anyone with repo access.

**Permission warning**: if `~/.config/dark-factory/config.yaml` has group or world read/write permissions, dark-factory logs a warning at startup and recommends `chmod 600 ~/.config/dark-factory/config.yaml`. Loading continues regardless.

**Effective config log**: the startup `effective config` log line reports `envFromGlobal`, `envProjectOverrides`, and `envProjectOnly` showing which keys came from which layer. Values are never logged.

### Routing via claude-code-router

`claude-yolo` v0.12.0+ honours `ANTHROPIC_BASE_URL`, so a local [claude-code-router](https://github.com/musistudio/claude-code-router) instance can serve non-Anthropic models (MiniMax, DeepSeek, GLM, etc.) to dark-factory containers without code changes.

```yaml
# ~/.config/dark-factory/config.yaml  (XDG)
maxContainers: 5
model: MiniMax-M2.7-highspeed
env:
  ANTHROPIC_BASE_URL: http://host.docker.internal:8788
```

- `model` is forwarded as-is to claude-yolo and must match a model the router exposes.
- `host.docker.internal` resolves to the macOS/Windows host from inside the container; on Linux add `--add-host=host.docker.internal:host-gateway` to the docker runtime or use the host LAN IP.
- The router runs on the host; start it before `dark-factory run`. The port (`8788` above) must match the router's listener.
- No `ANTHROPIC_API_KEY` is needed when the router handles upstream auth; keep your real Anthropic key out of this file in that case.

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
| `maxContainers` | (global limit) | Override the system-wide container limit for this project. Missing or 0 falls back to the global limit from `~/.config/dark-factory/config.yaml` (default: 3). Must be ≥ 1 if set. |

Counting remains system-wide (all running dark-factory containers across all projects). Only the threshold is per-project. Two projects both set to 5 can together exceed any single limit — this is intentional.

### Dirty File Threshold

Skip prompt execution when the git working tree has too many dirty (uncommitted) files.
Useful for large repos where dirty vendor directories cause slow `git status` inside containers.

```yaml
dirtyFileThreshold: 50
```

| Field | Default | Purpose |
|-------|---------|---------|
| `dirtyFileThreshold` | `0` (disabled) | Skip prompt execution when dirty file count exceeds this value. `0` disables the check. When exceeded, the prompt is skipped (not failed) and re-checked on the next poll cycle. User must clean up dirty files manually — no auto-cleanup. |

### Prompt Timeout

Limit how long a single prompt execution may run before it is killed and marked failed.

```yaml
maxPromptDuration: "90m"
```

| Field | Default | Purpose |
|-------|---------|---------|
| `maxPromptDuration` | `""` (disabled) | Maximum wall-clock time allowed for a single YOLO container execution. Empty string or omitted means no timeout. Accepts Go duration strings: `"30m"`, `"2h"`, `"90m"`. Invalid strings are rejected at daemon startup. |

### Auto-Retry

Automatically re-queue failed prompts up to a fixed number of times before marking them `failed`.

```yaml
autoRetryLimit: 3
```

| Field | Default | Purpose |
|-------|---------|---------|
| `autoRetryLimit` | `0` (disabled) | Number of automatic retries after a prompt fails. `0` disables auto-retry. When the retry count is exhausted the prompt transitions to `failed` and stops being retried automatically. |

### Queue and Sweep Intervals

```yaml
queueInterval: "5s"
sweepInterval: "60s"
idleLogInterval: "1m"
```

| Field | Default | Purpose |
|-------|---------|---------|
| `queueInterval` | `5s` | How often the daemon polls for queued prompts and re-checks committing prompts. Lower values give faster response to fsnotify-missed events at the cost of more frequent file scans. |
| `sweepInterval` | `60s` | How often the daemon scans `specs/in-progress/` for prompted specs whose linked prompts have all completed and transitions them to `verifying`. Self-healing safety net for the per-prompt auto-complete path; lower values give faster recovery from missed transitions. |
| `idleLogInterval` | `1m` | How often the daemon emits a heartbeat `"nothing to do, waiting for changes"` log line while idle. The first-entry line always fires immediately when the daemon enters an idle window. Set to `"0"` to disable the heartbeat entirely (only the first-entry line fires). Operators can raise this to reduce log volume during long idle periods. |

`queueInterval` and `sweepInterval` accept Go duration strings (`"5s"`, `"60s"`, `"5m"`, `"1h"`). Invalid strings or non-positive durations are rejected at daemon startup. `idleLogInterval` also accepts Go duration strings; `"0"` is valid and disables the heartbeat.

### Preflight Baseline Check

Run the project's baseline validation command on a clean tree before each prompt executes.
Prompts only start on a known-green baseline. When the baseline is broken, queued prompts
remain queued (no retry count increment, no status change), and the operator is notified.

```yaml
preflightCommand: "make precommit"
preflightInterval: "8h"
```

| Field | Default | Purpose |
|-------|---------|---------|
| `preflightCommand` | `make precommit` | Shell command run inside the container before each prompt. Empty string disables preflight entirely. |
| `preflightInterval` | `8h` | How long a successful preflight result is cached. After the daemon runs preflight once and it passes, prompts within the interval reuse that result — git commits between prompts do NOT invalidate the cache. Re-runs happen when the interval elapses, when the daemon restarts, or after a failed preflight (failures are never cached, so an operator fix is picked up on the next prompt). Accepts Go duration strings: `"30m"`, `"2h"`, `"8h"`. Invalid strings are rejected at daemon startup. |

**Caching:** Preflight runs at most once per `preflightInterval` after a successful check. Sequential prompts within the interval reuse the cached result without re-running the command.

**On failure:** The daemon logs the command, its captured output, and the commit SHA that was checked. A notification is sent. The prompt remains queued.

**Override:** Pass `--skip-preflight` to `run` or `daemon` to bypass preflight for a single invocation — see [CLI Flags](#cli-flags) below.

### Healthcheck Startup Gate

On `dark-factory daemon` start, run the same probe sequence as `dark-factory healthcheck`
(Docker daemon, container image, container boot, Claude session, workspace mount, `gh` auth
when `pr: true`, notification channels when configured) once before the prompt-watch loop
begins. This re-validates the pipeline stack the daemon depends on, complementing the
project-level `preflightCommand` (which only proves the project compiles in-container).

The gate runs only in `daemon` mode — `run` (one-shot) is unaffected.

```yaml
healthcheckEnabled: true
healthcheckInterval: "8h"
```

| Field | Default | Purpose |
|-------|---------|---------|
| `healthcheckEnabled` | `true` | Whether the daemon runs the healthcheck startup gate. Set `false` to disable the gate entirely — the daemon proceeds directly to the watch loop. |
| `healthcheckInterval` | `8h` | How long a successful healthcheck result is cached. A daemon restart within the interval skips the probes (cache hit). Failures are never cached, so an operator fix (image rebuild, token rotation) is picked up on the next start. Accepts Go duration strings: `"30m"`, `"2h"`, `"8h"`. Invalid strings are rejected at daemon startup. |

**Caching:** Successful results are cached on the host under `~/.dark-factory/healthcheck-cache/`, keyed by container image + project + interval, so different repos never collide. Only successes are cached; a corrupt or future-dated cache file is treated as a miss and the gate re-runs.

**On failure:** the daemon exits non-zero with a category-naming cause (e.g. `healthcheck failed: docker daemon unreachable`), fires a notification, and does NOT cache the result — matching the preflight terminal-failure policy.

**Source precedence:** the effective-config startup log reports `healthcheckEnabled`/`healthcheckInterval` together with `healthcheckEnabledSource`/`healthcheckIntervalSource` (`default`, `project`, or `arg`) so the active gate config is auditable.

**Override:** pass `--skip-healthcheck` to `daemon` to bypass the gate for a single invocation — see [CLI Flags](#cli-flags) below.

### CLI Flags

Override settings for a single run without editing config:

**`--max-containers N`**

```bash
dark-factory run --max-containers 5
dark-factory daemon --max-containers 1
```

Priority: CLI arg > project config > global config > default (3).

**`--skip-preflight`**

```bash
dark-factory run --skip-preflight
dark-factory daemon --skip-preflight
```

Bypasses the preflight baseline check for this invocation. When set:

- The configured `preflightCommand` is not executed.
- No preflight cache is read or written.
- No baseline-failure report is emitted.
- Prompts proceed directly to normal execution.
- A startup log line records that preflight was skipped.

The flag is position-agnostic: `dark-factory --skip-preflight run` and `dark-factory run --skip-preflight` are equivalent.

**Safety note:** Prompts may run on a broken baseline when this flag is used. The startup log line provides an audit trail. Use only when the baseline is knowingly broken (e.g., transient CVE, upstream flake) and the prompt must execute urgently.

The flag does not persist: the next invocation without the flag runs preflight as configured. It has no effect when `preflightCommand` is empty (already disabled).

**`--skip-healthcheck`**

```bash
dark-factory daemon --skip-healthcheck
```

Bypasses the healthcheck startup gate for this invocation (daemon only — the flag is rejected on other commands). When set:

- The probe sequence is not executed.
- No healthcheck cache is read or written.
- The daemon proceeds directly to the watch loop.
- A startup log line records that the healthcheck was skipped.

The flag is position-agnostic: `dark-factory --skip-healthcheck daemon` and `dark-factory daemon --skip-healthcheck` are equivalent.

**Safety note:** the daemon may run prompts against a broken pipeline stack when this flag is used; a stack failure then surfaces mid-prompt instead of at startup. Use only as an explicit override. The flag does not persist; the next invocation runs the gate as configured.

**`--model NAME`**

```bash
dark-factory run --model claude-haiku-4-5
dark-factory daemon --model claude-opus-4-7
dark-factory run --model docker.io/bborbe/claude-yolo:v0.6.1
```

Overrides the model for this invocation. Beats both global and project config.

`NAME` must match `^[a-zA-Z0-9._:/-]{1,256}$`. Values with spaces, semicolons, pipes, or other shell metacharacters are rejected.

Priority: `--model` arg > project config > global config > default.

**`--set key=value`**

```bash
dark-factory run --set hideGit=true
dark-factory run --set dirtyFileThreshold=5
dark-factory run --set model=claude-opus-4-7
dark-factory daemon --set autoRelease=false --set model=claude-haiku-4-5
dark-factory run --set workflow=branch --set pr=true
dark-factory run --set autoMerge=true
```

Overrides any supported config field for this invocation. The flag may appear multiple times; if the same key appears more than once, the last occurrence wins.

Supported keys and types:

| Key | Type | Example |
|-----|------|---------|
| `hideGit` | bool (`true` or `false`) | `--set hideGit=true` |
| `autoRelease` | bool (`true` or `false`) | `--set autoRelease=false` |
| `dirtyFileThreshold` | int ≥ 0 | `--set dirtyFileThreshold=5` |
| `model` | string (must match `^[a-zA-Z0-9._:/-]{1,256}$`) | `--set model=claude-opus-4-7` |
| `maxContainers` | int ≥ 1 | `--set maxContainers=2` |
| `workflow` | enum (`direct` \| `branch` \| `worktree` \| `clone`) | `--set workflow=branch` |
| `pr` | bool (`true` or `false`) | `--set pr=true` |
| `autoMerge` | bool (`true` or `false`) | `--set autoMerge=false` |
| `autoGeneratePrompts` | bool (`true` or `false`) | `--set autoGeneratePrompts=true` |

When set to `true`, the spec watcher will NOT auto-fire the generator container when a spec is approved. Use `/dark-factory:generate-prompts-for-spec <spec-path>` to trigger generation manually.

Bool fields accept only `true` or `false` (case-sensitive). Values like `1`, `0`, `yes`, `no` are rejected. Unknown keys exit non-zero with an error listing the supported keys.

The `workflow: pr` legacy yaml value is **not** accepted via `--set`. Use `--set workflow=clone --set pr=true` instead (the yaml loader maps the legacy value at load time; the arg layer intentionally does not reproduce that mapping).

Priority: `--set` arg > project config > global config > default.

### Auto Prompt Generation

Control whether the daemon auto-generates prompts when a spec moves to `status: approved`.

```yaml
autoGeneratePrompts: true
```

| Field | Default | Purpose |
|-------|---------|---------|
| `autoGeneratePrompts` | `false` (disabled) | When `true`, the spec watcher auto-fires the generator container when a spec is approved. When `false` (the default), the watcher logs an INFO line and does NOT start the generator; operators run `/dark-factory:generate-prompts-for-spec <spec-path>` manually to trigger generation. |

**When to use**: You want to approve a spec to lock its contents but defer prompt generation — review the generated prompts before the container runs, run with custom args, or skip generation entirely for spec-only experiments. See [running.md § Two ways to generate prompts](running.md#two-ways-to-generate-prompts-from-an-approved-spec) for the auto-vs-manual comparison and decision guidance.

**Behavior**:
- `autoGeneratePrompts: false` (default): Approving a spec logs an INFO line and does NOT start the generator. The spec stays at `status: approved` in `specs/in-progress/`.
- `autoGeneratePrompts: true`: Approving a spec triggers the generator container. Prompts appear in `prompts/` automatically.

**Expected log line** (INFO level):
```
spec approved — auto-generation disabled, run /dark-factory:generate-prompts-for-spec <spec-path> manually
```

**Manual invocation** (when flag is `true`):
```bash
# Trigger generation for a specific spec
/dark-factory:generate-prompts-for-spec <spec-path>

# Example: trigger for spec 088
/dark-factory:generate-prompts-for-spec specs/in-progress/088-disable-auto-prompt-generation.md
```

**Per-invocation override** (no yaml editing needed):
```bash
dark-factory daemon --set autoGeneratePrompts=true
dark-factory run --set autoGeneratePrompts=false
```

**Layering precedence**: default (`false`) ← global (`~/.config/dark-factory/config.yaml`, XDG-first) ← project (`.dark-factory.yaml`) ← CLI (`--set`). Matches `hideGit`, `autoRelease`, and other user-pref fields.

**Note**: This only affects the automatic trigger. The `commands/generate-prompts-for-spec.md` command on the host always works regardless of this flag.

## Common Patterns

### Run on an existing manual worktree

A project configured for `workflow: direct` (no clone, no auto-worktree) can still be run safely against a worktree you create by hand. Combine global `hideGit` with per-invocation auto-release-off — no project-config edits needed.

```bash
# One-time global setup (per machine)
mkdir -p ~/.config/dark-factory
cat > ~/.config/dark-factory/config.yaml <<YAML
hideGit: true
autoRelease: false
YAML

# Each session, in the manual worktree
cd /path/to/my-feature-worktree
dark-factory run
```

What this gives you:
- `hideGit: true` (global) — git status output is suppressed; the daemon trusts the worktree the operator chose, not the repo state at startup
- `autoRelease: false` (global) — completed prompts commit locally but are NOT pushed or tagged; the operator releases manually when the branch is merged
- Project's existing `workflow: direct` setting is unchanged — other operators (CI, other clones) keep their normal behavior

To override per invocation:
```bash
dark-factory run --set hideGit=false        # see git output for one run
dark-factory run --auto-approve             # auto-approve any new prompts found
dark-factory run --model claude-opus-4-7    # use opus for one run
```

### Personal model preference across all projects

```yaml
# ~/.config/dark-factory/config.yaml  (XDG)
model: claude-opus-4-7
```

Every project that doesn't set `model` in its `.dark-factory.yaml` will run with opus. A project that explicitly wants sonnet still wins. Per-invocation `--model NAME` always wins.

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

### `HOST_CACHE_DIR`

extraMounts `src` supports environment variable expansion. The `HOST_CACHE_DIR`
variable is auto-defaulted by dark-factory when unset:

- **macOS**: `$HOME/Library/Caches`
- **Linux/other**: `$XDG_CACHE_HOME` if set, otherwise `$HOME/.cache`

Set `HOST_CACHE_DIR` explicitly to override. Recommended for portable cache
mounts:

```yaml
extraMounts:
  - src: $HOST_CACHE_DIR/go-build
    dst: /home/node/.cache/go-build
```

Share documentation or config directories across repos without duplicating them:

> **Go module cache:** The Go module cache is no longer mounted by default. Add it explicitly if your project uses Go:
>
> ```yaml
> extraMounts:
>   - src: ${GOPATH}/pkg
>     dst: /home/node/go/pkg
> ```

```yaml
extraMounts:
  - src: ../docs/howto
    dst: /docs
  - src: ~/Documents/workspaces/coding/docs
    dst: /coding-docs
    readOnly: true
```

| Field | Required | Default | Purpose |
|-------|----------|---------|---------|
| `src` | yes | — | Host path. Environment variables (`$VAR`, `${VAR}`) are expanded. `~/` expanded to home. Relative paths resolved from project root. |
| `dst` | yes | — | Container path where `src` is mounted. |
| `readOnly` | no | `false` | Mount read-only (`:ro`). Set `true` for read-only access. Omitting the field defaults to read-write. |

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
| `project` | — | Optional override for the Docker container name prefix (`<project>-gen-<spec>`, `<project>-exec-<prompt>`). When absent, defaults to the git working tree root directory basename. Rejects empty or whitespace-only values. |
| `debounceMs` | `500` | File watcher debounce in milliseconds |
| `serverPort` | `0` | REST API port (0 = disabled) |

## Full Example

```yaml
workflow: clone
pr: true
autoMerge: true
autoReview: true
maxReviewRetries: 3
useCollaborators: true
defaultBranch: master
validationCommand: "make precommit"
validationPrompt: docs/dod.md
provider: github
containerImage: "docker.io/bborbe/claude-yolo:v0.11.1"
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
