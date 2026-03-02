---
status: completed
container: dark-factory-053-project-name-container-prefix
dark-factory-version: v0.12.0
created: "2026-03-02T21:53:52Z"
queued: "2026-03-02T21:53:52Z"
started: "2026-03-02T21:53:52Z"
completed: "2026-03-02T22:05:39Z"
---
<objective>
Use the project name as Docker container prefix instead of "dark-factory".
Container names become `{project}-{prompt-basename}` instead of `dark-factory-{prompt-basename}`.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/config/config.go` — Config struct with yaml tags.
Read `pkg/processor/processor.go` — `setupPromptMetadata` builds containerName as `"dark-factory-" + baseName`.
Read `pkg/factory/factory.go` — wiring, CreateRunner passes config to components.
Read `main.go` — entry point.
</context>

<requirements>

## 1. Add ProjectName to Config

In `pkg/config/config.go`:

```go
type Config struct {
    ProjectName    string   `yaml:"projectName"`
    // ... existing fields
}
```

No default in `Defaults()` — empty string means auto-detect.
No validation required — empty is valid (auto-detect).

## 2. Add project name resolution

Create `pkg/project/name.go`:

```go
package project

// Name resolves the project name using the fallback chain:
// 1. Config override (if non-empty)
// 2. Git remote repo name (origin URL → extract repo name)
// 3. Working directory name
func Name(configOverride string) string
```

Implementation:
- If `configOverride != ""`, return it
- Try `git rev-parse --show-toplevel`, then extract basename
- If that also fails, parse `git remote get-url origin` and extract repo name (strip .git suffix)
- Fallback: `filepath.Base` of current working directory

Keep it simple — no error returns, always returns something.

## 3. Update processor to use project name

Pass project name to processor. Update `NewProcessor` to accept `projectName string`.

In `setupPromptMetadata`, change:
```go
// Before:
containerName := "dark-factory-" + baseName

// After:
containerName := p.projectName + "-" + baseName
```

## 4. Update factory wiring

In `pkg/factory/factory.go`, resolve the project name and pass it to `CreateProcessor`:

```go
func CreateRunner(cfg config.Config, ver string) runner.Runner {
    projectName := project.Name(cfg.ProjectName)
    // ... pass projectName to CreateProcessor
}
```

## 5. Add Docker labels

When building Docker run args in `pkg/executor/executor.go`, add labels:

```go
"--label", "dark-factory.project=" + projectName,
"--label", "dark-factory.prompt=" + promptBaseName,
```

This requires passing projectName through to the executor. Update `NewDockerExecutor` to accept `projectName string`.

## 6. Log the resolved project name at startup

Add an slog.Info at startup showing the resolved project name:
```go
slog.Info("project name resolved", "name", projectName)
```

## 7. Add tests

In `pkg/project/name_test.go` (Ginkgo v2 + Gomega):
- Config override returns override value
- Empty override in a git repo returns repo directory name
- Verify sanitization (no special chars in container name)

</requirements>

<constraints>
- Do NOT change the prompt file format or frontmatter
- Do NOT modify pkg/report/ or pkg/watcher/
- Container names must be Docker-safe: [a-zA-Z0-9_-] only
- The sanitizeContainerName function already exists in processor.go — reuse it
- Fallback chain must always return a non-empty string
</constraints>

<verification>
Run: `make test`
Run: `make precommit`
</verification>

<success_criteria>
- Container names use project name prefix instead of "dark-factory"
- Config override works via `projectName` in .dark-factory.yaml
- Auto-detect works from git repo name
- Docker labels added to containers
- All tests pass
- `make precommit` passes
</success_criteria>
