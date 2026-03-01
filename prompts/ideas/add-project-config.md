---
status: queued
---
# Add project configuration file support

## Goal

Dark-factory needs project-level configuration to support different workflows (direct-to-master vs PR-based). Add `.dark-factory.yaml` config file support.

## Current Behavior

Dark-factory has no configuration — it always commits directly to master with tag+push.

## Expected Behavior

On startup, read `.dark-factory.yaml` from the project root (working directory). If missing, use defaults (backward compatible).

## Implementation

### 1. Add `pkg/config/` package

```go
// pkg/config/config.go

// Workflow defines how dark-factory handles git operations after prompt execution.
type Workflow string

const (
    // WorkflowDirect commits, tags, and pushes directly to the current branch (default).
    WorkflowDirect Workflow = "direct"
    // WorkflowPR creates a feature branch, commits, pushes, and opens a pull request.
    WorkflowPR Workflow = "pr"
)

// Config holds dark-factory project configuration.
//
//counterfeiter:generate -o ../../mocks/config-loader.go --fake-name ConfigLoader . ConfigLoader
type ConfigLoader interface {
    Load(ctx context.Context) (*Config, error)
}

type Config struct {
    Workflow Workflow `yaml:"workflow"`
}

type configLoader struct{}

func NewConfigLoader() ConfigLoader {
    return &configLoader{}
}

func (c *configLoader) Load(ctx context.Context) (*Config, error) {
    data, err := os.ReadFile(".dark-factory.yaml")
    if os.IsNotExist(err) {
        return &Config{Workflow: WorkflowDirect}, nil
    }
    if err != nil {
        return nil, errors.Wrap(ctx, err, "read config")
    }
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, errors.Wrap(ctx, err, "parse config")
    }
    if cfg.Workflow == "" {
        cfg.Workflow = WorkflowDirect
    }
    return &cfg, nil
}
```

### 2. Add yaml dependency

```bash
go get gopkg.in/yaml.v3
```

### 3. Update runner constructor

Inject `ConfigLoader` into runner:

```go
func NewRunner(
    promptsDir string,
    exec executor.Executor,
    promptManager prompt.PromptManager,
    releaser git.Releaser,
    configLoader config.ConfigLoader,
) Runner
```

Runner loads config once in `Run()` and stores it for use in `processPrompt()`.

### 4. Update factory

```go
func CreateRunner(promptsDir string) runner.Runner {
    return runner.NewRunner(
        promptsDir,
        executor.NewDockerExecutor(),
        prompt.NewPromptManager(promptsDir),
        git.NewReleaser(),
        config.NewConfigLoader(),
    )
}
```

### 5. Tests

- `Load()` returns default config when `.dark-factory.yaml` missing
- `Load()` returns `WorkflowPR` when file contains `workflow: pr`
- `Load()` returns `WorkflowDirect` when file contains `workflow: direct`
- `Load()` returns default when `workflow` field is empty
- `Load()` returns error for invalid YAML
- Runner tests use mock ConfigLoader

## Config File Format

```yaml
# .dark-factory.yaml
workflow: pr    # "direct" (default) or "pr"
```

## Constraints

- Backward compatible — missing file = direct workflow (current behavior)
- Run `make precommit` for validation only — do NOT commit, tag, or push
- Follow `~/.claude-yolo/docs/go-composition.md` — ConfigLoader is injected, not called directly
- Follow `~/.claude-yolo/docs/go-patterns.md` for interface + constructor pattern
