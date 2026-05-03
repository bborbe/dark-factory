# Configuration Layering Design

dark-factory resolves each config field by walking a precedence chain. This doc defines the layers, which fields belong at which layer, the secret carve-out, and the migration plan.

## Layer Model

Five layers, last writer wins:

```
default ← global ← project ← env ← arg
```

| Layer | Source | Scope | Examples |
|---|---|---|---|
| 1. Default | hardcoded constants in `pkg/config.Defaults()` | All fields | `model: claude-sonnet-4-6`, `maxContainers: 3` |
| 2. Global | `~/.dark-factory/config.yaml` | User-level prefs | `hideGit`, `autoRelease`, `model`, `maxContainers`, `dirtyFileThreshold` |
| 3. Project | `.dark-factory.yaml` in repo | Repo-shape | `workflow`, `validationCommand`, dirs |
| 4. Env | `DF_<FIELD>` env vars (planned, not implemented) | Ad-hoc / CI overrides | `DF_HIDE_GIT=true` |
| 5. Arg | CLI flags | Per-invocation | `--model NAME`, `--max-containers N`, `--skip-preflight`, `--auto-approve`, `--set key=value` |

## Field Categories

### A. User-prefs (eligible for global)

User's machine-wide preference. Same value across most projects, but per-project override is fine.

Currently supported in `~/.dark-factory/config.yaml`:

- `model` — which Claude model to use
- `hideGit` — display preference
- `maxContainers` — concurrency cap on this machine
- `autoRelease` — "I always want auto-release"
- `dirtyFileThreshold` — personal tolerance for repo mess

Planned but not yet supported globally (project-only today):

- `containerImage` — YOLO image version
- `claudeDir` — where `~/.claude-yolo` lives
- `verificationGate` — "always verify before completing"

### B. Project-shape (project-only)

Inherently per-repo. No sane global default exists.

- `workflow` / `pr` / `autoMerge` / `worktree` — depends on the repo's review culture; also available as `--set` arg overrides (per-invocation only; global config does not include them)
- `defaultBranch` — repo-shape (master vs main)
- `validationCommand`, `testCommand`, `preflightCommand`, `generateCommand` — each repo's makefile
- `prompts.*Dir`, `specs.*Dir` — directory layout
- `projectName` — auto-resolved from repo
- `serverPort` — collision-prone, set per project
- `extraMounts`, `env` — repo-specific
- `additionalInstructions`, `validationPrompt` — per-codebase guidance

### C. Per-invocation (arg-only)

Toggled fresh each run. No yaml home.

- `--skip-preflight` — one-off override
- `--auto-approve` — flush queue
- `-debug` — verbose logging

### D. Secrets (special — see below)

- `github.token`
- `bitbucket.tokenEnv` (already env-ref by design)
- `notifications.telegram.botTokenEnv`, `chatIDEnv`
- `notifications.discord.webhookEnv`
- `ANTHROPIC_API_KEY` (already env-only)

## Secrets Carve-out

Secrets are not a layer — they are a **constraint** on what layers may carry them.

| Field type | Allowed sources | Forbidden |
|---|---|---|
| Non-secret | default, global yaml, project yaml, env, arg | — |
| **Secret** | env, mounted file | yaml file with literal value |

### Rules

1. Secret fields **never carry literal values** at any yaml layer
2. Validation enforces env-ref pattern: `token: ${GITHUB_TOKEN}` ✓ vs `token: ghp_xxx` ✗
3. Resolved late (at access time, not config load) — prevents accidental log dumps
4. Auto-redacted in `effective config` log line
5. Listed explicitly in a registry — enumerable, not heuristic

### Current state

| Aspect | Status |
|---|---|
| `github.token` validates as env-ref or empty | ✓ |
| Notifications use `*Env` field naming | ✓ |
| Central secret registry | ✗ — to add |
| `effective config` log redaction | ⚠ verify before adding more |

## Precedence Implementation

Each field's effective value is computed by:

```go
value := defaults.Field
if global.IsSet("field") { value = global.Field }
if project.IsSet("field") { value = project.Field }
if env.IsSet("DF_FIELD") { value = env.Field }   // Phase 2 — not yet implemented
if arg.IsSet("--field") { value = arg.Field }
```

**Key constraints:**

- Each layer must distinguish "unset" from "zero value" (use `*bool`, `*int` or a sentinel) — otherwise `hideGit: false` at layer 3 silently overrides `hideGit: true` at layer 2
- Validation runs once on the final merged Config, not per-layer
- Project config is currently authoritative (loaded → validated → used). Migration adds layers behind it without changing downstream consumers
- Secrets always resolve from env regardless of which layer "set" them

## Migration Plan

### Phase 1: Establish global expansion (done)

Moved 5 user-pref fields to support layer 2 with proper precedence:

1. `hideGit`
2. `autoRelease`
3. `dirtyFileThreshold`
4. `model`
5. `maxContainers`

CLI args (layer 5) implemented:

- `--model NAME` — model override
- `--max-containers N` — concurrency override
- `--skip-preflight` — bypass preflight
- `--auto-approve` — flush queue (run only)
- `--set key=value` — generic per-invocation override; supported keys: `hideGit`, `autoRelease`, `dirtyFileThreshold`, `model`, `maxContainers`, `workflow`, `pr`, `autoMerge`. Bool values must be `true` or `false` (no 1/0/yes/no). Enum (`workflow`) accepts `direct`, `branch`, `worktree`, `clone`.

**Retrospective note (spec 062):** `workflow`, `pr`, and `autoMerge` are category B (project-shape) fields and were originally declared project-only forever. Spec 062 intentionally adds them to `--set` — the ergonomics win (one-shot delivery overrides without editing `.dark-factory.yaml`) outweighs the surface-area cost. The global config layer (layer 2) does NOT gain these fields; they remain project-only at the yaml layer. Only the arg layer (layer 5) is extended.

There is no `--hide-git` flag — use `--set hideGit=true`.

### Phase 2: Env layer

Once layer-merging is proven, add `DF_<FIELD>` env layer with same precedence rule.
Cheap: same merge code, one more source.

### Phase 3: Remaining user-prefs

Move category A leftovers to support layer 2: `containerImage`, `claudeDir`, `verificationGate`.

### Phase 4: Secrets registry

Codify `SecretField` tag on struct fields. Validate, redact, resolve at access. No behavior change — formalizes what's already partly enforced.

### Out of scope

- Project-shape (category B) fields stay project-only
- Lifecycle dirs stay project-only (extreme repo-shape)
- `extraMounts`, `env`, `notifications.*` — too structured for global override; per-project only

## Open Questions

- Should `--model` accept a short alias (`--model opus`) → resolve to full ID? (Probably yes, ergonomics.)
- For booleans at layer 2/3, use `*bool` sentinel or "default to false but distinguish absent"? (Likely `*bool`.)
- Validation layer ordering: validate each layer independently, or merge-then-validate? (Merge-then-validate — see Implementation above.)
- `effective config` log line: redact-by-list or redact-by-tag? (Tag once secrets registry exists.)
