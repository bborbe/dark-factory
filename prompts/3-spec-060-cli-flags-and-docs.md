---
status: draft
spec: [060-config-layering-phase-1]
created: "2026-05-01T08:02:00Z"
---

<summary>
- `dark-factory run` and `dark-factory daemon` accept three new CLI flags: `--hide-git`, `--no-hide-git`, and `--model NAME`.
- The flags override yaml values for the duration of the invocation. Source-tracking shows `hideGitSource=arg` / `modelSource=arg` in the effective-config log.
- Passing both `--hide-git` and `--no-hide-git` in the same invocation is a usage error.
- `--model` requires a value that matches the same regex used at every layer (`^[a-zA-Z0-9._:/-]{1,256}$`). Shell metacharacters and whitespace are rejected.
- `docs/configuration.md` documents the new flags and the layering precedence; `docs/config-layering.md` is linked from the docs index.
- A new scenario `scenarios/013-config-layering.md` provides a manual checklist verifying default, global, project, and CLI-arg precedence end-to-end.
- The `example/.dark-factory.yaml` file is refreshed to cover all current Config fields (out of date today), serving as a complete reference for operators.
</summary>

<objective>
Add CLI-flag layer (layer 4) on top of the layered config from prompt 2: `--hide-git`, `--no-hide-git`, and `--model NAME`. Document the full layering model in `docs/configuration.md`. Add a scenario that validates layered precedence end-to-end. Refresh `example/.dark-factory.yaml` to reflect all currently-supported fields.

**Precondition:** Prompt 2 (`2-spec-060-loader-merge-and-sources.md`) is complete. `pkg/config.Loader.Load` returns `(Config, ResolutionSources, error)`, and source tracking is wired through to `LogEffectiveConfig`.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Read these guides in `~/.claude/plugins/marketplaces/coding/docs/`:
- `go-cli-guide.md`
- `go-validation-framework-guide.md`
- `go-testing-guide.md`

Read these files before editing:
- `main.go` — `ParseArgs` (~line 685), `runCommand` (~line 113), `runRunCommand`, `runDaemonCommand`, `printRunHelp`, `printDaemonHelp`, `printHelp`, `extractMaxContainers` (existing flag-extraction pattern)
- `parse_args_test.go` — `parseArgsResult`, `assertParseArgs`, existing flag tests (use as template)
- `main_internal_test.go` — Ginkgo `ParseArgs` Describe block
- `pkg/factory/factory.go` — `CreateRunner`, `CreateOneShotRunner` signatures (need new params for hide-git / model overrides)
- `pkg/config/sources.go` (or `loader.go`) — `ResolutionSources` struct (extend with `arg` source value)
- `pkg/globalconfig/globalconfig.go` — `ModelPattern`, `ModelRegex` (shared)
- `docs/configuration.md` — existing CLI Flags section (`--max-containers`, `--skip-preflight`)
- `docs/config-layering.md` — design doc (already exists)
- `scenarios/010-preflight-baseline-gate.md` — scenario style template
- `scenarios/001-workflow-direct.md` — scenario frontmatter template
- `example/.dark-factory.yaml` — currently out of date, needs full refresh
- `pkg/config/config.go` — `Config` struct (full list of currently-supported fields for the example refresh)

Spec: `specs/in-progress/060-config-layering-phase-1.md` — desired behaviors 5, 6, 7, 8 and the `--model` regex constraint.
</context>

<requirements>

## 1. Add CLI flag extraction to `ParseArgs`

In `main.go`, the `ParseArgs` function currently returns `(bool, string, string, []string, bool, bool)` — six return values: `debug, command, subcommand, args, autoApprove, skipPreflight`.

This signature is approaching the smell limit. Refactor to return a struct:

```go
// ParsedArgs holds the result of parsing dark-factory CLI arguments.
type ParsedArgs struct {
    Debug          bool
    Command        string
    Subcommand     string
    Args           []string
    AutoApprove    bool
    SkipPreflight  bool
    HideGit        *bool   // nil = no flag passed; true = --hide-git; false = --no-hide-git
    Model          *string // nil = no flag passed; non-nil = validated --model NAME value
}

func ParseArgs(rawArgs []string) (ParsedArgs, error) {
    // ... existing logic, plus new flag extraction below ...
}
```

The signature now returns `(ParsedArgs, error)` — the error path is for `--model` validation (regex mismatch, missing value) and contradictory `--hide-git` / `--no-hide-git`.

Update all callers of `ParseArgs` (search `ParseArgs(os.Args[1:])` in `main.go` line ~40 and any internal calls) to use the struct + error.

## 2. Implement flag extraction logic

Inside `ParseArgs`, extend the loop that processes `rawArgs`:

```go
hideGit := (*bool)(nil)
hideGitFalseSeen := false
hideGitTrueSeen := false
modelArg := (*string)(nil)

filtered := make([]string, 0, len(rawArgs))
i := 0
for i < len(rawArgs) {
    arg := rawArgs[i]
    switch arg {
    case "-debug":
        debug = true
    case "--auto-approve":
        autoApprove = true
    case "--skip-preflight":
        skipPreflight = true
    case "--hide-git":
        hideGitTrueSeen = true
        t := true
        hideGit = &t
    case "--no-hide-git":
        hideGitFalseSeen = true
        f := false
        hideGit = &f
    case "--model":
        if i+1 >= len(rawArgs) {
            return ParsedArgs{}, errors.Errorf(ctx, "--model requires a value")
        }
        v := rawArgs[i+1]
        if !globalconfig.ModelRegex.MatchString(v) {
            return ParsedArgs{}, errors.Errorf(
                ctx,
                "--model value %q does not match required pattern %s",
                v,
                globalconfig.ModelPattern,
            )
        }
        modelArg = &v
        i++ // consume the value
    default:
        filtered = append(filtered, arg)
    }
    i++
}

if hideGitTrueSeen && hideGitFalseSeen {
    return ParsedArgs{}, errors.Errorf(ctx, "cannot pass both --hide-git and --no-hide-git in the same invocation")
}
```

Note: `ParseArgs` currently does not take a `ctx` parameter. Add it:

```go
func ParseArgs(ctx context.Context, rawArgs []string) (ParsedArgs, error)
```

Update the call site in `main()` to pass the existing `ctx`.

The existing `extractMaxContainers` helper takes `ctx` and works similarly — model the new extraction on it.

## 3. Restrict to `run` and `daemon` commands

Like `--skip-preflight`, the new flags only make sense for `run` and `daemon`. Adapt the existing rejection block in `runCommand`:

Find the block (~line 113 area) that currently rejects `--skip-preflight` on unsupported commands. Extend it:

```go
if skipPreflight || hideGit != nil || modelArg != nil {
    switch command {
    case "run", "daemon":
        // valid
    default:
        switch {
        case skipPreflight:
            return errors.Errorf(ctx, "unknown flag: --skip-preflight")
        case hideGit != nil:
            return errors.Errorf(ctx, "--hide-git / --no-hide-git only valid for run and daemon")
        case modelArg != nil:
            return errors.Errorf(ctx, "--model only valid for run and daemon")
        }
    }
}
```

## 4. Apply overrides after config load

After `config.NewLoader().Load(ctx, globalCfg)` returns `(cfg, sources, err)`, apply the CLI flag overrides:

```go
if parsed.HideGit != nil {
    cfg.HideGit = *parsed.HideGit
    sources.HideGit = "arg"
}
if parsed.Model != nil {
    cfg.Model = *parsed.Model
    sources.Model = "arg"
}
```

Place this block in both `runRunCommand` and `runDaemonCommand`, immediately after config load but before `cfg.Validate()` would have run (validate runs inside the loader, so the regex was already enforced at parse time — no re-validation needed).

Pass `parsed.HideGit` and `parsed.Model` from `runCommand` down through `runRunCommand` / `runDaemonCommand`. Both functions already accept `autoApprove`, `skipPreflight` — add `hideGit *bool` and `model *string` after.

## 5. Update help text

### 5a. `printRunHelp`

```go
func printRunHelp() {
    fmt.Fprintf(
        os.Stdout,
        "Usage: dark-factory run [--max-containers N] [--auto-approve] [--skip-preflight] [--hide-git|--no-hide-git] [--model NAME]\n\n"+
            "Process all queued prompts and exit.\n\n"+
            "Flags:\n"+
            "  --max-containers N   Override the container limit for this run\n"+
            "  --auto-approve       Automatically approve new prompts found during run\n"+
            "  --skip-preflight     Skip preflight baseline check for this invocation.\n"+
            "                       Prompts may run on a broken baseline — use with caution.\n"+
            "  --hide-git           Suppress git status output for this invocation\n"+
            "  --no-hide-git        Force git status output even if yaml says hideGit: true\n"+
            "  --model NAME         Override model for this invocation (e.g. claude-opus-4-7)\n"+
            "  --help, -h           Show this help\n",
    )
}
```

### 5b. `printDaemonHelp`

Same shape — include all four override flags.

### 5c. `printHelp`

Update the run/daemon command lines:

```go
"  run [--max-containers N] [--skip-preflight] [--hide-git|--no-hide-git] [--model NAME]    Process all queued prompts and exit\n"+
"  daemon [--max-containers N] [--skip-preflight] [--hide-git|--no-hide-git] [--model NAME] Watch for queued prompts and execute them (long-running)\n"+
```

## 6. Tests in `parse_args_test.go`

Update `parseArgsResult` to mirror the new struct. Update `assertParseArgs`. Add new tests:

```go
func TestParseArgsHideGit(t *testing.T) {
    t.Parallel()
    // --hide-git sets *bool to true
    // --no-hide-git sets *bool to false
    // neither flag → nil
    // both flags → error
}

func TestParseArgsModel(t *testing.T) {
    t.Parallel()
    // --model claude-opus-4-7 → *string "claude-opus-4-7"
    // --model qwen3.6:35b-a3b → success
    // --model "local/qwen3.6:35b-a3b" → success
    // --model docker.io/bborbe/claude-yolo:v0.6.1 → success
    // --model with no value → error mentioning "requires a value"
    // --model "foo;bar" → error mentioning "pattern"
    // --model "foo bar" (with space) → error mentioning "pattern"
    // --model "" → error mentioning "pattern" (empty doesn't match {1,256})
}
```

## 7. Tests in `main_internal_test.go`

Update the `ParseArgs` Describe block's local `result` struct and `parse` func to match the new return signature.

Add a new Describe block for the `runCommand` rejection of `--hide-git` and `--model` on unsupported commands, modeled on the existing skip-preflight rejection block.

## 8. Update `pkg/factory/factory_test.go`

The existing `CreateRunner` and `CreateOneShotRunner` call sites need the new `sources config.ResolutionSources` parameter added in prompt 2. If prompt 3 doesn't add new factory parameters, no change here. If prompt 3 changes the factory signature to accept arg-overrides directly, update existing calls.

Recommended: keep the factory signature minimal — apply CLI overrides in `main.go` BEFORE the factory call, so the factory only sees a final merged Config + sources. This avoids signature creep on the factory.

## 9. Update `docs/configuration.md`

Find the existing "CLI Flags" section (added by prompt 355 from spec 059, ~line 282 area). Extend it with the new flags:

```markdown
**`--hide-git` / `--no-hide-git`**

```bash
dark-factory run --hide-git
dark-factory daemon --no-hide-git
```

Override the `hideGit` setting for this invocation. `--hide-git` forces it on; `--no-hide-git` forces it off. Useful when the project yaml says `hideGit: true` but you want to debug a git-related issue without editing the file.

**`--model NAME`**

```bash
dark-factory run --model claude-opus-4-7
dark-factory daemon --model qwen3.6:35b-a3b
```

Override the model for this invocation. Validates against the same regex used in yaml: `^[a-zA-Z0-9._:/-]{1,256}$` — accepts Anthropic IDs, OSS model IDs (Ollama tags), namespaced/local paths, and Docker image refs. Shell metacharacters are rejected because the value flows to YOLO container args.

Priority: CLI arg > project config > global config > default.
```

Add (or extend) a section that lists the full precedence model with a link to the design doc:

```markdown
## Configuration Precedence

dark-factory resolves each setting through a layered chain:

```
default ← global ← project ← arg
```

| Layer | Source | Scope |
|---|---|---|
| 1. Default | hardcoded constants in dark-factory | All fields |
| 2. Global | `~/.dark-factory/config.yaml` | User-level prefs (`hideGit`, `autoRelease`, `dirtyFileThreshold`, `model`, `maxContainers`) |
| 3. Project | `.dark-factory.yaml` in repo | Repo-shape settings (everything) |
| 4. Arg | CLI flags | Per-invocation overrides |

The `effective config` startup log line shows which layer supplied each layered field (e.g. `hideGitSource=global`, `modelSource=arg`).

For background and the design rationale, see [config-layering.md](config-layering.md).
```

## 10. Create scenario `scenarios/013-config-layering.md`

Model on `scenarios/010-preflight-baseline-gate.md` and `scenarios/001-workflow-direct.md` (frontmatter conventions).

```markdown
---
status: active
---

# Config layering: default ← global ← project ← arg

Validates that user-level config (`~/.dark-factory/config.yaml`), project-level config (`.dark-factory.yaml`), and CLI flags compose correctly for `hideGit` and `model`. Verifies the `effective config` log line reports the correct source per field.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

## Setup

```bash
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/sandbox"
cd "$WORK_DIR/sandbox"

# Stash any existing global config
mv ~/.dark-factory/config.yaml ~/.dark-factory/config.yaml.bak 2>/dev/null || true

# Project config: workflow direct, model claude-sonnet-4-6 (project wins over future global)
cat > .dark-factory.yaml << 'YAML'
workflow: direct
autoRelease: false
model: claude-sonnet-4-6
YAML

git init --bare "$WORK_DIR/remote.git"
git remote set-url origin "$WORK_DIR/remote.git"
```

- [ ] Project yaml sets `model: claude-sonnet-4-6` and no `hideGit`
- [ ] No global config file exists (`ls ~/.dark-factory/config.yaml` → not found)
- [ ] Sandbox CHANGELOG.md unchanged

## Action 1 — defaults only (no global, no flag)

```bash
cd "$WORK_DIR/sandbox"
/tmp/new-dark-factory run > daemon.log 2>&1 || true
```

### Expected

- [ ] `daemon.log` contains `effective config`
- [ ] `daemon.log` contains `model=claude-sonnet-4-6 modelSource=project`
- [ ] `daemon.log` contains `hideGit=false hideGitSource=default`

## Action 2 — global config supplies hideGit, project still wins on model

```bash
mkdir -p ~/.dark-factory
cat > ~/.dark-factory/config.yaml << 'YAML'
hideGit: true
model: claude-opus-4-7
YAML

cd "$WORK_DIR/sandbox"
/tmp/new-dark-factory run > daemon-2.log 2>&1 || true
```

### Expected

- [ ] `daemon-2.log` contains `model=claude-sonnet-4-6 modelSource=project` (project still wins)
- [ ] `daemon-2.log` contains `hideGit=true hideGitSource=global` (global supplies hideGit since project doesn't)

## Action 3 — CLI arg overrides both yaml layers

```bash
cd "$WORK_DIR/sandbox"
/tmp/new-dark-factory run --model claude-haiku-4-5 --no-hide-git > daemon-3.log 2>&1 || true
```

### Expected

- [ ] `daemon-3.log` contains `model=claude-haiku-4-5 modelSource=arg`
- [ ] `daemon-3.log` contains `hideGit=false hideGitSource=arg`

## Action 4 — invalid model rejected

```bash
cd "$WORK_DIR/sandbox"
/tmp/new-dark-factory run --model 'foo;rm -rf /' > daemon-4.log 2>&1
echo "exit: $?"
```

### Expected

- [ ] Exit code is non-zero
- [ ] `daemon-4.log` contains `pattern` (regex rejection)
- [ ] No prompt was executed (sandbox state unchanged)

## Action 5 — contradictory hide-git flags rejected

```bash
cd "$WORK_DIR/sandbox"
/tmp/new-dark-factory run --hide-git --no-hide-git > daemon-5.log 2>&1
echo "exit: $?"
```

### Expected

- [ ] Exit code is non-zero
- [ ] `daemon-5.log` contains `cannot pass both --hide-git and --no-hide-git`

## Cleanup

```bash
rm -rf "$WORK_DIR"
rm -f ~/.dark-factory/config.yaml
mv ~/.dark-factory/config.yaml.bak ~/.dark-factory/config.yaml 2>/dev/null || true
```

## What this scenario locks down

| Failure | Symptom |
|---|---|
| Global layer ignored | Action 2: `hideGit=false hideGitSource=default` |
| Project does not win over global | Action 2: `model=claude-opus-4-7 modelSource=global` |
| Arg layer ignored | Action 3: `modelSource` is not `arg` |
| Regex bypass | Action 4: prompt executes despite shell metachar in --model |
| Both `--hide-git` and `--no-hide-git` accepted | Action 5: exit 0 |
```

## 11. Refresh `example/.dark-factory.yaml`

The current `example/.dark-factory.yaml` is missing many supported fields. Rewrite it to be a complete, currently-valid reference. Use `pkg/config/config.go` `Config` struct and `Defaults()` as the source of truth.

The refreshed file must:

- Cover every field in the `Config` struct
- Set realistic defaults (or commented-out optional fields)
- Be parseable by `config.NewLoader().Load(...)` without errors
- Include the four new global-eligible fields with a comment explaining "this can also live in `~/.dark-factory/config.yaml`"
- Update the stale `containerImage: docker.io/bborbe/claude-yolo:v0.5.4` to whatever `pkg.DefaultContainerImage` resolves to today

The file remains in `example/` and is documentation, not test data. It is parsed by no test today; just keep it valid yaml.

## 12. CHANGELOG

Append to `## Unreleased`:

```
- feat: --hide-git, --no-hide-git, --model NAME CLI flags for run and daemon (per-invocation overrides)
- docs: configuration.md documents the full layering precedence (default ← global ← project ← arg) with examples for each layer
- docs: refresh example/.dark-factory.yaml to cover all currently-supported fields
- test: scenario 013-config-layering exercises layered precedence end-to-end
```

## 13. Run validation

```bash
cd /workspace
make generate
make precommit
```

Both must exit 0. Run the scenario manually if possible (the file just needs to exist and parse — manual exercise is for the operator).

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT touch `go.mod` / `go.sum` / `vendor/`.
- The flags must NOT be persisted into `config.Config` — apply them to the loaded `cfg` after `Loader.Load` returns. They are per-invocation only.
- `--hide-git` and `--no-hide-git` apply only to `run` and `daemon`. Other commands (`status`, `list`, `prompt`, `spec`, `scenario`) reject them with a clear error via `errors.Errorf`.
- `--model` validation regex `^[a-zA-Z0-9._:/-]{1,256}$` lives in `pkg/globalconfig.ModelPattern` and `globalconfig.ModelRegex` (added in prompt 1, exported in prompt 2). Do NOT duplicate.
- Validation happens at the CLI layer (in `ParseArgs`) BEFORE any config load — reject malformed `--model` values before touching yaml.
- Use `errors.Errorf(ctx, ...)` from `github.com/bborbe/errors` for all new errors.
- The `ParseArgs` signature change to `(ParsedArgs, error)` is intentional. All callers update.
- Documentation goes to `docs/configuration.md` (operator-facing). The design doc `docs/config-layering.md` is already written and is referenced via link only.
- The scenario file follows the existing pattern (frontmatter, Setup, Action, Expected, Cleanup, "What this scenario locks down" table). It is a manual checklist, not auto-executed.
- `example/.dark-factory.yaml` refresh is documentation hygiene — keep it valid yaml that parses through `config.NewLoader`.
- Tests use Ginkgo/Gomega in external `_test` packages plus stdlib `testing` for table-driven cases.
- Existing scenarios 001, 006, 010, 011, 012 must still pass unchanged.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:

```bash
# New flags parsed
grep -n "\"--hide-git\"\|\"--no-hide-git\"\|\"--model\"" main.go

# Contradictory flags rejected
grep -n "cannot pass both --hide-git and --no-hide-git" main.go

# Regex shared, not duplicated
grep -rn "globalconfig.ModelRegex\|globalconfig.ModelPattern" main.go pkg/

# ParsedArgs struct used
grep -n "type ParsedArgs\|ParsedArgs{" main.go parse_args_test.go main_internal_test.go

# Help text updated
grep -n "hide-git\|--model NAME" main.go

# Scenario exists
ls scenarios/013-config-layering.md

# Example file refreshed
grep -E "preflightCommand|verificationGate|hideGit" example/.dark-factory.yaml

# CHANGELOG entries
grep -n "hide-git\|--model\|layering" CHANGELOG.md
```

```bash
make generate
make precommit
```

Manual scenario verification (operator runs after merge):

```bash
# Run scenario 013 setup actions and verify expected log lines
# This is documented in scenarios/013-config-layering.md
```
</verification>
