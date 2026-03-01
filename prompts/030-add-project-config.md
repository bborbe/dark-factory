# Add project configuration file support

## Goal

Dark-factory has hardcoded values scattered across packages. Add `.dark-factory.yaml` config file support to centralize configuration.

## Current Hardcoded Values

| Setting | Current Value | Location |
|---------|--------------|----------|
| workflow | direct (implicit) | no config exists |
| promptsDir | `"prompts"` | `main.go` → factory |
| containerImage | `"docker.io/bborbe/claude-yolo:v0.0.7"` | `pkg/executor/executor.go:17` |
| debounceMs | `500` | `pkg/watcher/watcher.go:120` |

## Expected Behavior

On startup, read `.dark-factory.yaml` from the project root (working directory). If missing, use defaults matching current hardcoded values (backward compatible).

## Config File Format

```yaml
# .dark-factory.yaml
workflow: direct        # "direct" (default) or "pr"
promptsDir: prompts     # directory to watch (default: "prompts")
containerImage: docker.io/bborbe/claude-yolo:v0.0.7  # YOLO image (default: current)
debounceMs: 500         # watcher debounce in milliseconds (default: 500)
```

## Implementation

### 1. Add `pkg/config/` package

```go
// pkg/config/config.go

type Workflow string

const (
    WorkflowDirect Workflow = "direct"
    WorkflowPR     Workflow = "pr"
)

type Config struct {
    Workflow       Workflow       `yaml:"workflow"`
    PromptsDir     string        `yaml:"promptsDir"`
    ContainerImage string        `yaml:"containerImage"`
    DebounceMs     int           `yaml:"debounceMs"`
}

// Defaults returns a Config with all default values.
func Defaults() Config {
    return Config{
        Workflow:       WorkflowDirect,
        PromptsDir:     "prompts",
        ContainerImage: "docker.io/bborbe/claude-yolo:v0.0.7",
        DebounceMs:     500,
    }
}
```

Follow `~/.claude-yolo/docs/go-enum-pattern.md` for `Workflow` type — add `Validate()`, `AvailableWorkflows`.

Follow `~/.claude-yolo/docs/go-validation.md` for `Config.Validate()`:
```go
func (c Config) Validate(ctx context.Context) error {
    return validation.All{
        validation.Name("workflow", c.Workflow),
        validation.Name("promptsDir", validation.NotEmptyString(c.PromptsDir)),
        validation.Name("containerImage", validation.NotEmptyString(c.ContainerImage)),
        validation.Name("debounceMs", validation.HasValidationFunc(func(ctx context.Context) error {
            if c.DebounceMs <= 0 {
                return errors.Errorf(ctx, "debounceMs must be positive, got %d", c.DebounceMs)
            }
            return nil
        })),
    }.Validate(ctx)
}
```

### 2. Add ConfigLoader interface

```go
//counterfeiter:generate -o ../../mocks/config-loader.go --fake-name ConfigLoader . ConfigLoader
type ConfigLoader interface {
    Load(ctx context.Context) (Config, error)
}
```

`Load()` reads `.dark-factory.yaml`, merges with defaults (missing fields get default values), validates, returns.

### 3. Add yaml dependency

```bash
go get gopkg.in/yaml.v3
```

### 4. Wire config through factory

Factory loads config first, then passes values to constructors:
- `Config.PromptsDir` → all packages that use promptsDir
- `Config.ContainerImage` → executor
- `Config.DebounceMs` → watcher (as `time.Duration`)
- `Config.Workflow` → processor (for future PR workflow)

Remove hardcoded `claudeYoloImage` constant from `pkg/executor/executor.go`. Pass as constructor parameter instead.

Remove hardcoded `500*time.Millisecond` from `pkg/watcher/watcher.go`. Pass as constructor parameter instead.

### 5. Update main.go

Load config before creating runner:
```go
loader := config.NewConfigLoader()
cfg, err := loader.Load(ctx)
if err != nil {
    log.Fatal(err)
}
runner := factory.CreateRunner(cfg)
```

Factory signature changes: `CreateRunner(cfg config.Config) runner.Runner`

### 6. Tests

- `Load()` returns defaults when `.dark-factory.yaml` missing
- `Load()` merges partial config with defaults (e.g., only `workflow: pr` → other fields get defaults)
- `Load()` returns error for invalid YAML
- `Load()` returns error for invalid workflow value
- `Load()` returns error for negative debounceMs
- `Config.Validate()` passes for valid config, fails for each invalid field
- `Workflow.Validate()` passes for direct/pr, fails for unknown
- Executor uses injected containerImage (not hardcoded)
- Watcher uses injected debounce duration (not hardcoded)

## Constraints

- Backward compatible — missing file = all defaults (current behavior)
- Run `make precommit` for validation only — do NOT commit, tag, or push
- Follow `~/.claude-yolo/docs/go-composition.md` — ConfigLoader is injected, not called directly
- Follow `~/.claude-yolo/docs/go-patterns.md` for interface + constructor pattern
- Follow `~/.claude-yolo/docs/go-enum-pattern.md` for Workflow type
- Follow `~/.claude-yolo/docs/go-validation.md` for Config.Validate()
- Follow `~/.claude-yolo/docs/go-precommit.md` for linter limits
- Coverage ≥80% for changed packages
