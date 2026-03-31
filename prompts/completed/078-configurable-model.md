---
status: completed
spec: [002-configuration]
summary: Added configurable model field to Config struct with default claude-sonnet-4-6
container: dark-factory-078-configurable-model
dark-factory-version: v0.17.9
created: "2026-03-05T21:30:25Z"
queued: "2026-03-05T21:30:25Z"
started: "2026-03-05T21:30:25Z"
completed: "2026-03-05T21:39:51Z"
---

Add a `model` field to `pkg/config/config.go` `Config` struct with default value `"claude-sonnet-4-6"` and yaml tag `model`.

## Changes required

### 1. `pkg/config/config.go`

Add to `Config` struct (after `ContainerImage`):
```go
Model string `yaml:"model"`
```

Add to `Defaults()` (after `ContainerImage`):
```go
Model: "claude-sonnet-4-6",
```

Add to `Validate()` (after `containerImage` validation):
```go
validation.Name("model", validation.NotEmptyString(c.Model)),
```

### 2. `pkg/executor/executor.go`

Add `model string` field to `dockerExecutor` struct.

Update `NewDockerExecutor` signature:
```go
func NewDockerExecutor(containerImage string, projectName string, model string) Executor
```

Pass `model` to struct. In `buildDockerCommand`, add to docker args before the image:
```go
"-e", "ANTHROPIC_MODEL=" + e.model,
```

### 3. `pkg/factory/factory.go`

Thread `cfg.Model` through `CreateProcessor` → `executor.NewDockerExecutor`.

Update `CreateProcessor` signature to accept `model string` and pass it to `NewDockerExecutor`.

In `CreateRunner`, pass `cfg.Model` to `CreateProcessor`.

## Tests

Update `executor_test.go` and `factory_test.go` to pass model parameter where needed.

Run `make test` to verify.
