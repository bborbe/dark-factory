---
status: active
---

# Config layering: global → project → CLI precedence

Validates the config layering feature introduced in spec 060. Checks:
1. Global config sets model; project config is silent → global wins
2. Project config explicitly sets model → project beats global
3. CLI `--model` flag → arg beats both
4. CLI `--no-hide-git` flag with global `hideGit: true` → arg beats global
5. Effective-config log shows the correct `*Source` for each scenario

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

## Setup

```bash
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/dark-factory-sandbox"
cd "$WORK_DIR/dark-factory-sandbox"

# Baseline project config (no model, no hideGit set explicitly)
cat > .dark-factory.yaml << 'YAML'
pr: false
worktree: false
maxContainers: 999
YAML

git init --bare "$WORK_DIR/remote.git"
git remote set-url origin "$WORK_DIR/remote.git"

# Global config: set model and hideGit
mkdir -p ~/.dark-factory
cat > ~/.dark-factory/config.yaml << 'YAML'
model: claude-opus-4-7
hideGit: true
YAML
```

- [ ] `~/.dark-factory/config.yaml` exists with `model: claude-opus-4-7` and `hideGit: true`
- [ ] `.dark-factory.yaml` does NOT set `model` or `hideGit`
- [ ] `/tmp/new-dark-factory` binary is freshly built

## Scenario A: global model applies when project is silent

Run `dark-factory run` (no prompts — it will exit immediately with nothing to do; we only care about the startup log):

```bash
timeout 15s /tmp/new-dark-factory run > run-a.log 2>&1 || true
```

### Expected A

- [ ] `run-a.log` contains `model=claude-opus-4-7`
- [ ] `run-a.log` contains `modelSource=global`
- [ ] `run-a.log` contains `hideGit=true`
- [ ] `run-a.log` contains `hideGitSource=global`

```bash
grep -E "model=claude-opus-4-7|modelSource=global" run-a.log
grep -E "hideGit=true|hideGitSource=global" run-a.log
```

## Scenario B: project model beats global model

```bash
cat >> .dark-factory.yaml << 'YAML'
model: claude-sonnet-4-6
YAML
timeout 15s /tmp/new-dark-factory run > run-b.log 2>&1 || true
```

### Expected B

- [ ] `run-b.log` contains `model=claude-sonnet-4-6`
- [ ] `run-b.log` contains `modelSource=project`
- [ ] `run-b.log` contains `hideGit=true` (not overridden by project)
- [ ] `run-b.log` contains `hideGitSource=global`

```bash
grep -E "model=claude-sonnet-4-6|modelSource=project" run-b.log
grep -E "hideGitSource=global" run-b.log
```

Reset project model:
```bash
# Remove the model line from .dark-factory.yaml
grep -v "^model:" .dark-factory.yaml > .dark-factory.yaml.tmp && mv .dark-factory.yaml.tmp .dark-factory.yaml
```

## Scenario C: CLI --model flag beats both

```bash
timeout 15s /tmp/new-dark-factory run --model claude-haiku-4-5 > run-c.log 2>&1 || true
```

### Expected C

- [ ] `run-c.log` contains `model=claude-haiku-4-5`
- [ ] `run-c.log` contains `modelSource=arg`

```bash
grep -E "model=claude-haiku-4-5|modelSource=arg" run-c.log
```

## Scenario D: CLI --no-hide-git beats global hideGit=true

```bash
timeout 15s /tmp/new-dark-factory run --no-hide-git > run-d.log 2>&1 || true
```

### Expected D

- [ ] `run-d.log` contains `hideGit=false`
- [ ] `run-d.log` contains `hideGitSource=arg`

```bash
grep -E "hideGit=false|hideGitSource=arg" run-d.log
```

## Scenario E: invalid global config fails startup

```bash
cat > ~/.dark-factory/config.yaml << 'YAML'
dirtyFileThreshold: -5
YAML
timeout 10s /tmp/new-dark-factory run > run-e.log 2>&1 || true
```

### Expected E

- [ ] Command exited non-zero
- [ ] `run-e.log` contains `dirtyFileThreshold` in the error message
- [ ] `run-e.log` contains `globalconfig` in the error message (names the file's context)

```bash
grep -i "dirtyFileThreshold" run-e.log
grep -i "globalconfig" run-e.log
```

Restore valid global config:
```bash
cat > ~/.dark-factory/config.yaml << 'YAML'
model: claude-opus-4-7
hideGit: true
YAML
```

## Scenario F: contradictory flags rejected

```bash
timeout 5s /tmp/new-dark-factory run --hide-git --no-hide-git > run-f.log 2>&1 || true
```

### Expected F

- [ ] Command exited non-zero
- [ ] `run-f.log` contains `mutually exclusive` (or similar)

```bash
grep -i "exclusive\|contradictory\|mutually" run-f.log
```

## Scenario G: model with shell metachar rejected

```bash
timeout 5s /tmp/new-dark-factory run --model 'claude;rm -rf /' > run-g.log 2>&1 || true
```

### Expected G

- [ ] Command exited non-zero
- [ ] `run-g.log` contains `invalid characters` or similar validation error

```bash
grep -i "invalid" run-g.log
```

## Scenario H: no global config file → defaults apply (no behavior change)

```bash
rm ~/.dark-factory/config.yaml
timeout 15s /tmp/new-dark-factory run > run-h.log 2>&1 || true
```

### Expected H

- [ ] Command did NOT exit with a config error
- [ ] `run-h.log` contains `model=claude-sonnet-4-6` (the default)
- [ ] `run-h.log` contains `modelSource=default`

```bash
grep -E "model=claude-sonnet-4-6|modelSource=default" run-h.log
```

## Failure modes this catches

| Failure | Symptom |
|---------|---------|
| Global config not loaded | `modelSource=default` even when `~/.dark-factory/config.yaml` sets `model` |
| Project override not detected | Project sets `model: claude-sonnet-4-6` but `modelSource=global` appears in log |
| Arg override not applied | `--model claude-haiku-4-5` but log shows different model |
| Invalid global config not rejected | `dirtyFileThreshold: -5` in global file but startup succeeds |
| Contradictory flags not rejected | `--hide-git --no-hide-git` succeeds silently |
| Shell metachar in --model not rejected | `--model 'claude;rm -rf /'` succeeds without validation error |
| Missing global file crashes daemon | Deleting `~/.dark-factory/config.yaml` causes a startup error instead of silently using defaults |

## Cleanup

```bash
rm -f ~/.dark-factory/config.yaml
rm -rf "$WORK_DIR"
```
